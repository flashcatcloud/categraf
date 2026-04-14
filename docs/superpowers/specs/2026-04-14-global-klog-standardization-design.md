# Global Klog Standardization Design

## Summary

Standardize `/Users/ruochen/workspace/youzan/categraf` on `k8s.io/klog/v2` as the primary repository logging surface. Replace repository-owned `log.Println` / `log.Printf` call sites with leveled `klog` logging, remove `DebugMode` branches whose only purpose is local debug log emission, and update tests plus documentation so the repository no longer teaches or preserves the legacy standard-library logging path.

## Goals

- Eliminate repository-owned `log.Println` and `log.Printf` usage from runtime code, tests, and docs.
- Remove `if config.Config.DebugMode { ... }` branches when they only gate local debug logging.
- Route informational, warning, error, and debug output through a shared `klog` policy.
- Keep external behavior stable apart from log formatting and verbosity control.
- Leave non-logging business logic unchanged.

## Non-Goals

- No unrelated refactors of collector logic, agent lifecycle, or writer behavior.
- No redesign of config structure beyond what is needed to map existing debug semantics onto `klog` verbosity.
- No changes to third-party code, vendored code, or generated code unless a repository-owned generated artifact clearly needs documentation cleanup.

## Scope

The migration applies to the full repository surface requested by the user:

- Runtime packages such as `agent`, `writer`, `inputs`, `heartbeat`, `config`, `parser`, `api`, `pkg`, and related repository-owned packages.
- Test files that currently demonstrate or depend on legacy logging style.
- Documentation under `docs/`, including prior implementation plans that show the old style.

If a `DebugMode` field controls downstream library behavior rather than just local logging, preserve that behavioral control and only remove the local log-only branch.

## Existing Context

The repository already contains an uncommitted plan under `docs/superpowers/plans/2026-04-13-global-klog-logging.md` plus logging-related test scaffolding. Current runtime call sites still include many `log.Println(...)` usages and several `config.Config.DebugMode` branches in files such as:

- `agent/metrics_agent.go`
- `agent/metrics_reader.go`
- `writer/writers.go`
- `inputs/http_provider.go`

`go.mod` already carries `k8s.io/klog/v2`, so the migration can standardize on the existing dependency rather than introducing a second logger.

## Logging Policy

### Primary API

Use `k8s.io/klog/v2` directly or through the repository's shared logging package if that package is already being introduced for initialization and policy enforcement.

### Level Mapping

- Normal lifecycle and status messages: `klog.InfoS(...)` or `klog.Infof(...)`
- Warnings: `klog.Warningf(...)`
- Errors with structured context: prefer `klog.ErrorS(err, msg, kv...)`
- Errors without a clean error object: `klog.Errorf(...)`
- Debug output previously guarded by `DebugMode`: `klog.V(1).InfoS(...)`
- Very noisy collector or request/response tracing: `klog.V(2).InfoS(...)`

### Formatting Rule

Do not preserve legacy `I!`, `W!`, `E!`, `D!` prefixes inside the message text unless a call site is intentionally preserving exact output for compatibility. `klog` level selection should carry the severity.

### DebugMode Rule

Replace log-only `DebugMode` gates with verbosity-driven logging. Example:

```go
klog.V(1).InfoS("before gather once", "input", r.inputName)
```

instead of:

```go
if config.Config.DebugMode {
	log.Println("D!", r.inputName, ": before gather once")
}
```

Where `DebugMode` controls printing of metrics or other operational behavior beyond logging, keep that behavior and only change the local logging expression.

## Architecture

### Shared Initialization

The repository should have one place that configures `klog`, bridges legacy standard-library logging if transitional support is still needed, and maps existing debug configuration onto `klog` verbosity.

Responsibilities of the shared logging setup:

- register `klog` flags
- choose output target consistently
- map existing debug configuration to a minimum verbosity level
- flush logs on shutdown
- optionally bridge `log` package output during migration, but not preserve `log.Println` as an accepted final repository pattern

### Call Site Migration Strategy

Apply migration in repository-owned packages in place rather than building a large compatibility wrapper for every old call pattern. The main work is mechanical but should still be reviewed per file for:

- severity choice
- structured fields worth extracting
- removal of stale `I!/W!/E!/D!` string prefixes
- preservation of non-logging behavior in `DebugMode` branches

### Policy Enforcement

Add or extend repository policy tests to fail when forbidden patterns remain in the intended scope. The policy should at minimum detect:

- `log.Println` / `log.Printf` in repository-owned code paths under migration
- `if config.Config.DebugMode {` branches that exist only to emit logs in the enforced scope

## File-Level Design

### Shared Logging Package

If `pkg/logging` is being introduced, it should own:

- `klog` flag registration
- configuration and output wiring
- standard-library bridging during transition
- flush lifecycle helpers
- repository policy tests

If equivalent initialization already exists elsewhere, prefer consolidating there rather than duplicating logging setup.

### Runtime Packages

Representative transformations:

- `agent/metrics_reader.go`: replace pre/post gather debug branches with `klog.V(1)` and panic logging with `klog.ErrorS` or `klog.Errorf`
- `agent/metrics_agent.go`: replace startup, init, unsupported input, and no-instance messages with `klog` and remove log-only debug branches
- `writer/writers.go`: keep `TestMode` behavior, preserve metric printing semantics, but move queue/debug logs to `klog`
- `inputs/http_provider.go`: convert request/response debug branches to verbosity-based logging
- `heartbeat/heartbeat.go` and similar packages: convert legacy prefixed messages to severity-appropriate `klog`

### Tests

Tests should validate two things:

- logging configuration and verbosity behavior work as intended
- forbidden legacy patterns are absent from enforced repository scope

Existing untracked tests such as `agent/metrics_agent_test.go` and `inputs/inputs_test.go` should be reviewed and aligned with the final logging policy rather than left half-integrated.

### Documentation

Update repository docs and plan files that still demonstrate `log.Println`, `log.Printf`, or `DebugMode`-guarded logging. Documentation should reflect the canonical `klog` style so future contributors do not reintroduce the old pattern.

## Error Handling

The migration should not silence errors. When replacing `log.Println("E! ...", err)`, preserve the original operational context and include key identifiers as structured fields where practical. If the original code logged and returned, the new code must still log and return. If the original code logged inside a recover block, the new code must preserve that recover path.

## Testing Strategy

1. Add or finish focused tests for shared logging configuration.
2. Add or finish repository policy tests covering the first enforced scope.
3. Run package-level `go test` for directly modified packages.
4. Run repository-wide `rg` verification for forbidden legacy patterns.
5. Expand policy scope if needed once the first migration batch is stable.

## Risks And Mitigations

### Risk: Mechanical replacement changes semantics

Mitigation: Review each `DebugMode` branch to distinguish pure logging from real behavior control before deleting the branch.

### Risk: Inconsistent severity mapping

Mitigation: Use a simple, repo-wide mapping and normalize legacy prefix-based messages into `klog` levels.

### Risk: Partial migration leaves repo in mixed state

Mitigation: Use repository policy tests plus final `rg` sweeps on runtime code, tests, and docs.

### Risk: Untracked local work overlaps with migration

Mitigation: Avoid reverting existing untracked tests/docs; incorporate them where they match the approved direction.

## Success Criteria

- Repository-owned `log.Println` / `log.Printf` call sites targeted by the migration are replaced with `klog`.
- Log-only `DebugMode` branches are removed in favor of verbosity-based `klog` calls.
- Tests and docs no longer demonstrate the legacy logging style.
- Modified packages pass targeted `go test` runs.
- Final repository sweeps confirm the targeted legacy patterns are gone from code, tests, and docs.
