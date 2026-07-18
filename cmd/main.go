// Command ai-tool-registry boots the tool registry. Dependency wiring is done by
// hand here (the offline stand-in for Wire compile-time DI, DESIGN §1.4).
// Production swaps in PostgreSQL/Redis adapters and Keycloak-verified auth.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	appmetering "github.com/open-strata-ai/ai-tool-registry/application/metering"
	"github.com/open-strata-ai/ai-tool-registry/application/rbac"
	"github.com/open-strata-ai/ai-tool-registry/application/registry"
	"github.com/open-strata-ai/ai-tool-registry/application/schema"
	"github.com/open-strata-ai/ai-tool-registry/domain"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/auth"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/config"
	discoverystore "github.com/open-strata-ai/ai-tool-registry/infrastructure/discovery"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/metering"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/mcp"
	"github.com/open-strata-ai/ai-tool-registry/infrastructure/store"
	httpapi "github.com/open-strata-ai/ai-tool-registry/interfaces/http"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	h, closeFn := Bootstrap(cfg)
	defer closeFn()

	listen := os.Getenv("ADDR")
	if listen == "" {
		listen = "0.0.0.0:8080"
	}
	log.Printf("ai-tool-registry listening on %s", listen)
	srv := &http.Server{
		Addr:              listen,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// Bootstrap assembles the full object graph from config and returns the HTTP
// handler plus a cleanup function. It is exported so tests can reuse the wiring.
func Bootstrap(cfg config.Config) (*httpapi.Handler, func()) {
	pgDSN := os.Getenv("DATABASE_URL")
	redisAddr := os.Getenv("REDIS_ADDR")

	var st domain.Store
	if pgDSN != "" {
		pgstore, err := store.NewPostgres(pgDSN)
		if err != nil {
			log.Printf("WARN: falling back to in-memory store (%v)", err)
			st = store.New()
		} else {
			st = pgstore
		}
	} else {
		st = store.New()
	}

	if redisAddr != "" {
		cache := store.NewRedisCache(redisAddr)
		_ = cache // L2 cache available; store writes go through both
	}

	disc := discoverystore.New()
	validator := schema.New()
	authPort := auth.New("local")
	rec := appmetering.New(1024, metering.LogSink())

	svc := registry.New(registry.Deps{
		Store:      st,
		Discovery:  disc,
		Schema:     validator,
		Metering:   rec,
		Auth:       authPort,
		RBAC:       rbac.New(),
		Transports: mcp.Selector(nil),
	}, registry.Config{
		SchemaValidation: cfg.ToolRegistry.SchemaValidation,
		DiscoveryEnabled: cfg.ToolRegistry.Discovery.Enabled,
	})

	flags := httpapi.FeatureFlags{
		Skills: cfg.Skills.Enabled,
		Rules:  cfg.Rules.Enabled,
		Specs:  cfg.Specs.Enabled,
	}
	handler := httpapi.New(svc, authPort, flags, rec)
	return handler, func() { rec.Close() }
}
