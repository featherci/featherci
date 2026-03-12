# AGENTS.md

## Documentation Requirements

Every user-facing feature change MUST update documentation before the task is considered complete:

1. **README.md** (root) — Update the workflow reference table and add examples if applicable
2. **docs/docs/reference.html** — Update the Step Fields table with type, default, and description
3. **docs/docs/guides.html** — Add a usage section with a YAML example if the feature warrants explanation
4. **docs/docs/deployment.html** — Update if the feature affects deployment or infrastructure configuration

Do NOT wait to be asked. Documentation updates are part of the implementation, not a follow-up task.

## Test Schema Consistency

The test database schema in `internal/models/build_test.go` uses an inline `CREATE TABLE` statement (not migrations). When adding columns to `build_steps` or other tables:

1. Add the column to the migration file (e.g., `internal/database/migration_NNN_*.go`)
2. **Also** add the column to the test schema in `internal/models/build_test.go`

Both must stay in sync or model tests will fail.

## Build Step Feature Checklist

When adding a new field/option to workflow steps, touch all of these:

1. `internal/workflow/types.go` — Add field to `Step` struct (yaml tag)
2. `internal/models/build_step.go` — Add field to `BuildStep` struct (db tag)
3. `internal/models/build_step.go` — Add to `Create()` and `CreateBatch()` INSERT queries
4. `internal/services/build_creator.go` — Map workflow field to model field in `createSteps()`
5. `internal/executor/runner.go` — Use the field in `RunStep()` if it affects execution
6. `internal/database/migration_NNN_*.go` — Add DB migration
7. `internal/models/build_test.go` — Update test schema
8. Documentation (see above)

## Verification

- `go build ./...` must compile
- `go test ./internal/...` must pass
- Spot-check that the new feature flows from YAML parse → DB storage → executor usage
