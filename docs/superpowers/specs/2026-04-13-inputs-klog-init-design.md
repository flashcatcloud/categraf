# Inputs Klog Initialization Design

## Goal

Standardize `inputs` initialization logging by passing a shared `klog` logger into plugin and instance initialization, while preserving compatibility for existing plugins that still implement `Init() error`.

## Context

`MetricsAgent.inputGo` currently initializes inputs and instances through `inputs.MayInit`, and both the agent and plugins emit logs through mixed mechanisms. The initialization path does not carry structured logger context, which makes it hard to normalize fields like plugin name, checksum, and instance identity.

## Approaches Considered

### 1. Explicit logger injection in `InitWithLogger(logger)` with compatibility fallback

Add a new logger-aware initializer interface in `inputs`, change `MayInit` to accept a logger, and let it call either `InitWithLogger(klog.Logger) error` or legacy `Init() error`.

This keeps logger dependency explicit, allows per-plugin and per-instance child loggers, and avoids a flag day migration across all plugins.

### 2. Two-phase injection with `SetLogger(logger)` plus existing `Init()`

Inject the logger before `Init()` through a secondary interface.

This reduces signature churn but weakens lifecycle guarantees because logger setup and initialization become separate calls.

### 3. Global logger only via `klog.SetLogger(...)`

Rely on package-global logging and let plugins fetch a logger implicitly.

This is the smallest change, but it does not make initialization dependencies explicit and does not naturally carry per-plugin or per-instance context from the caller.

## Decision

Use approach 1.

Introduce a new logger-aware initializer interface and update the initialization path to pass derived `klog` loggers with stable context fields. Keep legacy `Init() error` support so unchanged plugins continue to work.

## Design

### Interface changes

In `inputs/inputs.go`:

- Add `type KlogInitializer interface { InitWithLogger(klog.Logger) error }`
- Change `MayInit` to `func MayInit(t interface{}, logger klog.Logger) error`
- `MayInit` should prefer `KlogInitializer`; if unavailable, it should fall back to the current `Initializer`

This preserves backwards compatibility while creating a single framework entrypoint for initialization logging.

### Logger derivation

In `agent/metrics_agent.go`:

- Create a root logger for the metrics agent initialization flow
- Derive an input logger with fields such as:
  - `component=inputs`
  - `input=<full input name>`
  - `plugin=<parsed input key>`
  - `checksum=<config checksum>`
- Derive an instance logger from the input logger for each instance with:
  - `instance_index=<index>`
  - optional identifying labels when available and cheap to compute

Framework logs emitted during initialization should use these structured loggers instead of the current plain `log.Println` calls in the touched path.

### Plugin migration model

No immediate repo-wide migration is required.

- Existing plugins that only implement `Init() error` continue to work through the compatibility branch in `MayInit`
- Plugins that need structured initialization logs can opt into `InitWithLogger(klog.Logger) error`
- The framework remains the single place where context is attached

### Error handling

- Preserve current initialization behavior and error semantics
- Continue special-casing `types.ErrInstancesEmpty`
- Do not silently swallow logger-aware initializer errors
- Avoid introducing expensive reflection or plugin-specific heuristics for instance identity

## Testing

Add focused unit tests for `inputs.MayInit`:

- logger-aware initializer is preferred when both interfaces are present
- legacy initializer still works
- non-initializer returns `nil`
- errors propagate unchanged

Add focused tests for `MetricsAgent.inputGo` or the smallest practical extraction around it:

- input initialization receives a logger with plugin/input context
- instance initialization receives a derived logger
- initialization failure paths still stop startup as before

## Files Expected To Change

- `inputs/inputs.go`
- `inputs/inputs_test.go` or equivalent new test file
- `agent/metrics_agent.go`
- `agent/metrics_agent_test.go` or equivalent new test file

## Non-Goals

- Converting all plugin runtime logging to `klog` in this change
- Rewriting unrelated non-input logging paths
- Adding a new logging abstraction beyond `klog`
