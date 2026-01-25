# Bugs

This file tracks previously reported issues and their current status.

## Resolved
- Missing `normalizeWorkspace` function in store package. Fixed in `internal/store/workspace.go`.
- `init` welcome memory errors were swallowed. Now non-constraint errors return in `internal/app/init.go`.
- Dead health-check path removed in `internal/health/health.go`.
- Inconsistent chunk workspace normalization in `AddArtifactWithChunks`. Now defaults to the artifact workspace unless a chunk workspace is explicitly set in `internal/store/records.go`.
- `SchemaVersion()` export missing. Added in `internal/store/health.go`.
- Potential nil `RepoCache` map. Defaults and `Load()` now ensure non-nil in `internal/config/config.go`.

## Open / Low-Risk
- Repo cache updates are not atomic across concurrent CLI processes. `Save()` now merges existing cache entries, but a lock/atomic update is still not implemented.

## Mitigated / Watch
- FTS query injection risk: queries are now length-validated, tokenized into AND + NEAR, and escaped via `formatFTSVariant` in `internal/store/memory.go`. Keep an eye on future operator handling or prefix expansions, but no known exploit paths remain.
