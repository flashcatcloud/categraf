package modbus

import (
	"fmt"
	"testing"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/types"
	"github.com/stretchr/testify/require"
	"github.com/tbrandon/mbserver"
)

func TestParseConfigurationOriginal(t *testing.T) {
	ins := &Instance{
		Name:              "test",
		Controller:        "tcp://localhost:502",
		TransmissionMode:  "TCP",
		Timeout:           config.Duration(time.Second),
		ConfigurationType: "register",
	}

	err := ins.Init()
	require.NoError(t, err)
}

func TestParseConfigurationMetric(t *testing.T) {
	ins := &Instance{
		Name:              "test_metric",
		Controller:        "tcp://localhost:502",
		TransmissionMode:  "TCP",
		Timeout:           config.Duration(time.Second),
		ConfigurationType: "metric",
	}

	err := ins.Init()
	require.NoError(t, err)
}

func TestParseConfigurationRequest(t *testing.T) {
	ins := &Instance{
		Name:              "test_request",
		Controller:        "tcp://localhost:502",
		TransmissionMode:  "TCP",
		Timeout:           config.Duration(time.Second),
		ConfigurationType: "request",
	}

	err := ins.Init()
	require.NoError(t, err)
}

func TestGather(t *testing.T) {

	// Mock global config for Process() since it expands agent_hostname
	t.Setenv("HOSTIP", "127.0.0.1")
	config.InitHostInfo()
	oldConfig := config.Config
	t.Cleanup(func() {
		config.Config = oldConfig
	})
	config.Config = &config.ConfigType{
		Global: config.Global{
			Hostname: "test_host",
		},
	}

	serv := mbserver.NewServer()
	// Dynamically allocate port for parallel tests
	var port int
	var err error
	for port = 15020; port < 16000; port++ {
		err = serv.ListenTCP(fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			break
		}
	}
	require.NoError(t, err, "failed to bind to any port for mbserver")
	defer serv.Close()

	// Put some data in registers
	// Holding registers:
	// 0: 123 (UINT16)
	// 1-2: "AB\x00\x00" (STRING, length 2 words)
	serv.HoldingRegisters[0] = 123
	serv.HoldingRegisters[1] = ('A' << 8) | 'B'
	serv.HoldingRegisters[2] = 0

	ins := &Instance{
		InstanceConfig: config.InstanceConfig{
			InternalConfig: config.InternalConfig{
				Labels: map[string]string{
					"instance_label": "should_exist",
					"name":           "override_name",
				},
			},
		},
		Name:              "test_gather",
		Controller:        fmt.Sprintf("tcp://127.0.0.1:%d", port),
		TransmissionMode:  "TCP",
		Timeout:           config.Duration(time.Second),
		ConfigurationType: "metric",
	}

	// Metric config embedded directly
	ins.Metrics = []metricDefinition{
		{
			SlaveID:     1,
			ByteOrder:   "ABCD",
			Measurement: "hvac",
			Tags: map[string]string{
				"custom_tag": "value",
			},
			Fields: []metricFieldDefinition{
				{
					RegisterType: "holding",
					Address:      0,
					Name:         "temperature",
					InputType:    "UINT16",
				},
				{
					RegisterType: "holding",
					Address:      1,
					Length:       2,
					Name:         "device_name",
					InputType:    "STRING",
				},
			},
		},
	}

	err = ins.Init()
	require.NoError(t, err)

	slist := types.NewSampleList()
	ins.Gather(slist)
	slist = ins.Process(slist)

	samples := slist.PopBackAll()
	require.NotEmpty(t, samples)

	var tempSample, nameSample *types.Sample
	for _, s := range samples {
		if s.Metric == "hvac_temperature" {
			tempSample = s
		} else if s.Metric == "hvac_device_name" {
			nameSample = s
		}
	}

	require.NotNil(t, tempSample)
	require.NotNil(t, nameSample)

	// Check values
	require.Equal(t, uint64(123), tempSample.Value)
	require.Equal(t, "AB", nameSample.Value) // NUL cutoff test

	// Check labels (custom tags override + instance labels)
	require.Equal(t, "value", tempSample.Labels["custom_tag"])
	require.Equal(t, "should_exist", tempSample.Labels["instance_label"])
	require.Equal(t, "override_name", tempSample.Labels["name"])
}
