# Global Klog Standardization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Standardize repository-owned logging on `k8s.io/klog/v2`, remove `log.Println` / `log.Printf` from repository code, tests, and docs, and replace log-only `DebugMode` branches with verbosity-based `klog` output.

**Architecture:** Introduce one shared logging bootstrap under `pkg/logging`, wire it into process startup, then migrate call sites in batches. Use focused package tests, a repository policy test for core files, and final `rg` sweeps to ensure repository-owned runtime code, tests, and docs no longer preserve the legacy logging path.

**Tech Stack:** Go 1.25, `k8s.io/klog/v2`, `flag`, `gopkg.in/natefinch/lumberjack.v2`, existing `agent`, `writer`, `inputs`, `api`, `heartbeat`, `ibex`, `pkg`, `parser`, `config` packages, `go test`, `rg`

---

## File Map

- `pkg/logging/logging.go`: shared `klog` flag registration, output routing, verbosity mapping, stdlib bridge, and flush helper
- `pkg/logging/logging_test.go`: logging bootstrap tests
- `pkg/logging/repository_policy_test.go`: core repository guardrails for runtime logging patterns
- `main.go`, `main_posix.go`, `main_windows.go`: process bootstrap, service command logs, flush on exit
- `agent/*.go`, `writer/*.go`, `heartbeat/*.go`, `config/*.go`: first migration wave for core runtime paths and `DebugMode` cleanup
- `inputs/**/*.go`, `pkg/**/*.go`, `api/**/*.go`, `parser/**/*.go`, `ibex/**/*.go`: second migration wave for repository-owned packages
- `agent/metrics_agent_test.go`, `inputs/inputs_test.go`, any other touched `*_test.go`: tests aligned with the new logger behavior
- `docs/superpowers/plans/*.md`, `docs/superpowers/specs/*.md`, other repo docs that demonstrate old logging: documentation cleanup

### Task 1: Add Shared Logging Bootstrap And First Policy Tests

**Files:**
- Create: `pkg/logging/logging.go`
- Create: `pkg/logging/logging_test.go`
- Create: `pkg/logging/repository_policy_test.go`

- [ ] **Step 1: Write the failing logging bootstrap test**

Create `pkg/logging/logging_test.go`:

```go
package logging

import (
	"bytes"
	"flag"
	stdlog "log"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestConfigureMapsDebugToVerbosity(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()

	fs := flag.NewFlagSet("logging-test", flag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	var buf bytes.Buffer
	if err := configureWithWriter(&buf, fs, true, 0); err != nil {
		t.Fatalf("configureWithWriter() error = %v", err)
	}

	klog.V(1).InfoS("debug enabled")
	klog.Flush()

	if !strings.Contains(buf.String(), "debug enabled") {
		t.Fatalf("expected verbosity output, got %q", buf.String())
	}
}

func TestConfigureBridgesStandardLibraryLog(t *testing.T) {
	state := klog.CaptureState()
	defer state.Restore()

	fs := flag.NewFlagSet("logging-bridge-test", flag.ContinueOnError)
	RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	var buf bytes.Buffer
	if err := configureWithWriter(&buf, fs, false, 0); err != nil {
		t.Fatalf("configureWithWriter() error = %v", err)
	}

	stdlog.Println("legacy bridge message")
	klog.Flush()

	if !strings.Contains(buf.String(), "legacy bridge message") {
		t.Fatalf("expected bridged log output, got %q", buf.String())
	}
}
```

- [ ] **Step 2: Write the failing core policy test**

Create `pkg/logging/repository_policy_test.go`:

```go
package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var forbiddenStdLog = regexp.MustCompile(`\blog\.(Println|Printf|Fatal|Fatalf|Fatalln)\b`)
var forbiddenDebugBranch = regexp.MustCompile(`if\s+config\.Config\.DebugMode\s*\{`)

func TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches(t *testing.T) {
	files := []string{
		filepath.Join("..", "..", "main.go"),
		filepath.Join("..", "..", "main_posix.go"),
		filepath.Join("..", "..", "main_windows.go"),
		filepath.Join("..", "..", "agent", "agent.go"),
		filepath.Join("..", "..", "agent", "metrics_agent.go"),
		filepath.Join("..", "..", "agent", "metrics_reader.go"),
		filepath.Join("..", "..", "writer", "writer.go"),
		filepath.Join("..", "..", "writer", "writers.go"),
		filepath.Join("..", "..", "heartbeat", "heartbeat.go"),
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", file, err)
		}
		if forbiddenStdLog.Match(content) {
			t.Fatalf("forbidden standard log call remains in %s", file)
		}
		if forbiddenDebugBranch.Match(content) {
			t.Fatalf("forbidden DebugMode branch remains in %s", file)
		}
	}
}
```

- [ ] **Step 3: Verify RED**

Run:

```bash
go test ./pkg/logging -count=1
```

Expected: FAIL because the package does not exist yet and the policy test references files still using stdlib logging.

- [ ] **Step 4: Implement the shared logging bootstrap**

Create `pkg/logging/logging.go`:

```go
package logging

import (
	"flag"
	"io"
	stdlog "log"
	"os"
	"strconv"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"k8s.io/klog/v2"
)

var flushOnce sync.Once

func RegisterFlags(fs *flag.FlagSet) {
	klog.InitFlags(fs)
}

func Configure(output string, maxSize, maxAge, maxBackups int, localTime, compress, debug bool, debugLevel int) error {
	return configureWithWriter(newWriter(output, maxSize, maxAge, maxBackups, localTime, compress), flag.CommandLine, debug, debugLevel)
}

func configureWithWriter(writer io.Writer, fs *flag.FlagSet, debug bool, debugLevel int) error {
	level := debugLevel
	if debug && level == 0 {
		level = 1
	}

	if err := fs.Set("logtostderr", "false"); err != nil {
		return err
	}
	if err := fs.Set("alsologtostderr", "false"); err != nil {
		return err
	}
	if err := fs.Set("stderrthreshold", "FATAL"); err != nil {
		return err
	}
	if err := fs.Set("v", strconv.Itoa(level)); err != nil {
		return err
	}

	stdlog.SetFlags(0)
	klog.SetOutput(writer)
	klog.CopyStandardLogTo("INFO")
	flushOnce.Do(func() {
		klog.StartFlushDaemon(5 * time.Second)
	})
	return nil
}

func newWriter(output string, maxSize, maxAge, maxBackups int, localTime, compress bool) io.Writer {
	switch output {
	case "", "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	default:
		return &lumberjack.Logger{
			Filename:   output,
			MaxSize:    maxSize,
			MaxAge:     maxAge,
			MaxBackups: maxBackups,
			LocalTime:  localTime,
			Compress:   compress,
		}
	}
}

func Sync() {
	klog.Flush()
}
```

- [ ] **Step 5: Verify GREEN for the bootstrap tests**

Run:

```bash
go test ./pkg/logging -run 'TestConfigure' -count=1
```

Expected: PASS for the bootstrap tests, while the repository policy test still fails until runtime call sites are migrated.

- [ ] **Step 6: Commit the bootstrap**

Run:

```bash
git add pkg/logging/logging.go pkg/logging/logging_test.go pkg/logging/repository_policy_test.go
git commit -m "feat: add shared klog bootstrap"
```

### Task 2: Wire Bootstrap Into Process Startup And Convert Core Runtime Files

**Files:**
- Modify: `main.go`
- Modify: `main_posix.go`
- Modify: `main_windows.go`
- Modify: `agent/agent.go`
- Modify: `agent/metrics_agent.go`
- Modify: `agent/metrics_reader.go`
- Modify: `writer/writers.go`
- Modify: `writer/writer.go`
- Modify: `heartbeat/heartbeat.go`
- Test: `pkg/logging/repository_policy_test.go`
- Test: `agent/metrics_agent_test.go`

- [ ] **Step 1: Lock the core policy test scope**

Keep `pkg/logging/repository_policy_test.go` enforcing the core runtime files before any migration work:

```go
files := []string{
	filepath.Join("..", "..", "main.go"),
	filepath.Join("..", "..", "main_posix.go"),
	filepath.Join("..", "..", "main_windows.go"),
	filepath.Join("..", "..", "agent", "agent.go"),
	filepath.Join("..", "..", "agent", "metrics_agent.go"),
	filepath.Join("..", "..", "agent", "metrics_reader.go"),
	filepath.Join("..", "..", "writer", "writer.go"),
	filepath.Join("..", "..", "writer", "writers.go"),
	filepath.Join("..", "..", "heartbeat", "heartbeat.go"),
}
```

- [ ] **Step 2: Run the core policy test to verify RED**

Run:

```bash
go test ./pkg/logging -run TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches -count=1
```

Expected: FAIL on existing `log.Println` / `DebugMode` usage.

- [ ] **Step 3: Wire `pkg/logging` into startup and migrate core files**

Update `main.go` to register `klog` flags and initialize logging after config load:

```go
import (
	"flag"
	"fmt"
	"os"
	// ...

	"flashcat.cloud/categraf/pkg/logging"
	"k8s.io/klog/v2"
)

func init() {
	logging.RegisterFlags(flag.CommandLine)
	// existing appPath/chdir logic
}

func initLog() {
	if err := logging.Configure(
		config.Config.Log.FileName,
		config.Config.Log.MaxSize,
		config.Config.Log.MaxAge,
		config.Config.Log.MaxBackups,
		config.Config.Log.LocalTime,
		config.Config.Log.Compress,
		config.Config.DebugMode,
		config.Config.DebugLevel,
	); err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure logging: %v\n", err)
		os.Exit(1)
	}
}
```

Update representative call sites:

```go
// main.go
klog.InfoS("received signal", "signal", sig.String())
klog.InfoS("runner env", "binarydir", runner.Cwd, "hostname", runner.Hostname, "fd_limits", runner.FdLimits(), "vm_limits", runner.VMLimits())

// main_posix.go
klog.ErrorS(err, "reaping children failed")
klog.InfoS("reaped child process", "pid", e.pid, "status", e.status)

// agent/metrics_reader.go
klog.V(1).InfoS("before gather once", "input", r.inputName)
klog.V(1).InfoS("after gather once", "input", r.inputName, "duration", time.Since(start))
klog.ErrorS(fmt.Errorf("panic: %v", rc), "gather metrics panic", "input", r.inputName, "stack", string(runtimex.Stack(3)))

// agent/metrics_agent.go
klog.Warningf("no instances for input: %s", inputKey)
klog.InfoS("input started", "name", name)

// writer/writers.go
klog.Errorf("write %d samples failed, please increase queue size(%d)", len(items), l)
klog.V(1).InfoS("write time series", "count", len(timeSeries), "duration_ms", time.Since(now).Milliseconds())
```

- [ ] **Step 4: Verify GREEN on core packages**

Run:

```bash
go test ./pkg/logging ./agent ./writer ./heartbeat -count=1
```

Expected: PASS, including the repository policy test for the enforced core files.

- [ ] **Step 5: Commit the core runtime migration**

Run:

```bash
git add main.go main_posix.go main_windows.go agent/agent.go agent/metrics_agent.go agent/metrics_reader.go writer/writer.go writer/writers.go heartbeat/heartbeat.go pkg/logging/repository_policy_test.go
git commit -m "refactor: migrate core runtime logs to klog"
```

### Task 3: Remove Log-Only `DebugMode` Branches And Align Logger-Aware Tests

**Files:**
- Modify: `inputs/http_provider.go`
- Modify: `agent/metrics_agent.go`
- Modify: `agent/metrics_reader.go`
- Modify: `writer/writers.go`
- Modify: `agent/metrics_agent_test.go`
- Modify: `inputs/inputs_test.go`

- [ ] **Step 1: Write or extend a failing test for logger-aware initialization**

Add or extend `agent/metrics_agent_test.go` to assert logger-aware initialization still happens after the migration:

```go
func TestMetricsAgentInputGoUsesLoggerInitForInputAndInstances(t *testing.T) {
	restore := setupMetricsAgentTestConfig()
	defer restore()

	agent := &MetricsAgent{
		InputReaders: NewReaders(),
	}
	instance := &testMetricsInstance{
		labels: map[string]string{"target": "demo"},
	}
	input := &testMetricsInput{
		instances: []inputs.Instance{instance},
	}

	agent.inputGo("provider.demo", "sum", input)

	if input.loggerInit != 1 {
		t.Fatalf("expected logger-aware init once, got %d", input.loggerInit)
	}
	if instance.loggerInit != 1 {
		t.Fatalf("expected instance logger-aware init once, got %d", instance.loggerInit)
	}
}
```

- [ ] **Step 2: Verify RED for the affected packages**

Run:

```bash
go test ./agent ./inputs -count=1
```

Expected: FAIL until `inputGo`, `MayInit`, and the log-gated code paths are aligned with the new logger flow.

- [ ] **Step 3: Replace log-only `DebugMode` branches with verbosity-based logging**

Representative edits:

```go
// agent/metrics_reader.go
klog.V(1).InfoS("before gather once", "input", r.inputName)
r.gatherOnce()
klog.V(1).InfoS("after gather once", "input", r.inputName, "duration", time.Since(start))

// writer/writers.go
if config.Config.TestMode {
	printTestMetrics(samples)
	return
}
klog.V(1).InfoS("queued samples", "count", len(samples))

// inputs/http_provider.go
klog.V(2).InfoS("collector request", "plugin", ins.Name(), "url", url)
klog.V(2).InfoS("collector response", "plugin", ins.Name(), "status", resp.StatusCode)
```

Keep `DebugMode` only where it changes behavior beyond logging.

- [ ] **Step 4: Align logger-aware helpers**

Ensure `inputs.MayInit` keeps preferring `InitWithLogger`:

```go
func MayInit(target interface{}, logger klog.Logger) error {
	if in, ok := target.(interface{ InitWithLogger(klog.Logger) error }); ok {
		return in.InitWithLogger(logger)
	}
	if in, ok := target.(interface{ Init() error }); ok {
		return in.Init()
	}
	return nil
}
```

- [ ] **Step 5: Verify GREEN**

Run:

```bash
go test ./agent ./inputs -count=1
```

Expected: PASS, with no remaining log-only `DebugMode` branches in the touched files.

- [ ] **Step 6: Commit the debug-branch cleanup**

Run:

```bash
git add agent/metrics_agent.go agent/metrics_reader.go writer/writers.go inputs/http_provider.go agent/metrics_agent_test.go inputs/inputs_test.go
git commit -m "refactor: replace debug log branches with klog verbosity"
```

### Task 4: Migrate Remaining Repository-Owned Packages In Batches

**Files:**
- Modify: `api/**/*.go`
- Modify: `config/**/*.go`
- Modify: `ibex/**/*.go`
- Modify: `parser/**/*.go`
- Modify: `pkg/**/*.go`
- Modify: `inputs/**/*.go`

- [ ] **Step 1: Capture the next failing scope with a search**

Run:

```bash
rg -n 'log\.(Println|Printf)\(' api config ibex parser pkg inputs
```

Expected: non-empty output listing the next migration batch.

- [ ] **Step 2: Migrate one batch at a time**

Apply the same severity mapping throughout the batch:

```go
// old
log.Println("E! failed to collect metrics:", err)
log.Printf("W! Couldn't stat target %v: %v", target, err)
log.Println("D! http_response... target:", target)

// new
klog.ErrorS(err, "failed to collect metrics")
klog.Warningf("couldn't stat target %v: %v", target, err)
klog.V(1).InfoS("http_response target", "target", target)
```

Prefer `InfoS` / `ErrorS` when there are stable key/value fields to extract.

- [ ] **Step 3: Verify each batch immediately**

Run package tests after each edited subtree. Use the smallest package set that matches the edit:

```bash
go test ./api/... -count=1
go test ./config/... -count=1
go test ./ibex/... -count=1
go test ./parser/... -count=1
go test ./pkg/... -count=1
go test ./inputs/... -count=1
```

Expected: PASS for each touched subtree before moving on.

- [ ] **Step 4: Commit the package migrations**

Use one or more commits, but keep them scoped by area:

```bash
git add api config ibex parser pkg inputs
git commit -m "refactor: migrate repository logs to klog"
```

### Task 5: Clean Tests And Documentation, Then Run Final Verification

**Files:**
- Modify: `**/*_test.go` that still demonstrates stdlib logging
- Modify: `docs/**/*.md`
- Modify: `docs/superpowers/specs/2026-04-14-global-klog-standardization-design.md`
- Modify: `docs/superpowers/plans/2026-04-13-global-klog-logging.md`

- [ ] **Step 1: Find failing cleanup scope**

Run:

```bash
rg -n 'log\.(Println|Printf)\(' . --glob '**/*_test.go' --glob 'docs/**/*.md' --glob '!docs/superpowers/plans/2026-04-14-global-klog-standardization.md'
rg -n 'if config\.Config\.DebugMode \{' . --glob '**/*_test.go' --glob 'docs/**/*.md' --glob '!docs/superpowers/plans/2026-04-14-global-klog-standardization.md'
```

Expected: matches in tests and documentation only.

- [ ] **Step 2: Update test and doc examples to canonical `klog` style**

Representative replacements:

```go
// tests/docs old
log.Println("E! failed to collect metrics:", err)
if config.Config.DebugMode {
	log.Println("D! heartbeat response:", string(bs))
}

// tests/docs new
klog.ErrorS(err, "failed to collect metrics")
klog.V(1).InfoS("heartbeat response", "body", string(bs))
```

For prose examples, remove `I!/W!/E!/D!` prefixes unless the doc is explicitly discussing the legacy format.

- [ ] **Step 3: Run final verification**

Run:

```bash
go test ./pkg/logging ./agent ./writer ./heartbeat ./api/... ./config/... ./ibex/... ./parser/... ./pkg/... ./inputs/... -count=1
rg -n 'log\.(Println|Printf)\(' . --glob '!vendor/**' --glob '!docs/superpowers/plans/2026-04-14-global-klog-standardization.md'
rg -n 'if config\.Config\.DebugMode \{' . --glob '!vendor/**' --glob '!docs/superpowers/plans/2026-04-14-global-klog-standardization.md'
```

Expected:
- `go test` passes for all touched packages
- `rg` returns no repository-owned runtime, test, or doc matches other than any explicitly accepted exclusions

- [ ] **Step 4: Commit the cleanup and verification state**

Run:

```bash
git add docs/superpowers/plans/2026-04-13-global-klog-logging.md docs/superpowers/specs/2026-04-14-global-klog-standardization-design.md
git commit -m "docs: align tests and docs with klog logging policy"
```
