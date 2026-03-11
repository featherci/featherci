# YAML Config File + Homebrew Docs

## Config File Support
- [x] 1. Add `--config` flag to main.go, pass path to config.Load()
- [x] 2. Add YAML config file loading to internal/config/config.go
- [x] 3. Create scripts/config.yaml.example with all options documented
- [x] 4. Add tests for YAML config loading
- [x] 5. Update install.sh to install the example YAML config

## Documentation
- [x] 6. Add Homebrew installation section to docs/docs/index.html
- [x] 7. Update deployment.html systemd section to show YAML config option
- [x] 8. Update Homebrew formula caveats to mention config file
- [x] 9. Add Homebrew to the marketing landing page (tabbed install)

## Verification
- [x] go build ./... — clean
- [x] go vet ./... — clean
- [x] go test ./... — all pass
