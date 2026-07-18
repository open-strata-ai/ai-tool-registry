// Package httpapi exposes the tool-registry HTTP surface (DESIGN §7.1 / SPECS
// §7). It uses the standard library net/http; production runs behind Higress with
// Keycloak JWT verification (auth-contract.md).
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	appmetering "github.com/open-strata-ai/ai-tool-registry/application/metering"
	"github.com/open-strata-ai/ai-tool-registry/application/registry"
	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// FeatureFlags gate the optional capability packages (DESIGN §2.3 / §11.5).
type FeatureFlags struct {
	Skills bool
	Rules  bool
	Specs  bool
}

// Handler wires the registry service and auth to HTTP endpoints.
type Handler struct {
	svc      *registry.Service
	auth     domain.AuthPort
	flags    FeatureFlags
	metricRec *appmetering.Recorder
}

// New builds a Handler.
func New(svc *registry.Service, auth domain.AuthPort, flags FeatureFlags, metrics *appmetering.Recorder) *Handler {
	return &Handler{svc: svc, auth: auth, flags: flags, metricRec: metrics}
}

// Routes returns a ServeMux with all endpoints registered.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tools", h.toolsRoot)
	mux.HandleFunc("/v1/tools/", h.toolsNamed)
	mux.HandleFunc("/v1/skills", h.skillsRoot)
	mux.HandleFunc("/v1/rules", h.rulesRoot)
	mux.HandleFunc("/v1/specs", h.specsRoot)
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/metrics", h.metrics)
	return mux
}

func (h *Handler) toolsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.registerTool(w, r)
	case http.MethodGet:
		h.listTools(w, r)
	default:
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (h *Handler) toolsNamed(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/tools/")
	parts := strings.Split(rest, "/")
	name := parts[0]
	if name == "" {
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest, "tool name required"))
		return
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	switch {
	case r.Method == http.MethodDelete && sub == "":
		h.deregisterTool(w, r, name)
	case r.Method == http.MethodPost && sub == "resolve":
		h.resolveTool(w, r, name)
	case r.Method == http.MethodPost && sub == "invoke":
		h.invokeTool(w, r, name)
	default:
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (h *Handler) registerTool(w http.ResponseWriter, r *http.Request) {
	ctx, _, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	var def domain.ToolDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest, "invalid JSON body"))
		return
	}
	id, err := h.svc.Register(ctx, def)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": string(id), "name": def.Name, "version": def.Version})
}

func (h *Handler) listTools(w http.ResponseWriter, r *http.Request) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	if q := r.URL.Query().Get("tenant_id"); q != "" {
		tenant = q
	}
	filter := domain.ToolFilter{
		Kind: r.URL.Query().Get("kind"),
		Tags: splitCSV(r.URL.Query().Get("tags")),
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, e := strconv.Atoi(l); e == nil {
			filter.Limit = n
		}
	}
	tools := h.svc.List(ctx, tenant, filter)
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func (h *Handler) deregisterTool(w http.ResponseWriter, r *http.Request, name string) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	id := domain.ToolKey(tenant, name, "*")
	if err := h.svc.Deregister(ctx, id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deregistered", "name": name})
}

func (h *Handler) resolveTool(w http.ResponseWriter, r *http.Request, name string) {
	ctx, tenant, role, err := h.resolveCtxRole(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	ctx = registry.WithRole(ctx, role)
	resolved, err := h.svc.Resolve(ctx, name, tenant)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resolved)
}

func (h *Handler) invokeTool(w http.ResponseWriter, r *http.Request, name string) {
	ctx, tenant, role, err := h.resolveCtxRole(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	ctx = registry.WithRole(ctx, role)
	var input map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}
	out, err := h.svc.Invoke(ctx, name, tenant, input)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": out})
}

func (h *Handler) skillsRoot(w http.ResponseWriter, r *http.Request) {
	if !h.flags.Skills {
		writeErr(w, domain.NewError(domain.ErrNotImplemented, http.StatusNotFound, "skills management is disabled"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.registerSkill(w, r)
	case http.MethodGet:
		h.listSkills(w, r)
	default:
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (h *Handler) registerSkill(w http.ResponseWriter, r *http.Request) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	var sk domain.Skill
	if err := json.NewDecoder(r.Body).Decode(&sk); err != nil {
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest, "invalid JSON body"))
		return
	}
	if sk.TenantID == "" {
		sk.TenantID = tenant
	}
	id, err := h.svc.RegisterSkill(ctx, sk)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": sk.Name, "version": sk.Version})
}

func (h *Handler) listSkills(w http.ResponseWriter, r *http.Request) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	version := r.URL.Query().Get("version")
	writeJSON(w, http.StatusOK, map[string]any{"skills": h.svc.ListSkills(ctx, tenant, version)})
}

func (h *Handler) rulesRoot(w http.ResponseWriter, r *http.Request) {
	if !h.flags.Rules {
		writeErr(w, domain.NewError(domain.ErrNotImplemented, http.StatusNotFound, "rules management is disabled"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.registerRule(w, r)
	case http.MethodGet:
		h.listRules(w, r)
	default:
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (h *Handler) registerRule(w http.ResponseWriter, r *http.Request) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	var ru domain.Rule
	if err := json.NewDecoder(r.Body).Decode(&ru); err != nil {
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest, "invalid JSON body"))
		return
	}
	if ru.TenantID == "" {
		ru.TenantID = tenant
	}
	id, err := h.svc.RegisterRule(ctx, ru)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": ru.Name})
}

func (h *Handler) listRules(w http.ResponseWriter, r *http.Request) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": h.svc.ListRules(ctx, tenant)})
}

func (h *Handler) specsRoot(w http.ResponseWriter, r *http.Request) {
	if !h.flags.Specs {
		writeErr(w, domain.NewError(domain.ErrNotImplemented, http.StatusNotFound, "specs management is disabled"))
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.registerSpec(w, r)
	case http.MethodGet:
		h.listSpecs(w, r)
	default:
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (h *Handler) registerSpec(w http.ResponseWriter, r *http.Request) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	var sp domain.Spec
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		writeErr(w, domain.NewError(domain.ErrInvalidRequest, http.StatusBadRequest, "invalid JSON body"))
		return
	}
	if sp.TenantID == "" {
		sp.TenantID = tenant
	}
	id, err := h.svc.RegisterSpec(ctx, sp)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": sp.Name})
}

func (h *Handler) listSpecs(w http.ResponseWriter, r *http.Request) {
	ctx, tenant, err := h.resolveCtx(r)
	if err != nil {
		writeErr(w, domain.NewError(domain.ErrUnauthorized, http.StatusUnauthorized, err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"specs": h.svc.ListSpecs(ctx, tenant)})
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	if h.metricRec == nil {
		_, _ = w.Write([]byte("# tool-registry metrics (no recorder)\n"))
		return
	}
	var b strings.Builder
	b.WriteString("# tool-registry call metrics\n")
	for _, m := range h.metricRec.Snapshot() {
		b.WriteString("# HELP tool_calls_total total invocations\n")
		b.WriteString("# TYPE tool_calls_total counter\n")
		b.WriteString("tool_calls_total{tool=\"" + m.ToolName + "\"} " + strconv.FormatInt(m.Calls, 10) + "\n")
		b.WriteString("# HELP tool_call_failures_total failed invocations\n")
		b.WriteString("# TYPE tool_call_failures_total counter\n")
		b.WriteString("tool_call_failures_total{tool=\"" + m.ToolName + "\"} " + strconv.FormatInt(m.Failures, 10) + "\n")
		b.WriteString("# HELP tool_call_avg_latency_ms average latency\n")
		b.WriteString("# TYPE tool_call_avg_latency_ms gauge\n")
		b.WriteString("tool_call_avg_latency_ms{tool=\"" + m.ToolName + "\"} " + strconv.FormatInt(m.AvgMs, 10) + "\n")
	}
	_, _ = w.Write([]byte(b.String()))
}

// resolveCtx returns a context carrying the tenant (from auth). Role is omitted.
func (h *Handler) resolveCtx(r *http.Request) (context.Context, string, error) {
	ctx, tenant, _, err := h.resolveCtxRole(r)
	return ctx, tenant, err
}

func (h *Handler) resolveCtxRole(r *http.Request) (context.Context, string, string, error) {
	tenant, role, err := h.auth.Resolve(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Tenant-Id"))
	if err != nil || tenant == "" {
		return r.Context(), "", "", err
	}
	ctx := registry.WithTenant(r.Context(), tenant)
	return ctx, tenant, role, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := string(domain.ErrUpstream)
	msg := err.Error()
	if re, ok := err.(*domain.RegistryError); ok {
		status = re.Status
		code = string(re.Code)
		msg = re.Message
	}
	var body errBody
	body.Error.Code = code
	body.Error.Message = msg
	writeJSON(w, status, body)
}
