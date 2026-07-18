# ADR-0003: Sandbox binding timing of code class Tool

- **Status**: Pending (Open)
- **Date**: 2026-07-17
- **Suggested by**: OpenStrata Architecture Group
- **Repository**: ai-tool-registry
- **Source**: `docs/DESIGN.md` §14 Open Issue
- **Association**: `ai-sandbox-manager`

##Context

Does declaring `kind: code` when registering force `ai-sandbox-manager` to be online (§10.6 Dependency Rules)? Weak dependence or strong dependence?

## Decision Options (Options Considered)

1. **Maintain status quo / conservative default**: Maintain current behavior, controlled by configuration switches or explicit parameters, and do not introduce destructive changes.
2. **Unified implementation after cross-repository alignment**: Make a clear contract with the relevant service (`ai-sandbox-manager`) before implementation.
3. **Phased introduction**: Leave a placeholder/default switch in the current stage, and solidify it in subsequent stages after the dependent capabilities are ready (see Related Architecture §).

## Recommended decision (Decision)

This ADR solidifies the "sandbox binding timing of the code class Tool" into an architectural decision record and incorporates it into `docs/adr/` for continuous tracking. This issue stems from the `docs/DESIGN.md` §14 open issue and is still open.

**Conservative Default Principle**: Before the final decision is made, the "minimum available + explicit configuration switch" shall prevail, maintain the current behavior, and not destroy the existing contract and cross-repository SPI interface; this ADR status will be written back after review by the relevant team.



## To be aligned / Follow-ups (Follow-ups)

- Alignment confirmation with `ai-sandbox-manager`: clarify responsibility boundaries/interface contracts/data flow direction to avoid double writing or semantic drift.
- Associated architecture documents §10.6 (as a basis for decision-making and a source of consistency verification).
- Solidify the decision before the review at the corresponding stage, and write the final conclusion back into this ADR (the status is changed from "Pending" to "Adopted").

## Traceback

- Upstream design: `docs/DESIGN.md` §14 Open issue
- Relevance index: see `docs/adr/README.md`
