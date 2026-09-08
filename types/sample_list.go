package types

import (
	"container/list"
	"reflect"
	"time"
)

type SampleList struct {
	SafeList[*Sample]
	// Timestamp is the default time for samples without an explicit timestamp.
	// Periodic inputs set it to the scheduled collection time.
	Timestamp time.Time
}

func NewSampleList() *SampleList {
	return &SampleList{SafeList: *NewSafeList[*Sample]()}
}

func (l *SampleList) PushSample(prefix, metric string, value interface{}, labels ...map[string]string) *list.Element {
	v := NewSample(prefix, metric, value, labels...)
	e := l.PushFront(v)
	return e
}

func (l *SampleList) PushSamples(prefix string, fields map[string]interface{}, labels ...map[string]string) {
	vs := make([]*Sample, 0, len(fields))
	for metric, value := range fields {
		v := NewSample(prefix, metric, convertPtrToValue(value), labels...)
		vs = append(vs, v)
	}
	l.PushFrontN(vs)
}

func convertPtrToValue(value interface{}) interface{} {
	if value == nil {
		return value
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v.Interface()
}
