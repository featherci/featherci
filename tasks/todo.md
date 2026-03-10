# Step 16: Build Orchestration

## Tasks
- [x] 1. Models: Add `SkipDependentSteps`, `CancelBuildSteps`, `ResetStepsForWorker` to build_step.go
- [x] 2. Models: Add `CancelBuild` to build.go
- [x] 3. Models: Add `ListStale` to worker.go
- [x] 4. Tests: Add tests for all new model methods
- [x] 5. Worker: Add `advanceBuild` helper, integrate skip logic
- [x] 6. Webhook: Wire webhook handler to BuildCreator + dependencies
- [x] 7. Server: Pass new deps to WebhookHandler
- [x] 8. Handler: Create build cancel endpoint
- [x] 9. Routes: Add cancel route
- [x] 10. Main: Add stale worker cleanup goroutine
- [x] 11. Verify: All tests pass, vet clean, builds compile

## Review
- All 17 test suites pass
- `go vet ./...` clean
- `go build ./cmd/featherci/` compiles
