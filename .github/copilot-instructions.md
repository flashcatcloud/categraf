Repository Onboarding Instructions for Copilot Coding Agent

Purpose

Enable Copilot to efficiently review PRs and answer Issues using repository-grounded knowledge, while building, testing, linting, and validating changes reliably.
Trust these instructions first; only search when information is missing or provably incorrect.
High-Level Details

Summary
Categraf is a high-performance, metrics/logs/traces collector written in Go. It collects metrics/logs/traces from multiple sources (exporters, system/procfs, services), applies configurations, and forwards data to backends used by prometheus-like ecosystems.
Repository type and scale
Project type: single-service/agent with multiple input modules and plugins.
Size: medium-to-large Go codebase with modular inputs, configs, and packaging assets.
Languages, frameworks, runtimes
Language: Go (modules).
Target runtime: Go 1.20+ (verify go.mod for exact version); builds as a single static-ish binary per OS/arch.
Entry points and artifacts
Main entry: main.go.
Build artifacts: categraf (local); release archives and checksums via GoReleaser (dist/*).
Documentation
Primary docs: README.md; configs and sample configs under conf/ or similar; CHANGELOG/RELEASE notes via GoReleaser.
Build and Validation Instructions
Follow this canonical sequence for reproducibility. Start from a clean environment.

Environment prerequisites

OS: Linux/macOS recommended; Windows possible via Go toolchain.
Tooling:
Go: use the version pinned in go.mod (go env GOVERSION or cat go.mod). If unsure, prefer Go 1.23.x.
Git, Make, Bash.
Optional: Docker and goreleaser for release simulation.
Environment variables:
Do not require secrets for local build/test.
For cross-compilation or CGO, set GOOS/GOARCH/CGO_ENABLED as needed.
Bootstrap (fresh checkout)

Ensure submodules (if any): git submodule update --init --recursive
Clean caches:
go clean -modcache
rm -rf ./categraf ./dist
Download dependencies:
go mod download
Build

Default local build:
go build main.go
go build -o categraf main.go
Static build (if needed):
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o categraf main.go
Postconditions:
Binary present at categraf and runs with --help or version flag if available.
Test

Donot run unit tests for all packages:

Order:
Bootstrap → 2) Build (optional)

Do not do lint and formatting

Formatting:
go fmt ./...
go vet ./...
If golangci-lint is configured (check .golangci.yml):
golangci-lint run
Donot run fmt/vet (and golangci-lint if present) before pushing.
Run (local)

Typical:
./categraf --help
Provide config path if required, e.g., ./categraf --configs conf/categraf.toml (verify actual flag and path in README/conf/)
Docker (if provided):
docker build -f docker/Dockerfile.goreleaser -t flaschcatcloud/categraf:local .
docker run --rm -it categraf:local --help
Release simulation (optional, for validation)

GoReleaser dry-run (requires GoReleaser installed):
goreleaser release --clean --snapshot
Artifacts appear under dist/ without publishing.
Notes:
CI uses GoReleaser to build/publish multi-platform binaries and checksums.
Do not commit version tags during PR; CI release is tag-driven.
Validation and CI

GitHub Actions workflows:
.github/workflows/*.yml define build/test/lint and release (tag-triggered) via GoReleaser.
Required checks before merge typically include: build, test (and lint if configured). Ensure they pass locally with the commands above.
No introduction of platform-specific breakage; avoid CGO unless intended.
Project Layout and Architecture

Key directories
main.go: main entry and CLI wiring.
inputs/: core logic, inputs, processors, outputs (verify exact paths).
conf/ or etc/: default configuration files and sample configs.
scripts/: helper scripts for build or packaging (if present).
.github/: workflows, issue/PR templates, CODEOWNERS, copilot-instructions.md.
Key files
go.mod, go.sum: module management and Go version.
.golangci.yml (if present): linter configuration.
.goreleaser.yml: release configuration (targets, ldflags, archives).
docker/ (if present): directory for containerization.
README.md: build/run instructions and configuration overview.
Architectural notes
Modular inputs/collectors pattern common in agent design:
inputs/: metric collectors for specific services or system sources.
Dependency guideline:
Keep collectors decoupled; share common code via internal/pkg utility packages.
Avoid circular dependencies between inputs and core.
PR Review Behavior for Copilot

Triage
Check CI status. If red, analyze failure logs first (build/test/lint).
Review diffs under cmd/, internal/pkg/, inputs/, and configurations for behavior changes.
Code quality checks
Build correctness: code compiles with the pinned Go version; no unused imports; no build tags accidentally altered.
Concurrency: correct use of context, goroutines, channels; avoid goroutine leaks; ensure proper shutdown on signals.
Error handling: return wrapped errors (fmt.Errorf with %w), no silent ignores, log levels appropriate.
Performance: avoid unnecessary allocations in hot paths; prefer preallocated buffers; careful with time.Ticker/Timer leaks.
Configuration: new options documented in README/conf and have sane defaults; no breaking changes without migration notes.
Observability: meaningful logs; metrics names stable; avoid label cardinality explosion.
Security: do not hardcode secrets; validate input; safe file and network operations.
Validation actions
If touching release config: verify .goreleaser.yml changes maintain expected artifacts.
If adding new input/collector: verify it is wired into build tags or registries as required, and add minimal tests.
Issue Answering Behavior for Copilot

Base answers on concrete repository files:
Cite exact paths like /main.go, inputs/<name>/..., conf/..., .goreleaser.yml, .github/workflows/....
For build/run issues, respond with the canonical sequences in this document.
For config questions, point to the relevant conf/* sample and flags in main/README.
If symbol/path is not found:
State the uncertainty and suggest precise locations to check (e.g., inputs/, internal/, pkg/, conf/).
Avoid fabricating APIs or paths.
Search Minimization Policy

Use this document first. Search only when:
A command here fails or a file/dir does not exist, or
You need a specific symbol/implementation detail.
When searching, prioritize:
README.md and conf/ (for configs and flags)
go.mod, .goreleaser.yml, .golangci.yml
.github/workflows/**
categraf and core packages (api/, pkg/, inputs/)
scripts/
Prefer exact package/path lookup over broad grep.
Common Errors and Workarounds

Go version mismatch:
Align to the version in go.mod (use goenv/asdf or install exact Go toolchain).
CGO build failures:
Set CGO_ENABLED=0 for pure-Go build unless a specific input requires CGO.
Race/build flakiness:
Use -race locally to surface data races; ensure proper context cancellation and goroutine joins.
GoReleaser snapshot issues:
Run goreleaser release --clean --snapshot; if module replaced locally, check go.mod replace directives.
Repository Root Quick Map

README.md: overview, build/run, configuration reference.
go.mod / go.sum: dependencies and Go version.
.goreleaser.yml: release targets and archives/checksums.
.golangci.yml (if present): lint rules.
.github/workflows/*.yml: CI pipelines (build/lint/release).
main.go: main entry.
inputs/, internal/, pkg/: core logic and collectors.
conf/: sample/default configs.
scripts/: tooling helpers (if present).
Final Notes

Before proposing changes or answering issues, mentally simulate: bootstrap → build → run.
Keep changes minimal.
If you discover a reliable sequence or fix not captured here, append it to this file for future agents.
