package types

import (
	"container/list"
	"sync"
)

type SampleList struct {
	sync.RWMutex
	L *list.List
}

func NewSampleList() *SampleList {
	return &SampleList{L: list.New()}
}

func (l *SampleList) PushSample(prefix, metric string, value interface{}, labels ...map[string]string) *list.Element {
	l.Lock()
	v := NewSample(prefix, metric, value, labels...)
	e := l.L.PushFront(v)
	l.Unlock()
	return e
}

func (l *SampleList) PushSamples(prefix string, fields map[string]interface{}, labels ...map[string]string) {
	l.Lock()
	for metric, value := range fields {
		v := NewSample(prefix, metric, value, labels...)
		l.L.PushFront(v)
	}
	l.Unlock()
}

func (l *SampleList) PushFront(v *Sample) *list.Element {
	l.Lock()
	e := l.L.PushFront(v)
	l.Unlock()
	return e
}

func (l *SampleList) PushFrontBatch(vs []*Sample) {
	l.Lock()
	for i := 0; i < len(vs); i++ {
		l.L.PushFront(vs[i])
	}
	l.Unlock()
}

func (l *SampleList) PopBack() *Sample {
	l.Lock()

	if elem := l.L.Back(); elem != nil {
		item := l.L.Remove(elem)
		l.Unlock()
		v, ok := item.(*Sample)
		if !ok {
			return nil
		}
		return v
	}

	l.Unlock()
	return nil
}

func (l *SampleList) PopBackBy(max int) []*Sample {
	l.Lock()

	count := l.len()
	if count == 0 {
		l.Unlock()
		return []*Sample{}
	}

	if count > max {
		count = max
	}

	items := make([]*Sample, 0, count)
	for i := 0; i < count; i++ {
		item := l.L.Remove(l.L.Back())
		v, ok := item.(*Sample)
		if ok {
			items = append(items, v)
		}
	}

	l.Unlock()
	return items
}

func (l *SampleList) PopBackAll() []*Sample {
	l.Lock()

	count := l.len()
	if count == 0 {
		l.Unlock()
		return []*Sample{}
	}

	items := make([]*Sample, 0, count)
	for i := 0; i < count; i++ {
		item := l.L.Remove(l.L.Back())
		v, ok := item.(*Sample)
		if ok {
			items = append(items, v)
		}
	}

	l.Unlock()
	return items
}

func (l *SampleList) Remove(e *list.Element) *Sample {
	l.Lock()
	defer l.Unlock()
	item := l.L.Remove(e)
	v, ok := item.(*Sample)
	if ok {
		return v
	}
	return nil
}

func (l *SampleList) RemoveAll() {
	l.Lock()
	l.L = list.New()
	l.Unlock()
}

func (l *SampleList) FrontAll() []*Sample {
	l.RLock()
	defer l.RUnlock()

	count := l.len()
	if count == 0 {
		return []*Sample{}
	}

	items := make([]*Sample, 0, count)
	for e := l.L.Front(); e != nil; e = e.Next() {
		v, ok := e.Value.(*Sample)
		if ok {
			items = append(items, v)
		}
	}
	return items
}

func (l *SampleList) BackAll() []*Sample {
	l.RLock()
	defer l.RUnlock()

	count := l.len()
	if count == 0 {
		return []*Sample{}
	}

	items := make([]*Sample, 0, count)
	for e := l.L.Back(); e != nil; e = e.Prev() {
		v, ok := e.Value.(*Sample)
		if ok {
			items = append(items, v)
		}
	}
	return items
}

func (l *SampleList) Front() *Sample {
	l.RLock()

	if f := l.L.Front(); f != nil {
		l.RUnlock()
		v, ok := f.Value.(*Sample)
		if ok {
			return v
		}
		return nil
	}

	l.RUnlock()
	return nil
}

func (l *SampleList) Len() int {
	l.RLock()
	defer l.RUnlock()
	return l.len()
}

func (l *SampleList) len() int {
	return l.L.Len()
}
