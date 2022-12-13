package types

import (
	"container/list"
)

type SampleList struct {
	SafeList[*Sample]
}

func NewSampleList() *SampleList {
	return &SampleList{*NewSafeList[*Sample]()}
}

func (l *SampleList) PushSample(prefix, metric string, value interface{}, labels ...map[string]string) *list.Element {
	v := NewSample(prefix, metric, value, labels...)
	e := l.PushFront(v)
	return e
}

func (l *SampleList) PushSamples(prefix string, fields map[string]interface{}, labels ...map[string]string) {
	vs := make([]*Sample, 0, len(fields))
	for metric, value := range fields {
		v := NewSample(prefix, metric, value, labels...)
		vs = append(vs, v)
	}
	l.PushFrontN(vs)
}
