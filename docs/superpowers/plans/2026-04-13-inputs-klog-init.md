# Inputs Klog Init Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a logger-aware `inputs` initialization path that passes a shared `klog` logger with plugin and instance context during input startup, while keeping legacy `Init() error` plugins working unchanged.

**Architecture:** Extend the `inputs` framework with a logger-aware initializer interface and route all initialization through `inputs.MayInit(t, logger)`. In `MetricsAgent.inputGo`, build structured child loggers for the input and each instance, use them for framework initialization logs, and pass them into `MayInit`. Keep compatibility by calling `InitWithLogger(klog.Logger)` when available and falling back to legacy `Init() error` otherwise.

**Tech Stack:** Go 1.25, `k8s.io/klog/v2`, existing `agent` and `inputs` packages, `go test`

---

### Task 1: Create the Branch And Lock Down `inputs` Initializer Behavior With Tests

**Files:**
- Modify: `inputs/inputs.go`
- Create: `inputs/inputs_test.go`

- [ ] **Step 1: Create the feature branch from the current local `main`**

Run:

```bash
git switch -c feat/inputs-klog-init
```

Expected: Git reports a new branch named `feat/inputs-klog-init`.

- [ ] **Step 2: Write the failing tests for logger-aware and legacy initializer dispatch**

Add `inputs/inputs_test.go` with focused table-free tests:

```go
package inputs

import (
	"errors"
	"testing"

	"k8s.io/klog/v2"
)

type legacyInitializer struct {
	called bool
	err    error
}

func (i *legacyInitializer) Init() error {
	i.called = true
	return i.err
}

type klogInitializer struct {
	called bool
	logger klog.Logger
	err    error
}

func (i *klogInitializer) InitWithLogger(logger klog.Logger) error {
	i.called = true
	i.logger = logger
	return i.err
}

func TestMayInitPrefersKlogInitializer(t *testing.T) {}
func TestMayInitFallsBackToLegacyInitializer(t *testing.T) {}
func TestMayInitReturnsNilForNonInitializer(t *testing.T) {}
func TestMayInitPropagatesInitializerErrors(t *testing.T) {}
```

- [ ] **Step 3: Run the new `inputs` tests to verify RED**

Run:

```bash
go test ./inputs -run TestMayInit -count=1
```

Expected: FAIL because `MayInit` does not yet accept a logger or dispatch to the new interface.

- [ ] **Step 4: Implement the minimal `inputs` framework change**

Update `inputs/inputs.go` to introduce the new interface and logger-aware dispatch:

```go
package inputs

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

type Initializer interface {
	Init() error
}

type KlogInitializer interface {
	InitWithLogger(klog.Logger) error
}

func MayInit(t interface{}, logger klog.Logger) error {
	if initializer, ok := t.(KlogInitializer); ok {
		return initializer.InitWithLogger(logger)
	}
	if initializer, ok := t.(Initializer); ok {
		return initializer.Init()
	}
	return nil
}
```

- [ ] **Step 5: Update the tests to use the final interface and verify GREEN**

Complete `inputs/inputs_test.go` so it checks:

```go
func TestMayInitPrefersKlogInitializer(t *testing.T) {
	logger := klog.Background().WithName("inputs-test")
	initializer := &testKlogInitializer{}

	if err := MayInit(initializer, logger); err != nil {
		t.Fatalf("MayInit() error = %v", err)
	}

	if !initializer.called {
		t.Fatal("expected logger-aware initializer to be called")
	}
	if initializer.logger != logger {
		t.Fatal("expected logger to be passed through unchanged")
	}
}
```

Run:

```bash
go test ./inputs -run TestMayInit -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the isolated framework dispatch change**

Run:

```bash
git add inputs/inputs.go inputs/inputs_test.go
git commit -m "test: cover inputs logger-aware init dispatch"
```

Expected: A commit containing only the `inputs` dispatch and tests.

### Task 2: Add Metrics Agent Tests For Input And Instance Logger Context

**Files:**
- Modify: `agent/metrics_agent.go`
- Create: `agent/metrics_agent_test.go`

- [ ] **Step 1: Write the failing metrics agent tests around initialization logger propagation**

Add `agent/metrics_agent_test.go` with a fake input, fake provider-free agent setup, and logger-aware fake input/instance types:

```go
package agent

import (
	"testing"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

type fakeInput struct {
	initLogger     klog.Logger
	instanceLogger klog.Logger
	instances      []inputs.Instance
}

func (f *fakeInput) Clone() inputs.Input                                { return f }
func (f *fakeInput) Name() string                                       { return "fake" }
func (f *fakeInput) GetLabels() map[string]string                       { return nil }
func (f *fakeInput) GetInterval() config.Duration                       { return 0 }
func (f *fakeInput) InitInternalConfig() error                          { return nil }
func (f *fakeInput) Process(s *types.SampleList) *types.SampleList      { return s }
func (f *fakeInput) InitWithLogger(logger klog.Logger) error            { f.initLogger = logger; return nil }
func (f *fakeInput) GetInstances() []inputs.Instance                    { return f.instances }

type fakeInstance struct {
	initLogger  klog.Logger
	initialized bool
}

func (f *fakeInstance) Initialized() bool                               { return f.initialized }
func (f *fakeInstance) SetInitialized()                                 { f.initialized = true }
func (f *fakeInstance) GetLabels() map[string]string                    { return map[string]string{"target": "demo"} }
func (f *fakeInstance) GetIntervalTimes() int64                         { return 1 }
func (f *fakeInstance) InitInternalConfig() error                       { return nil }
func (f *fakeInstance) Process(s *types.SampleList) *types.SampleList   { return s }
func (f *fakeInstance) InitWithLogger(logger klog.Logger) error         { f.initLogger = logger; return nil }

func TestMetricsAgentInputGoPassesLoggerToInputAndInstance(t *testing.T) {}
func TestMetricsAgentInputGoKeepsErrInstancesEmptyBehavior(t *testing.T) {}
```

The final test should assert logger context through production-owned helper functions that return the key-value pairs used to build the logger.

- [ ] **Step 2: Run the agent tests to verify RED**

Run:

```bash
go test ./agent -run TestMetricsAgentInputGo -count=1
```

Expected: FAIL because `MetricsAgent.inputGo` does not yet construct or pass structured `klog` loggers.

- [ ] **Step 3: Introduce the smallest production helper needed for deterministic logger testing**

In `agent/metrics_agent.go`, add narrow helpers instead of embedding all logger derivation inline:

```go
func inputInitLoggerValues(name string, sum string) []interface{} {
	_, inputKey := inputs.ParseInputName(name)
	return []interface{}{"input", name, "plugin", inputKey, "checksum", sum}
}

func newInputInitLogger(name string, sum string) klog.Logger {
	return klog.Background().
		WithName("inputs").
		WithValues(inputInitLoggerValues(name, sum)...)
}

func instanceInitLoggerValues(idx int, labels map[string]string) []interface{} {
	values := []interface{}{"instance_index", idx}
	if target, ok := labels["target"]; ok && target != "" {
		values = append(values, "instance_target", target)
	}
	return values
}

func newInstanceInitLogger(logger klog.Logger, idx int, labels map[string]string) klog.Logger {
	return logger.WithValues(instanceInitLoggerValues(idx, labels)...)
}
```

Keep the helper small and stable so tests can verify semantics without poking at unrelated agent state.

- [ ] **Step 4: Update `inputGo` to use the logger helpers and `inputs.MayInit(..., logger)`**

Wire the initialization path like this:

```go
func (ma *MetricsAgent) inputGo(name string, sum string, input inputs.Input) {
	inputLogger := newInputInitLogger(name, sum)

	if err := input.InitInternalConfig(); err != nil {
		inputLogger.Error(err, "failed to init input internal config")
		return
	}

	if err := inputs.MayInit(input, inputLogger); err != nil {
		// preserve ErrInstancesEmpty behavior
		return
	}

	instances := inputs.MayGetInstances(input)
	for i := 0; i < len(instances); i++ {
		instanceLogger := newInstanceInitLogger(inputLogger, i, instances[i].GetLabels())
		if err := inputs.MayInit(instances[i], instanceLogger); err != nil {
			// preserve current semantics
			continue
		}
	}
}
```

- [ ] **Step 5: Complete the tests and verify GREEN**

Update `agent/metrics_agent_test.go` to check:

```go
func TestMetricsAgentInputGoPassesLoggerToInputAndInstance(t *testing.T) {
	instance := &fakeInstance{}
	input := &fakeInput{instances: []inputs.Instance{instance}}
	agent := &MetricsAgent{InputReaders: NewReaders()}

	agent.inputGo("local.fake", "sum-1", input)

	if !instance.initialized {
		t.Fatal("expected instance to be marked initialized")
	}

	inputValues := inputInitLoggerValues("local.fake", "sum-1")
	if !reflect.DeepEqual([]interface{}{"input", "local.fake", "plugin", "fake", "checksum", "sum-1"}, inputValues) {
		t.Fatalf("unexpected input logger values: %#v", inputValues)
	}

	instanceValues := instanceInitLoggerValues(0, map[string]string{"target": "demo"})
	if !reflect.DeepEqual([]interface{}{"instance_index", 0, "instance_target", "demo"}, instanceValues) {
		t.Fatalf("unexpected instance logger values: %#v", instanceValues)
	}
}
```

Run:

```bash
go test ./agent -run TestMetricsAgentInputGo -count=1
```

Expected: PASS.

- [ ] **Step 6: Run the targeted package tests together**

Run:

```bash
go test ./inputs ./agent -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the metrics agent logger propagation**

Run:

```bash
git add agent/metrics_agent.go agent/metrics_agent_test.go
git commit -m "feat: pass klog through input initialization"
```

Expected: A commit containing the agent logger propagation and tests.

### Task 3: Verify End-To-End Behavior And Clean Up

**Files:**
- Modify: `agent/metrics_agent.go`
- Modify: `inputs/inputs.go`
- Modify: `agent/metrics_agent_test.go`
- Modify: `inputs/inputs_test.go`

- [ ] **Step 1: Replace touched initialization log statements with structured `klog` logging**

Keep this change scoped to the touched initialization path. For example:

```go
inputLogger.Error(err, "failed to init input")
inputLogger.V(1).Info("no instances for input")
inputLogger.Info("input started")
```

Do not rewrite unrelated files or unrelated agent startup logs in this task.

- [ ] **Step 2: Run focused regression tests**

Run:

```bash
go test ./inputs ./agent -run 'TestMayInit|TestMetricsAgentInputGo' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run broader verification for touched packages**

Run:

```bash
go test ./inputs ./agent -count=1
```

Expected: PASS.

- [ ] **Step 4: Inspect the diff for scope control**

Run:

```bash
git diff -- inputs/inputs.go inputs/inputs_test.go agent/metrics_agent.go agent/metrics_agent_test.go
```

Expected: The diff is limited to logger-aware init interfaces, structured init logging, and tests.

- [ ] **Step 5: Commit the final cleanup if needed**

Run:

```bash
git add inputs/inputs.go inputs/inputs_test.go agent/metrics_agent.go agent/metrics_agent_test.go
git commit -m "refactor: normalize input init logging"
```

Expected: Either no-op if previous commits already captured the exact diff, or a small final cleanup commit.
