//go:build !no_logs

package agent

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	coreconfig "flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
)

var logsEndpoints = map[string]int{
	"agent-http.logs.flashcat.cloud": 443,
	"agent-tcp.logs.flashcat.cloud":  8848,
}

// logs-intake endpoint prefix.
const (
	tcpEndpointPrefix  = "agent-tcp.logs"
	httpEndpointPrefix = "agent-http.logs."
)

// BuildEndpoints returns the endpoints to send logs.
func BuildEndpoints(intakeTrackType logsconfig.IntakeTrackType, intakeProtocol logsconfig.IntakeProtocol, intakeOrigin logsconfig.IntakeOrigin) (*logsconfig.Endpoints, error) {
	return BuildEndpointsWithConfig(httpEndpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildEndpointsWithConfig returns the endpoints to send logs.
func BuildEndpointsWithConfig(endpointPrefix string, intakeTrackType logsconfig.IntakeTrackType, intakeProtocol logsconfig.IntakeProtocol, intakeOrigin logsconfig.IntakeOrigin) (*logsconfig.Endpoints, error) {
	logsConfig := coreconfig.Config.Logs

	switch logsConfig.SendType {
	case "http":
		return BuildHTTPEndpointsWithConfig(endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	case "tcp":
		return buildTCPEndpoints(logsConfig)
	case "kafka":
		return buildKafkaEndpoints(logsConfig)

	}
	return buildTCPEndpoints(logsConfig)
}

func buildKafkaEndpoints(logsConfig coreconfig.Logs) (*logsconfig.Endpoints, error) {
	// return nil, nil
	// Provide default values for legacy settings when the configuration key does not exist
	defaultTLS := coreconfig.Config.Logs.SendWithTLS

	main := logsconfig.Endpoint{
		APIKey:                  strings.TrimSpace(logsConfig.APIKey),
		UseCompression:          logsConfig.UseCompression,
		CompressionLevel:        logsConfig.CompressionLevel,
		ConnectionResetInterval: 0,
		BackoffBase:             1.0,
		BackoffMax:              120.0,
		BackoffFactor:           2.0,
		RecoveryInterval:        2,
		RecoveryReset:           false,
		Addr:                    logsConfig.SendTo,
		Topic:                   logsConfig.Topic,
	}

	if intakeTrackType != "" {
		main.Version = logsconfig.EPIntakeVersion2
		main.TrackType = intakeTrackType
	} else {
		main.Version = logsconfig.EPIntakeVersion1
	}

	if len(logsConfig.SendTo) != 0 {
		brokers := strings.Split(logsConfig.SendTo, ",")
		if len(brokers) == 0 {
			return nil, fmt.Errorf("wrong send_to content %s", logsConfig.SendTo)
		}
		host, port, err := parseAddress(brokers[0])
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsConfig.SendTo, err)
		}
		main.Host = host
		main.Port = port
		main.UseSSL = defaultTLS
	} else {
		return nil, fmt.Errorf("empty send_to is not allowed when send_type is kafka")
	}
	return NewEndpoints(main, false, "kafka"), nil
}

func buildTCPEndpoints(logsConfig coreconfig.Logs) (*logsconfig.Endpoints, error) {
	main := logsconfig.Endpoint{
		APIKey:                  logsConfig.APIKey,
		ProxyAddress:            "",
		ConnectionResetInterval: 0,
	}

	if len(logsConfig.SendTo) != 0 {
		host, port, err := parseAddress(logsConfig.SendTo)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsConfig.SendTo, err)
		}
		main.Host = host
		main.Port = port
		main.UseSSL = logsConfig.SendWithTLS
	} else {
		main.Host = "agent-tcp.logs.flashcat.cloud"
		main.Port = logsEndpoints[main.Host]
		main.UseSSL = logsConfig.SendWithTLS
	}

	return NewEndpoints(main, false, "tcp"), nil
}

// BuildHTTPEndpoints returns the HTTP endpoints to send logs to.
func BuildHTTPEndpoints(intakeTrackType logsconfig.IntakeTrackType, intakeProtocol logsconfig.IntakeProtocol, intakeOrigin logsconfig.IntakeOrigin) (*logsconfig.Endpoints, error) {
	return BuildHTTPEndpointsWithConfig(httpEndpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
}

// BuildHTTPEndpointsWithConfig uses two arguments that instructs it how to access configuration parameters, then returns the HTTP endpoints to send logs to. This function is able to default to the 'classic' BuildHTTPEndpoints() w ldHTTPEndpointsWithConfigdefault variables logsConfigDefaultKeys and httpEndpointPrefix
func BuildHTTPEndpointsWithConfig(endpointPrefix string, intakeTrackType logsconfig.IntakeTrackType, intakeProtocol logsconfig.IntakeProtocol, intakeOrigin logsconfig.IntakeOrigin) (*logsconfig.Endpoints, error) {
	// Provide default values for legacy settings when the configuration key does not exist
	logsConfig := coreconfig.Config.Logs
	defaultTLS := coreconfig.Config.Logs.SendWithTLS

	main := logsconfig.Endpoint{
		APIKey:                  strings.TrimSpace(logsConfig.APIKey),
		UseCompression:          logsConfig.UseCompression,
		CompressionLevel:        logsConfig.CompressionLevel,
		ConnectionResetInterval: 0,
		BackoffBase:             1.0,
		BackoffMax:              120.0,
		BackoffFactor:           2.0,
		RecoveryInterval:        2,
		RecoveryReset:           false,
	}

	if intakeTrackType != "" {
		main.Version = logsconfig.EPIntakeVersion2
		main.TrackType = intakeTrackType
		main.Protocol = intakeProtocol
		main.Origin = intakeOrigin
	} else {
		main.Version = logsconfig.EPIntakeVersion1
	}

	if len(logsConfig.SendTo) != 0 {
		host, port, err := parseAddress(logsConfig.SendTo)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s: %v", logsConfig.SendTo, err)
		}
		main.Host = host
		main.Port = port
		main.UseSSL = defaultTLS
	} else {
		main.Host = logsConfig.SendTo
		main.UseSSL = defaultTLS
	}

	batchWait := time.Duration(logsConfig.BatchWait) * time.Second
	// TODO support custom param
	batchMaxConcurrentSend := 0
	batchMaxSize := 100
	batchMaxContentSize := 1000000

	return NewEndpointsWithBatchSettings(main, false, "http", batchWait, batchMaxConcurrentSend, batchMaxSize, batchMaxContentSize), nil
}

// parseAddress returns the host and the port of the address.
func parseAddress(address string) (string, int, error) {
	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// NewEndpoints returns a new endpoints composite with default batching settings
func NewEndpoints(main logsconfig.Endpoint, useProto bool, typ string) *logsconfig.Endpoints {
	logsConfig := coreconfig.Config.Logs
	return &logsconfig.Endpoints{
		Main:        main,
		Additionals: nil,
		UseProto:    useProto,
		Type:        typ,
		BatchWait:   time.Duration(logsConfig.BatchWait) * time.Second,
		// TODO support custom param
		BatchMaxConcurrentSend: 0,
		BatchMaxSize:           100,
		BatchMaxContentSize:    1000000,
	}
}

// NewEndpointsWithBatchSettings returns a new endpoints composite with non-default batching settings specified
func NewEndpointsWithBatchSettings(main logsconfig.Endpoint, useProto bool, typ string, batchWait time.Duration, batchMaxConcurrentSend int, batchMaxSize int, batchMaxContentSize int) *logsconfig.Endpoints {
	return &logsconfig.Endpoints{
		Main:                   main,
		Additionals:            nil,
		UseProto:               useProto,
		Type:                   typ,
		BatchWait:              batchWait,
		BatchMaxConcurrentSend: batchMaxConcurrentSend,
		BatchMaxSize:           batchMaxSize,
		BatchMaxContentSize:    batchMaxContentSize,
	}
}
