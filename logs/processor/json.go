//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/logs/message"
)

const nanoToMillis = 1000000

// JSONEncoder is a shared json encoder.
var JSONEncoder Encoder = &jsonEncoder{}

// jsonEncoder transforms a message into a JSON byte array.
type jsonEncoder struct{}

// JSON representation of a message.
type jsonPayload struct {
	Message   string `json:"message"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
	Hostname  string `json:"agent_hostname"`
	Service   string `json:"fcservice"`
	Source    string `json:"fcsource"`
	Tags      string `json:"fctags"`
	Topic     string `json:"topic"`
	MsgKey    string `json:"msg_key"`
}

// Encode encodes a message into a JSON byte array.
func (j *jsonEncoder) Encode(msg *message.Message, redactedMsg []byte) ([]byte, error) {
	ts := time.Now().UTC()
	if !msg.Timestamp.IsZero() {
		ts = msg.Timestamp
	}
	accuracy := config.Config.Logs.Accuracy
	if msg.Origin.LogSource.Config.Accuracy != "" {
		accuracy = msg.Origin.LogSource.Config.Accuracy
	}
	if accuracy == "" {
		accuracy = "ms"
	}
	timestamp := ts.UnixMilli()
	switch accuracy {
	case "s":
		timestamp = timestamp / 1000
	case "m":
		timestamp = timestamp / 60000
	}
	topic := config.Config.Logs.Topic
	if msg.Origin.LogSource.Config.Topic != "" {
		topic = msg.Origin.LogSource.Config.Topic
	}
	msgKey := config.Config.Logs.APIKey
	if config.Config.Logs.SendType == "kafka" {
		msgKey = msg.GetHostname() + "/" + msg.Origin.GetIdentifier()
	}

	return json.Marshal(jsonPayload{
		Message:   toValidUtf8(redactedMsg),
		Status:    msg.GetStatus(),
		Timestamp: timestamp,
		Hostname:  msg.GetHostname(),
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      msg.Origin.TagsToJsonString(),
		Topic:     topic,
		MsgKey:    msgKey,
	})
}
