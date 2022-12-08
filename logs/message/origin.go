//go:build !no_logs

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"encoding/json"
	"log"
	"strings"

	logsconfig "flashcat.cloud/categraf/config/logs"
)

// Origin represents the Origin of a message
type Origin struct {
	Identifier string
	LogSource  *logsconfig.LogSource
	Offset     string
	service    string
	source     string
	tags       []string
}

// NewOrigin returns a new Origin
func NewOrigin(source *logsconfig.LogSource) *Origin {
	return &Origin{
		LogSource: source,
	}
}

// Tags returns the tags of the origin.
func (o *Origin) Tags() []string {
	return o.tagsToStringArray()
}

// TagsPayload returns the raw tag payload of the origin.
func (o *Origin) TagsPayload() []byte {
	var tagsPayload []byte

	source := o.Source()
	if source != "" {
		tagsPayload = append(tagsPayload, []byte("[fc fcsource=\""+source+"\"]")...)
	}
	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" {
		tagsPayload = append(tagsPayload, []byte("[fc fcsourcecategory=\""+sourceCategory+"\"]")...)
	}

	var tags []string
	tags = append(tags, o.LogSource.Config.Tags...)
	tags = append(tags, o.tags...)

	if len(tags) > 0 {
		tagsPayload = append(tagsPayload, []byte("[fc fctags=\""+strings.Join(tags, ",")+"\"]")...)
	}
	if len(tagsPayload) == 0 {
		tagsPayload = []byte{}
	}
	return tagsPayload
}

func (o *Origin) TagsToJsonString() string {
	tagsMap := make(map[string]string)
	tags := append(o.tags, o.LogSource.Config.Tags...)
	for _, tag := range tags {
		pair := strings.FieldsFunc(tag, func(r rune) bool {
			return r == '=' || r == ':'
		})
		if len(pair) == 2 {
			tagsMap[pair[0]] = pair[1]
		}
	}
	ret := ""
	if len(tagsMap) != 0 {
		data, err := json.Marshal(tagsMap)
		if err != nil {
			log.Println("marshal tags error:", err)
			return ret
		}
		ret = string(data)
	}
	return ret
}

// TagsToString encodes tags to a single string, in a comma separated format
func (o *Origin) TagsToString() string {
	tags := o.tagsToStringArray()

	if tags == nil {
		return ""
	}

	return strings.Join(tags, ",")
}

func (o *Origin) tagsToStringArray() []string {
	tags := o.tags

	sourceCategory := o.LogSource.Config.SourceCategory
	if sourceCategory != "" {
		tags = append(tags, "sourcecategory"+":"+sourceCategory)
	}

	tags = append(tags, o.LogSource.Config.Tags...)

	return tags
}

// SetTags sets the tags of the origin.
func (o *Origin) SetTags(tags []string) {
	o.tags = tags
}

// SetSource sets the source of the origin.
func (o *Origin) SetSource(source string) {
	o.source = source
}

// Source returns the source of the configuration if set or the source of the message,
// if none are defined, returns an empty string by default.
func (o *Origin) Source() string {
	if o.LogSource.Config.Source != "" {
		return o.LogSource.Config.Source
	}
	return o.source
}

// SetService sets the service of the origin.
func (o *Origin) SetService(service string) {
	o.service = service
}

// Service returns the service of the configuration if set or the service of the message,
// if none are defined, returns an empty string by default.
func (o *Origin) Service() string {
	if o.LogSource.Config.Service != "" {
		return o.LogSource.Config.Service
	}
	return o.service
}
