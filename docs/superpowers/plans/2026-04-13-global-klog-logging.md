# Global Klog Logging Standardization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Standardize repository-owned logging on shared `klog`, remove standard-library `log.Printf` / `log.Println` as the normal logging path, and replace `DebugMode`-gated extra logs with verbosity-based `klog` output.

**Architecture:** Add a shared `pkg/logging` package that owns `klog` setup, output routing, standard-library bridging, and contextual logger helpers. Migrate repository callsites in batches, using repository policy tests plus `go test` and `rg` sweeps to enforce that runtime code no longer relies on standard-library logging or `DebugMode` branches just to emit extra logs. Preserve business behavior and keep `DebugMod` fields only where they control downstream library behavior rather than local log emission.

**Tech Stack:** Go 1.25, `k8s.io/klog/v2`, standard `flag` package, existing `agent`, `inputs`, `writer`, `heartbeat`, `ibex`, `logs`, and `pkg` packages, `go test`, `rg`

---

## File Map

- `pkg/logging/logging.go`: shared `klog` registration, configuration, output writer selection, flush lifecycle, and contextual logger helpers
- `pkg/logging/logging_test.go`: focused unit tests for output selection, verbosity mapping, and standard-library bridge behavior
- `pkg/logging/repository_policy_test.go`: repo policy tests that fail when scoped runtime files still use `log.Printf` / `log.Println` or `DebugMode`-only log branches
- `main.go`, `main_posix.go`, `main_windows.go`: process-wide logger registration, early service-path initialization, and shutdown flushing
- `agent/metrics_agent.go`, `agent/metrics_reader.go`, `agent/agent.go`, `agent/prometheus_agent.go`, `agent/ibex_agent.go`: shared logger usage in the main collection loop and lifecycle logs
- `writer/writers.go`, `writer/writer.go`: writer queue and remote write logs converted to `klog`, with debug output moved to `V(level)`
- `config/config.go`, `config/hostname.go`, `config/urllabel.go`: config, hostname, and URL label logs moved off standard `log`
- `heartbeat/heartbeat.go`: debug helper converted from `DebugMode` gate to verbosity-driven `klog`
- `ibex/*.go`, `logs/**/*.go`, `pkg/**/*.go`, `parser/**/*.go`, `api/**/*.go`: runtime packages migrated in batches
- `inputs/**/*.go`: migrated in two waves, first framework/common collectors, then heavyweight subtrees such as `elasticsearch`, `mtail`, `node_exporter`, `ipmi`, and `snmp_zabbix`

### Task 1: Add Shared Logging Base And Repository Policy Tests

**Files:**
- Create: `pkg/logging/logging.go`
- Create: `pkg/logging/logging_test.go`
- Create: `pkg/logging/repository_policy_test.go`

- [ ] **Step 1: Write the failing tests for the shared logging package**

Add `pkg/logging/logging_test.go` with focused tests around output routing and the standard-library bridge:

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
		t.Fatalf("expected verbosity 1 output, got %q", buf.String())
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
		t.Fatalf("expected bridged standard log output, got %q", buf.String())
	}
}
```

- [ ] **Step 2: Write the failing repository policy test for the first migration scope**

Add `pkg/logging/repository_policy_test.go` to enforce the first batch of runtime files:

```go
package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var forbiddenStdLog = regexp.MustCompile(`\blog\.(Printf|Println|Fatal|Fatalf|Fatalln)\b`)
var forbiddenDebugBranch = regexp.MustCompile(`if\s+(config\.Config\.DebugMode|Config\.DebugMode)\s*\{`)

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
			t.Fatalf("forbidden DebugMode-only log branch remains in %s", file)
		}
	}
}
```

- [ ] **Step 3: Run the new logging tests to verify RED**

Run:

```bash
go test ./pkg/logging -count=1
```

Expected: FAIL because `pkg/logging` does not exist and the policy test points at files that still use standard-library logging.

- [ ] **Step 4: Implement the shared logging package**

Create `pkg/logging/logging.go` with explicit flag registration, configurable output, and a bridge for legacy `log`:

```go
package logging

import (
	"flag"
	"fmt"
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

func Component(name string) klog.Logger {
	return klog.Background().WithName(name)
}

func ComponentValues(name string, kv ...interface{}) klog.Logger {
	return Component(name).WithValues(kv...)
}

func Verbose(level int, msg string, kv ...interface{}) {
	klog.V(klog.Level(level)).InfoS(msg, kv...)
}
```

- [ ] **Step 5: Run the shared logging tests again and confirm the policy test still fails for unmigrated runtime files**

Run:

```bash
go test ./pkg/logging -count=1
```

Expected: `logging_test.go` passes, while `TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches` still fails because the scoped runtime files are not migrated yet.

- [ ] **Step 6: Commit the logging base and policy test scaffolding**

Run:

```bash
git add pkg/logging/logging.go pkg/logging/logging_test.go pkg/logging/repository_policy_test.go
git commit -m "test: add shared logging base and policy checks"
```

Expected: One commit with the shared logging package and failing-policy scaffold.

### Task 2: Wire Shared Logging Into Process Startup And Inputs Initialization

**Files:**
- Modify: `main.go`
- Modify: `main_posix.go`
- Modify: `main_windows.go`
- Modify: `agent/metrics_agent.go`
- Modify: `agent/metrics_agent_test.go`
- Modify: `inputs/inputs.go`
- Modify: `inputs/inputs_test.go`

- [ ] **Step 1: Register `klog` flags before parsing CLI flags**

In `main.go`, register `klog` flags once before `flag.Parse()` and remove the old `initLog` helper:

```go
func init() {
	logging.RegisterFlags(flag.CommandLine)

	var err error
	if appPath, err = winsvc.GetAppPath(); err != nil {
		klog.Fatal(err)
	}
	if err := os.Chdir(filepath.Dir(appPath)); err != nil {
		klog.Fatal(err)
	}
}
```

- [ ] **Step 2: Configure a minimal stderr logger before service-control paths and reconfigure after config load**

Update `main.go` so both early service flows and the normal runtime use `pkg/logging`:

```go
func main() {
	flag.Parse()

	if err := logging.Configure("stderr", 0, 0, 0, false, false, *debugMode, *debugLevel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logging.Sync()

	if *install || *remove || *start || *stop || *status || *update {
		if err := serviceProcess(); err != nil {
			klog.ErrorS(err, "service command failed")
		}
		return
	}

	if err := config.InitConfig(*configDir, *debugLevel, *debugMode, *testMode, *interval, *inputFilters); err != nil {
		klog.Fatal(err)
	}

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
		klog.Fatal(err)
	}
}
```

- [ ] **Step 3: Update Windows and POSIX entrypoints to use `klog` and shared flushing**

Replace direct `log.Fatalln` / `log.Printf` usage in `main_posix.go` and `main_windows.go` with `klog`:

```go
func reapDaemon() {
	...
	if err != nil {
		klog.ErrorS(err, "failed to reap child processes")
		continue
	}
	klog.InfoS("reaped child process", "pid", e.pid, "status", e.status)
}
```

```go
if err := winsvc.RunAsService(*flagWinSvcName, ag.Start, ag.Stop, false); err != nil {
	klog.Fatal(err)
}
```

- [ ] **Step 4: Fold the current metrics-agent logger derivation into the shared logging helper**

Update `agent/metrics_agent.go` to derive child loggers from `pkg/logging`:

```go
func metricsAgentInputLogger(name string, sum string) klog.Logger {
	return logging.ComponentValues(
		"inputs",
		"component", "inputs",
		"input", name,
		"plugin", parsedInputKey(name),
		"checksum", sum,
	)
}

func metricsAgentInstanceLogger(inputLogger klog.Logger, idx int) klog.Logger {
	return inputLogger.WithValues("instance_index", idx)
}
```

Keep `inputs.MayInit(t, logger)` and the existing tests, but update assertions to check the logger still comes from the shared path.

- [ ] **Step 5: Run the focused tests for startup-adjacent code**

Run:

```bash
go test ./pkg/logging ./inputs ./agent -run 'Test(MayInit|MetricsAgentInputGo|Configure|CoreRuntime)' -count=1
```

Expected: `pkg/logging` tests pass, `inputs` and `agent` tests pass, and the core runtime policy test still fails until Task 3 migrates the runtime files.

- [ ] **Step 6: Commit the startup wiring and shared logger integration**

Run:

```bash
git add main.go main_posix.go main_windows.go agent/metrics_agent.go agent/metrics_agent_test.go inputs/inputs.go inputs/inputs_test.go
git commit -m "refactor: wire shared klog startup logging"
```

Expected: One commit containing only startup wiring plus the shared `inputs` logger integration.

### Task 3: Migrate Core Runtime Packages And Remove Pure Debug Log Branches

**Files:**
- Modify: `agent/agent.go`
- Modify: `agent/metrics_agent.go`
- Modify: `agent/metrics_reader.go`
- Modify: `agent/prometheus_agent.go`
- Modify: `agent/ibex_agent.go`
- Modify: `config/config.go`
- Modify: `config/hostname.go`
- Modify: `config/urllabel.go`
- Modify: `heartbeat/heartbeat.go`
- Modify: `writer/writer.go`
- Modify: `writer/writers.go`
- Modify: `parser/influx/parser.go`
- Modify: `parser/prometheus/parser.go`
- Modify: `pkg/aop/logger.go`
- Modify: `pkg/aop/recovery.go`
- Modify: `pkg/httpx/client.go`
- Modify: `pkg/httpx/transport.go`
- Modify: `pkg/kubernetes/pod.go`
- Modify: `pkg/pprof/profile.go`
- Modify: `pkg/snmp/translate.go`
- Modify: `pkg/logging/repository_policy_test.go`

- [ ] **Step 1: Extend the policy test to cover the full core runtime batch**

Append these files to `TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches` in `pkg/logging/repository_policy_test.go`:

```go
files = append(files,
	filepath.Join("..", "..", "agent", "prometheus_agent.go"),
	filepath.Join("..", "..", "agent", "ibex_agent.go"),
	filepath.Join("..", "..", "config", "config.go"),
	filepath.Join("..", "..", "config", "hostname.go"),
	filepath.Join("..", "..", "config", "urllabel.go"),
	filepath.Join("..", "..", "parser", "influx", "parser.go"),
	filepath.Join("..", "..", "parser", "prometheus", "parser.go"),
	filepath.Join("..", "..", "pkg", "httpx", "client.go"),
	filepath.Join("..", "..", "pkg", "httpx", "transport.go"),
)
```

- [ ] **Step 2: Run the policy test and capture the RED failures**

Run:

```bash
go test ./pkg/logging -run TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches -count=1
```

Expected: FAIL with the first remaining file that still contains standard-library `log` calls or a `DebugMode` logging branch.

- [ ] **Step 3: Replace direct runtime logs with structured `klog` calls**

Apply the same style across the files in this task:

```go
klog.InfoS("agent started", "agent", fmt.Sprintf("%T", agent))
klog.ErrorS(err, "failed to start agent", "agent", fmt.Sprintf("%T", agent))
klog.Warningf("writers queue is full, dropped %d samples (queue_size=%d)", len(items), l)
```

For debug-only branches such as `agent/metrics_reader.go`, remove the branch and use verbosity:

```go
klog.V(1).InfoS("before gather once", "input", r.inputName)
r.gatherOnce()
klog.V(1).InfoS("after gather once", "input", r.inputName, "duration", time.Since(start))
```

For `heartbeat/heartbeat.go`, keep the helper but convert it to verbosity:

```go
func heartbeatLogger() klog.Logger {
	return logging.Component("heartbeat")
}

func heartbeatDebug() klog.Verbose {
	level := klog.V(1)
	return level
}
```

Then replace `if debug() { log.Printf(...) }` with:

```go
heartbeatDebug().InfoS("heartbeat request", "body", string(bs))
```

- [ ] **Step 4: Preserve non-logging debug behavior and only remove log-only branches**

Do not remove `DebugMod` or downstream debug toggles that are passed into other libraries. Restrict this task to code where `DebugMode` exists only to guard local logging, for example:

```go
if config.Config.DebugMode {
	printTestMetrics(samples)
}
```

Replace this in `writer/writers.go` with:

```go
if klog.V(1).Enabled() {
	printTestMetrics(samples)
}
```

- [ ] **Step 5: Run targeted tests plus the core runtime policy test**

Run:

```bash
go test ./pkg/logging ./agent ./writer ./heartbeat ./config ./parser/... ./pkg/... -count=1
```

Expected: PASS for the touched packages, including `TestCoreRuntimeDoesNotUseStandardLogOrDebugBranches`.

- [ ] **Step 6: Commit the core runtime migration**

Run:

```bash
git add agent/agent.go agent/metrics_agent.go agent/metrics_reader.go agent/prometheus_agent.go agent/ibex_agent.go config/config.go config/hostname.go config/urllabel.go heartbeat/heartbeat.go writer/writer.go writer/writers.go parser/influx/parser.go parser/prometheus/parser.go pkg/aop/logger.go pkg/aop/recovery.go pkg/httpx/client.go pkg/httpx/transport.go pkg/kubernetes/pod.go pkg/pprof/profile.go pkg/snmp/translate.go pkg/logging/repository_policy_test.go
git commit -m "refactor: migrate core runtime logging to klog"
```

Expected: One commit containing the shared runtime-path migration and the updated core policy scope.

### Task 4: Migrate API, Service Management, Ibex, And Logs Runtime Packages

**Files:**
- Modify: `api/router_falcon.go`
- Modify: `api/router_opentsdb.go`
- Modify: `api/server.go`
- Modify: `agent/install/service_linux.go`
- Modify: `agent/update/update_linux.go`
- Modify: `agent/update/update_windows.go`
- Modify: `ibex/client/cli.go`
- Modify: `ibex/heartbeat.go`
- Modify: `ibex/task.go`
- Modify: `ibex/tasks.go`
- Modify: `logs/auditor/auditor.go`
- Modify: `logs/client/http/destination.go`
- Modify: `logs/client/kafka/destination.go`
- Modify: `logs/client/kafka/producer.go`
- Modify: `logs/client/tcp/connection_manager.go`
- Modify: `logs/decoder/auto_multiline_handler.go`
- Modify: `logs/decoder/decoder.go`
- Modify: `logs/decoder/line_parser.go`
- Modify: `logs/input/container/launcher.go`
- Modify: `logs/input/file/file_provider.go`
- Modify: `logs/input/file/scanner.go`
- Modify: `logs/input/file/tailer.go`
- Modify: `logs/input/file/tailer_nix.go`
- Modify: `logs/input/file/tailer_windows.go`
- Modify: `logs/input/journald/launcher.go`
- Modify: `logs/input/journald/tailer.go`
- Modify: `logs/input/kubernetes/json_parser.go`
- Modify: `logs/input/kubernetes/launcher.go`
- Modify: `logs/input/kubernetes/scanner.go`
- Modify: `logs/input/listener/tailer.go`
- Modify: `logs/input/listener/tcp.go`
- Modify: `logs/input/listener/udp.go`
- Modify: `logs/message/origin.go`
- Modify: `logs/processor/processor.go`
- Modify: `logs/sender/batch_strategy.go`
- Modify: `logs/sender/stream_strategy.go`
- Modify: `logs/tag/provider.go`
- Modify: `logs/util/containers/filter.go`
- Modify: `logs/util/containers/providers/provider.go`
- Modify: `logs/util/debug.go`
- Modify: `logs/util/docker/containers.go`
- Modify: `logs/util/docker/docker.go`
- Modify: `logs/util/docker/event_pull.go`
- Modify: `logs/util/docker/event_stream.go`
- Modify: `logs/util/docker/global.go`
- Modify: `logs/util/docker/network.go`
- Modify: `logs/util/docker/rancher.go`
- Modify: `logs/util/docker/storage.go`
- Modify: `logs/util/kubernetes/kubelet/containers.go`
- Modify: `logs/util/kubernetes/kubelet/kubelet.go`
- Modify: `logs/util/kubernetes/kubelet/kubelet_client.go`
- Modify: `logs/util/kubernetes/kubelet/kubelet_hosts.go`
- Modify: `logs/util/kubernetes/kubelet/podwatcher.go`
- Modify: `logs/util/kubernetes/tags/builder.go`
- Modify: `pkg/logging/repository_policy_test.go`

- [ ] **Step 1: Expand the repository policy test to the API, service, ibex, and logs batch**

Add a second policy test that walks these directories recursively:

```go
func TestServiceAndLogsRuntimeDoesNotUseStandardLogOrDebugBranches(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "api"),
		filepath.Join("..", "..", "ibex"),
		filepath.Join("..", "..", "logs"),
		filepath.Join("..", "..", "agent", "install"),
		filepath.Join("..", "..", "agent", "update"),
	}
	assertNoForbiddenLogging(t, roots)
}
```

Use an `assertNoForbiddenLogging` helper that skips `vendor`, `docs`, `README.md`, generated protobufs, and `_test.go`.

- [ ] **Step 2: Run the new policy test to confirm RED**

Run:

```bash
go test ./pkg/logging -run TestServiceAndLogsRuntimeDoesNotUseStandardLogOrDebugBranches -count=1
```

Expected: FAIL on the first file in these directories that still contains `log.Printf`, `log.Println`, or a pure log-only `DebugMode` branch.

- [ ] **Step 3: Migrate the batch to structured `klog`**

Use consistent `klog` patterns:

```go
logger := logging.ComponentValues("ibex", "task_id", t.Id)
logger.Error(err, "failed to write args file", "path", argsFile)
logger.Info("task finished")
```

For `logs/util/debug.go`, keep a helper but return verbosity instead of a `DebugMode` boolean:

```go
func Verbose() klog.Verbose {
	return klog.V(1)
}
```

Then change callsites from:

```go
if util.Debug() {
	log.Println("D! docker event", event)
}
```

to:

```go
util.Verbose().InfoS("docker event", "event", event)
```

- [ ] **Step 4: Run package tests for the touched service/runtime code**

Run:

```bash
go test ./pkg/logging ./api ./agent/... ./ibex/... ./logs/... -count=1
```

Expected: PASS for the touched packages, including the new service/logs policy test.

- [ ] **Step 5: Commit the service and logs migration**

Run:

```bash
git add api/router_falcon.go api/router_opentsdb.go api/server.go agent/install/service_linux.go agent/update/update_linux.go agent/update/update_windows.go ibex/client/cli.go ibex/heartbeat.go ibex/task.go ibex/tasks.go logs pkg/logging/repository_policy_test.go
git commit -m "refactor: migrate service and logs runtime logging"
```

Expected: One commit containing only the API/service, ibex, and logs runtime migration.

### Task 5: Migrate Inputs Framework And Medium-Complexity Collectors

**Files:**
- Modify: `inputs/collector.go`
- Modify: `inputs/http_provider.go`
- Modify: `inputs/provider_manager.go`
- Modify: `inputs/aliyun/cloud.go`
- Modify: `inputs/amd_rocm_smi/amd_rocm_smi.go`
- Modify: `inputs/appdynamics/instances.go`
- Modify: `inputs/arp_packet/arp_packet.go`
- Modify: `inputs/bind/bind.go`
- Modify: `inputs/cadvisor/instances.go`
- Modify: `inputs/chrony/chrony.go`
- Modify: `inputs/clickhouse/clickhouse.go`
- Modify: `inputs/cloudwatch/cloudwatch.go`
- Modify: `inputs/conntrack/conntrack.go`
- Modify: `inputs/consul/consul.go`
- Modify: `inputs/cpu/cpu.go`
- Modify: `inputs/dcgm/exporter.go`
- Modify: `inputs/disk/disk.go`
- Modify: `inputs/diskio/diskio.go`
- Modify: `inputs/dmesg/dmesg.go`
- Modify: `inputs/dns_query/dns_query.go`
- Modify: `inputs/docker/docker.go`
- Modify: `inputs/emc_unity/emc_unity.go`
- Modify: `inputs/ethtool/command_linux.go`
- Modify: `inputs/ethtool/ethtool_linux.go`
- Modify: `inputs/ethtool/ethtool_notlinux.go`
- Modify: `inputs/ethtool/namespace_linux.go`
- Modify: `inputs/exec/exec.go`
- Modify: `inputs/filecount/filecount.go`
- Modify: `inputs/gnmi/gnmi.go`
- Modify: `inputs/gnmi/handler.go`
- Modify: `inputs/googlecloud/instances.go`
- Modify: `inputs/greenplum/greenplum.go`
- Modify: `inputs/hadoop/hadoop.go`
- Modify: `inputs/haproxy/exporter.go`
- Modify: `inputs/haproxy/haproxy.go`
- Modify: `inputs/http_response/http_response.go`
- Modify: `inputs/huatuo/huatuo.go`
- Modify: `inputs/ipmi/instances.go`
- Modify: `inputs/iptables/iptables.go`
- Modify: `inputs/ipvs/ipvs_linux_amd64.go`
- Modify: `inputs/jenkins/jenkins.go`
- Modify: `inputs/jolokia/gatherer.go`
- Modify: `inputs/jolokia_agent/jolokia_agent.go`
- Modify: `inputs/jolokia_proxy/jolokia_proxy.go`
- Modify: `inputs/kafka/kafka.go`
- Modify: `inputs/kernel/kernel.go`
- Modify: `inputs/kernel_vmstat/kernel_vmstat.go`
- Modify: `inputs/kubernetes/kubernetes.go`
- Modify: `inputs/ldap/ldap.go`
- Modify: `inputs/linux_sysctl_fs/linux_sysctl_fs_linux.go`
- Modify: `inputs/logstash/logstash.go`
- Modify: `inputs/mem/mem.go`
- Modify: `inputs/mongodb/mongodb.go`
- Modify: `inputs/mongodb/mongodb_server.go`
- Modify: `inputs/mysql/binlog.go`
- Modify: `inputs/mysql/custom_queries.go`
- Modify: `inputs/mysql/engine_innodb.go`
- Modify: `inputs/mysql/global_status.go`
- Modify: `inputs/mysql/global_variables.go`
- Modify: `inputs/mysql/mysql.go`
- Modify: `inputs/mysql/processlist.go`
- Modify: `inputs/mysql/processlist_by_user.go`
- Modify: `inputs/mysql/schema_size.go`
- Modify: `inputs/mysql/slave_status.go`
- Modify: `inputs/mysql/table_size.go`
- Modify: `inputs/nats/nats.go`
- Modify: `inputs/net/net.go`
- Modify: `inputs/net_response/net_response.go`
- Modify: `inputs/netstat/netstat.go`
- Modify: `inputs/netstat_filter/netstat_filter.go`
- Modify: `inputs/netstat_filter/netstat_tcp.go`
- Modify: `inputs/nfsclient/nfsclient.go`
- Modify: `inputs/nginx/nginx.go`
- Modify: `inputs/nginx_upstream_check/nginx_upstream_check.go`
- Modify: `inputs/nsq/nsq.go`
- Modify: `inputs/ntp/ntp.go`
- Modify: `inputs/nvidia_smi/builder.go`
- Modify: `inputs/nvidia_smi/nvidia_smi.go`
- Modify: `inputs/oracle/oracle.go`
- Modify: `inputs/phpfpm/phpfpm.go`
- Modify: `inputs/ping/ping.go`
- Modify: `inputs/ping/ping_notwindows.go`
- Modify: `inputs/ping/ping_windows.go`
- Modify: `inputs/postgresql/postgresql.go`
- Modify: `inputs/processes/processes_notwindows.go`
- Modify: `inputs/procstat/procstat.go`
- Modify: `inputs/procstat/win_service_windows.go`
- Modify: `inputs/prometheus/consul.go`
- Modify: `inputs/prometheus/prometheus.go`
- Modify: `inputs/rabbitmq/rabbitmq.go`
- Modify: `inputs/redfish/redfish.go`
- Modify: `inputs/redis/redis.go`
- Modify: `inputs/redis_sentinel/redis_sentinel.go`
- Modify: `inputs/rocketmq_offset/rocketmq.go`
- Modify: `inputs/self_metrics/metrics.go`
- Modify: `inputs/smart/instances.go`
- Modify: `inputs/snmp/health_check.go`
- Modify: `inputs/snmp/instances.go`
- Modify: `inputs/snmp/netsnmp.go`
- Modify: `inputs/snmp/table.go`
- Modify: `inputs/snmp/wrapper.go`
- Modify: `inputs/snmp_trap/snmp_trap.go`
- Modify: `inputs/sockstat/sockstat.go`
- Modify: `inputs/sqlserver/sqlserver.go`
- Modify: `inputs/supervisor/supervisor.go`
- Modify: `inputs/switch_legacy/switch_legacy.go`
- Modify: `inputs/system/ps.go`
- Modify: `inputs/system/system.go`
- Modify: `inputs/systemd/systemd_linux.go`
- Modify: `inputs/tengine/tengine.go`
- Modify: `inputs/tomcat/tomcat.go`
- Modify: `inputs/traffic_server/traffic_server.go`
- Modify: `inputs/vsphere/client.go`
- Modify: `inputs/vsphere/endpoint.go`
- Modify: `inputs/vsphere/finder.go`
- Modify: `inputs/vsphere/tscache.go`
- Modify: `inputs/vsphere/vsphere.go`
- Modify: `inputs/whois/whois.go`
- Modify: `inputs/x509_cert/x509_cert.go`
- Modify: `inputs/xskyapi/xskyapi.go`
- Modify: `inputs/zookeeper/zookeeper.go`
- Modify: `pkg/logging/repository_policy_test.go`

- [ ] **Step 1: Add a recursive inputs policy test for the medium-complexity batch**

Add this test to `pkg/logging/repository_policy_test.go`:

```go
func TestInputsMediumBatchDoesNotUseStandardLog(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "inputs"),
	}
	skip := []string{
		filepath.Join("inputs", "elasticsearch"),
		filepath.Join("inputs", "ipmi", "exporter"),
		filepath.Join("inputs", "mtail"),
		filepath.Join("inputs", "node_exporter"),
		filepath.Join("inputs", "snmp_zabbix"),
	}
	assertNoForbiddenLoggingExcept(t, roots, skip)
}
```

- [ ] **Step 2: Run the new inputs policy test to verify RED**

Run:

```bash
go test ./pkg/logging -run TestInputsMediumBatchDoesNotUseStandardLog -count=1
```

Expected: FAIL because the medium-complexity inputs files still contain standard-library logging or log-only `DebugMode` branches.

- [ ] **Step 3: Migrate inputs callsites while preserving non-logging debug controls**

Use `klog` directly for local logs and keep `DebugMod` only where it is passed into another system:

```go
logger := logging.ComponentValues("inputs", "plugin", ins.Name(), "target", ins.Address)
logger.Error(err, "failed to query target")
klog.V(2).InfoS("collector request", "plugin", ins.Name(), "url", url)
```

For cases like `inputs/cloudwatch/cloudwatch.go` and `inputs/vsphere/client.go`, keep `ins.DebugMod` when it is forwarded into SDK or collector configuration, but replace local branches such as:

```go
if ins.DebugMod {
	log.Printf("D! cloudwatch request: %s", req)
}
```

with:

```go
klog.V(1).InfoS("cloudwatch request", "request", req)
```

- [ ] **Step 4: Run the medium-batch package tests and policy test**

Run:

```bash
go test ./pkg/logging ./inputs/... -count=1
```

Expected: PASS for the touched inputs packages and `TestInputsMediumBatchDoesNotUseStandardLog`.

- [ ] **Step 5: Commit the medium-batch inputs migration**

Run:

```bash
git add inputs pkg/logging/repository_policy_test.go
git commit -m "refactor: migrate collector logging to klog"
```

Expected: One commit covering the inputs framework and medium-complexity collector migration.

### Task 6: Migrate Heavyweight Collector Subtrees And Run The Final Repository Sweep

**Files:**
- Modify: `inputs/elasticsearch/collector/categraf_utils.go`
- Modify: `inputs/elasticsearch/collector/cluster_health.go`
- Modify: `inputs/elasticsearch/collector/cluster_health_indices.go`
- Modify: `inputs/elasticsearch/collector/cluster_settings.go`
- Modify: `inputs/elasticsearch/collector/cluster_stats.go`
- Modify: `inputs/elasticsearch/collector/collector.go`
- Modify: `inputs/elasticsearch/collector/indices.go`
- Modify: `inputs/elasticsearch/collector/indices_mappings.go`
- Modify: `inputs/elasticsearch/collector/indices_settings.go`
- Modify: `inputs/elasticsearch/collector/nodes.go`
- Modify: `inputs/elasticsearch/collector/shards.go`
- Modify: `inputs/elasticsearch/collector/tasks.go`
- Modify: `inputs/elasticsearch/collector/util.go`
- Modify: `inputs/elasticsearch/elasticsearch.go`
- Modify: `inputs/elasticsearch/pkg/clusterinfo/clusterinfo.go`
- Modify: `inputs/elasticsearch/pkg/roundtripper/roundtripper.go`
- Modify: `inputs/ipmi/exporter/collector_bmc.go`
- Modify: `inputs/ipmi/exporter/collector_bmc_watchdog.go`
- Modify: `inputs/ipmi/exporter/collector_chassis.go`
- Modify: `inputs/ipmi/exporter/collector_dcmi.go`
- Modify: `inputs/ipmi/exporter/collector_ipmi.go`
- Modify: `inputs/ipmi/exporter/collector_notwindows.go`
- Modify: `inputs/ipmi/exporter/collector_sel.go`
- Modify: `inputs/ipmi/exporter/collector_sm_lan_mode.go`
- Modify: `inputs/ipmi/exporter/freeipmi/freeipmi.go`
- Modify: `inputs/mtail/internal/exporter/export.go`
- Modify: `inputs/mtail/internal/exporter/json.go`
- Modify: `inputs/mtail/internal/exporter/prometheus.go`
- Modify: `inputs/mtail/internal/metrics/metric.go`
- Modify: `inputs/mtail/internal/metrics/store.go`
- Modify: `inputs/mtail/internal/mtail/golden/reader.go`
- Modify: `inputs/mtail/internal/mtail/httpstatus.go`
- Modify: `inputs/mtail/internal/mtail/mtail.go`
- Modify: `inputs/mtail/internal/runtime/compiler/checker/checker.go`
- Modify: `inputs/mtail/internal/runtime/compiler/codegen/codegen.go`
- Modify: `inputs/mtail/internal/runtime/compiler/compiler.go`
- Modify: `inputs/mtail/internal/runtime/compiler/parser/lexer.go`
- Modify: `inputs/mtail/internal/runtime/compiler/types/types.go`
- Modify: `inputs/mtail/internal/runtime/runtime.go`
- Modify: `inputs/mtail/internal/runtime/vm/vm.go`
- Modify: `inputs/mtail/internal/tailer/logstream/cancel.go`
- Modify: `inputs/mtail/internal/tailer/logstream/dgramstream.go`
- Modify: `inputs/mtail/internal/tailer/logstream/fifostream.go`
- Modify: `inputs/mtail/internal/tailer/logstream/filestream.go`
- Modify: `inputs/mtail/internal/tailer/logstream/logstream.go`
- Modify: `inputs/mtail/internal/tailer/logstream/socketstream.go`
- Modify: `inputs/mtail/internal/tailer/tail.go`
- Modify: `inputs/mtail/internal/waker/testwaker.go`
- Modify: `inputs/mtail/mtail.go`
- Modify: `inputs/node_exporter/collector/buddyinfo.go`
- Modify: `inputs/node_exporter/collector/collector.go`
- Modify: `inputs/node_exporter/collector/cpu_linux.go`
- Modify: `inputs/node_exporter/collector/diskstats_common.go`
- Modify: `inputs/node_exporter/collector/diskstats_linux.go`
- Modify: `inputs/node_exporter/collector/ethtool_linux.go`
- Modify: `inputs/node_exporter/collector/filesystem_common.go`
- Modify: `inputs/node_exporter/collector/netclass_rtnl_linux.go`
- Modify: `inputs/node_exporter/collector/netdev_common.go`
- Modify: `inputs/node_exporter/collector/ntp.go`
- Modify: `inputs/node_exporter/collector/perf_linux.go`
- Modify: `inputs/node_exporter/collector/qdisc_linux.go`
- Modify: `inputs/node_exporter/collector/runit.go`
- Modify: `inputs/node_exporter/collector/supervisord.go`
- Modify: `inputs/node_exporter/collector/systemd_linux.go`
- Modify: `inputs/node_exporter/collector/textfile.go`
- Modify: `inputs/node_exporter/exporter.go`
- Modify: `inputs/snmp_zabbix/collector.go`
- Modify: `inputs/snmp_zabbix/discovery.go`
- Modify: `inputs/snmp_zabbix/discovery_scheduler.go`
- Modify: `inputs/snmp_zabbix/preprocessing.go`
- Modify: `inputs/snmp_zabbix/scheduler.go`
- Modify: `inputs/snmp_zabbix/snmp.go`
- Modify: `inputs/snmp_zabbix/snmp_client.go`
- Modify: `inputs/snmp_zabbix/template.go`
- Modify: `pkg/logging/repository_policy_test.go`

- [ ] **Step 1: Expand the inputs policy test to cover the heavyweight subtrees**

Replace the skip list in `TestInputsMediumBatchDoesNotUseStandardLog` with a full recursive inputs assertion:

```go
func TestAllInputsDoNotUseStandardLog(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "inputs"),
	}
	assertNoForbiddenLogging(t, roots)
}
```

Keep explicit skips only for docs, fixtures, generated outputs, `README.md`, and `_test.go`.

- [ ] **Step 2: Run the full inputs policy test to confirm RED**

Run:

```bash
go test ./pkg/logging -run TestAllInputsDoNotUseStandardLog -count=1
```

Expected: FAIL on one of the heavyweight collector files listed in this task.

- [ ] **Step 3: Migrate the heavyweight subtrees to `klog`**

Use the same repository conventions throughout:

```go
logger := logging.ComponentValues("inputs", "plugin", "elasticsearch", "cluster", clusterName)
logger.Error(err, "failed to fetch cluster health")
klog.V(2).InfoS("collector request", "plugin", "node_exporter", "collector", name)
```

For collectors that still use `DebugMod`, keep external debug toggles but replace local log-only branches with `klog.V(level)` calls. For example:

```go
if klog.V(2).Enabled() {
	klog.V(2).InfoS("received cluster info update", "cluster", ci.ClusterName)
}
```

- [ ] **Step 4: Run the final package tests and repository-wide scans**

Run:

```bash
go test ./... -count=1
rg -n '\blog\.(Printf|Println|Fatal|Fatalf|Fatalln)\b' . --glob '!vendor/**' --glob '!docs/**' --glob '!**/*_test.go'
rg -n 'if\s+(config\.Config\.DebugMode|Config\.DebugMode)\s*\{' . --glob '!vendor/**' --glob '!docs/**'
```

Expected:

- `go test ./...` passes
- the first `rg` command returns no repository-owned runtime matches
- the second `rg` command returns no pure `DebugMode` log-branch matches; remaining `DebugMod` uses are limited to downstream behavior or non-logging logic

- [ ] **Step 5: Commit the final collector migration and policy closure**

Run:

```bash
git add inputs pkg/logging/repository_policy_test.go
git commit -m "refactor: complete repository-wide klog migration"
```

Expected: One commit containing the heavyweight subtree migration plus the final policy scope.

## Self-Review

- Spec coverage: the plan includes shared `klog` setup, startup integration, structured logger reuse for `inputs`, core/runtime migration, service/logs migration, inputs migration, and final repository verification
- Placeholder scan: no `TODO`, `TBD`, or “similar to previous task” references remain; each task names exact files, commands, and concrete code patterns
- Type consistency: the plan consistently uses `pkg/logging.RegisterFlags`, `pkg/logging.Configure`, `pkg/logging.Sync`, `logging.Component`, and `logging.ComponentValues` across tasks

