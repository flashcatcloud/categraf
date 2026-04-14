package agent

import (
	"reflect"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	klog "k8s.io/klog/v2"
)

type testMetricsInput struct {
	instances      []inputs.Instance
	initCalls      int
	loggerInit     int
	logger         klog.Logger
	internalConfig int
}

func (t *testMetricsInput) Clone() inputs.Input {
	return t
}

func (t *testMetricsInput) Name() string {
	return "test"
}

func (t *testMetricsInput) GetLabels() map[string]string {
	return nil
}

func (t *testMetricsInput) GetInterval() config.Duration {
	return 0
}

func (t *testMetricsInput) InitInternalConfig() error {
	t.internalConfig++
	return nil
}

func (t *testMetricsInput) Process(slist *types.SampleList) *types.SampleList {
	return slist
}

func (t *testMetricsInput) Init() error {
	t.initCalls++
	return nil
}

func (t *testMetricsInput) InitWithLogger(logger klog.Logger) error {
	t.loggerInit++
	t.logger = logger
	return nil
}

func (t *testMetricsInput) GetInstances() []inputs.Instance {
	return t.instances
}

type testMetricsInstance struct {
	initialized    bool
	internalConfig int
	loggerInit     int
	logger         klog.Logger
	labels         map[string]string
}

func (t *testMetricsInstance) Initialized() bool {
	return t.initialized
}

func (t *testMetricsInstance) SetInitialized() {
	t.initialized = true
}

func (t *testMetricsInstance) GetLabels() map[string]string {
	return t.labels
}

func (t *testMetricsInstance) GetIntervalTimes() int64 {
	return 0
}

func (t *testMetricsInstance) InitInternalConfig() error {
	t.internalConfig++
	return nil
}

func (t *testMetricsInstance) Process(slist *types.SampleList) *types.SampleList {
	return slist
}

func (t *testMetricsInstance) InitWithLogger(logger klog.Logger) error {
	t.loggerInit++
	t.logger = logger
	return nil
}

type testLegacyMetricsInstance struct {
	initialized    bool
	internalConfig int
	initCalls      int
}

func (t *testLegacyMetricsInstance) Initialized() bool {
	return t.initialized
}

func (t *testLegacyMetricsInstance) SetInitialized() {
	t.initialized = true
}

func (t *testLegacyMetricsInstance) GetLabels() map[string]string {
	return nil
}

func (t *testLegacyMetricsInstance) GetIntervalTimes() int64 {
	return 0
}

func (t *testLegacyMetricsInstance) InitInternalConfig() error {
	t.internalConfig++
	return nil
}

func (t *testLegacyMetricsInstance) Process(slist *types.SampleList) *types.SampleList {
	return slist
}

func (t *testLegacyMetricsInstance) Init() error {
	t.initCalls++
	return nil
}

type testEmptyMetricsInput struct {
	initCalled bool
}

func (t *testEmptyMetricsInput) Clone() inputs.Input {
	return t
}

func (t *testEmptyMetricsInput) Name() string {
	return "empty"
}

func (t *testEmptyMetricsInput) GetLabels() map[string]string {
	return nil
}

func (t *testEmptyMetricsInput) GetInterval() config.Duration {
	return 0
}

func (t *testEmptyMetricsInput) InitInternalConfig() error {
	return nil
}

func (t *testEmptyMetricsInput) Process(slist *types.SampleList) *types.SampleList {
	return slist
}

func (t *testEmptyMetricsInput) InitWithLogger(klog.Logger) error {
	t.initCalled = true
	return types.ErrInstancesEmpty
}

type testLegacyTopLevelMetricsInput struct {
	initCalls int
}

func (t *testLegacyTopLevelMetricsInput) Clone() inputs.Input {
	return t
}

func (t *testLegacyTopLevelMetricsInput) Name() string {
	return "legacy-top-level"
}

func (t *testLegacyTopLevelMetricsInput) GetLabels() map[string]string {
	return nil
}

func (t *testLegacyTopLevelMetricsInput) GetInterval() config.Duration {
	return 0
}

func (t *testLegacyTopLevelMetricsInput) InitInternalConfig() error {
	return nil
}

func (t *testLegacyTopLevelMetricsInput) Process(slist *types.SampleList) *types.SampleList {
	return slist
}

func (t *testLegacyTopLevelMetricsInput) Init() error {
	t.initCalls++
	return nil
}

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

	if input.internalConfig != 1 {
		t.Fatalf("expected input internal config once, got %d", input.internalConfig)
	}
	if input.loggerInit != 1 {
		t.Fatalf("expected input logger init once, got %d", input.loggerInit)
	}
	if input.initCalls != 0 {
		t.Fatalf("expected legacy input init to be skipped, got %d", input.initCalls)
	}
	if instance.internalConfig != 1 {
		t.Fatalf("expected instance internal config once, got %d", instance.internalConfig)
	}
	if instance.loggerInit != 1 {
		t.Fatalf("expected instance logger init once, got %d", instance.loggerInit)
	}
	if !instance.initialized {
		t.Fatal("expected instance to be marked initialized")
	}
	readers, ok := agent.InputReaders.GetInput("provider.demo")
	if !ok {
		t.Fatal("expected input reader to be registered")
	}
	reader, ok := readers["sum"]
	if !ok {
		t.Fatal("expected checksum reader to be registered")
	}
	defer reader.Stop()
	var zeroLogger klog.Logger
	if input.logger == zeroLogger {
		t.Fatal("expected input logger to be set")
	}
	if instance.logger == zeroLogger {
		t.Fatal("expected instance logger to be set")
	}
}

func TestMetricsAgentInputGoInitializesLegacyInput(t *testing.T) {
	restore := setupMetricsAgentTestConfig()
	defer restore()

	agent := &MetricsAgent{
		InputReaders: NewReaders(),
	}
	input := &testLegacyTopLevelMetricsInput{}

	agent.inputGo("provider.demo", "legacy-top-level", input)

	if input.initCalls != 1 {
		t.Fatalf("expected legacy input Init once, got %d", input.initCalls)
	}
	readers, ok := agent.InputReaders.GetInput("provider.demo")
	if !ok {
		t.Fatal("expected input reader to be registered")
	}
	reader, ok := readers["legacy-top-level"]
	if !ok {
		t.Fatal("expected legacy input checksum reader to be registered")
	}
	defer reader.Stop()
}

func TestMetricsAgentInputGoInitializesLegacyInstance(t *testing.T) {
	restore := setupMetricsAgentTestConfig()
	defer restore()

	agent := &MetricsAgent{
		InputReaders: NewReaders(),
	}
	instance := &testLegacyMetricsInstance{}
	input := &testMetricsInput{
		instances: []inputs.Instance{instance},
	}

	agent.inputGo("provider.demo", "legacy-sum", input)

	if input.loggerInit != 1 {
		t.Fatalf("expected input logger init once, got %d", input.loggerInit)
	}
	if instance.internalConfig != 1 {
		t.Fatalf("expected legacy instance internal config once, got %d", instance.internalConfig)
	}
	if instance.initCalls != 1 {
		t.Fatalf("expected legacy instance Init once, got %d", instance.initCalls)
	}
	if !instance.initialized {
		t.Fatal("expected legacy instance to be marked initialized")
	}
	readers, ok := agent.InputReaders.GetInput("provider.demo")
	if !ok {
		t.Fatal("expected input reader to be registered")
	}
	reader, ok := readers["legacy-sum"]
	if !ok {
		t.Fatal("expected checksum reader to be registered")
	}
	defer reader.Stop()
}

func TestMetricsAgentLoggerContextValues(t *testing.T) {
	inputContext := metricsAgentInputLoggerValues("provider.demo", "sum")
	wantInputContext := []interface{}{
		"component", "inputs",
		"input", "provider.demo",
		"plugin", "demo",
		"checksum", "sum",
	}
	if !reflect.DeepEqual(inputContext, wantInputContext) {
		t.Fatalf("unexpected input logger context: got %#v want %#v", inputContext, wantInputContext)
	}

	instanceContext := metricsAgentInstanceLoggerValues(2, map[string]string{"target": "demo"})
	wantInstanceContext := []interface{}{
		"instance_index", 2,
		"instance_target", "demo",
	}
	if !reflect.DeepEqual(instanceContext, wantInstanceContext) {
		t.Fatalf("unexpected instance logger context: got %#v want %#v", instanceContext, wantInstanceContext)
	}
}

func TestMetricsAgentInputGoKeepsErrInstancesEmptyBehavior(t *testing.T) {
	restore := setupMetricsAgentTestConfig()
	defer restore()

	agent := &MetricsAgent{
		InputReaders: NewReaders(),
	}
	input := &testEmptyMetricsInput{}

	agent.inputGo("provider.empty", "sum", input)

	if !input.initCalled {
		t.Fatal("expected input init to be attempted")
	}
	if _, ok := agent.InputReaders.GetInput("provider.empty"); ok {
		t.Fatal("expected no readers to be registered for empty instances")
	}
}

func setupMetricsAgentTestConfig() func() {
	prevConfig := config.Config
	config.Config = &config.ConfigType{
		TestMode: true,
		Global: config.Global{
			Interval:    config.Duration(time.Hour),
			Concurrency: 1,
			Precision:   "ms",
		},
	}
	return func() {
		if prevConfig == nil {
			config.Config = &config.ConfigType{
				Global: config.Global{
					Interval:    config.Duration(time.Hour),
					Concurrency: 1,
					Precision:   "ms",
				},
			}
			return
		}
		config.Config = prevConfig
	}
}
