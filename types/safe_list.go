package types

import (
	"container/list"
	"sync"
)

// SafeList is a thread-safe list
type SafeList[T any] struct {
	sync.RWMutex
	L *list.List
}

func NewSafeList[T any]() *SafeList[T] {
	return &SafeList[T]{L: list.New()}
}

func (sl *SafeList[T]) PushFront(v T) *list.Element {
	sl.Lock()
	e := sl.L.PushFront(v)
	sl.Unlock()
	return e
}

func (sl *SafeList[T]) PushFrontN(vs []T) {
	sl.Lock()
	for _, item := range vs {
		sl.L.PushFront(item)
	}
	sl.Unlock()
}

func (sl *SafeList[T]) PopBack() *T {
	sl.Lock()
	defer sl.Unlock()
	if elem := sl.L.Back(); elem != nil {
		item := sl.L.Remove(elem)
		v, ok := item.(T)
		if !ok {
			return nil
		}
		return &v
	}
	return nil
}

func (sl *SafeList[T]) PopBackN(n int) []T {
	sl.Lock()
	defer sl.Unlock()

	count := sl.L.Len()
	if count == 0 {
		return nil
	}

	if count > n {
		count = n
	}

	items := make([]T, 0, count)
	for i := 0; i < count; i++ {
		data := sl.L.Remove(sl.L.Back())
		item, ok := data.(T)
		if ok {
			items = append(items, item)
		}
	}
	return items
}

func (sl *SafeList[T]) PopBackAll() []T {
	sl.Lock()
	defer sl.Unlock()
	count := sl.L.Len()
	if count == 0 {
		return nil
	}

	items := make([]T, 0, count)
	for i := 0; i < count; i++ {
		data := sl.L.Remove(sl.L.Back())
		item, ok := data.(T)
		if ok {
			items = append(items, item)
		}
	}
	return items
}

func (sl *SafeList[T]) RemoveAll() {
	sl.Lock()
	sl.L.Init()
	sl.Unlock()
}

func (sl *SafeList[T]) Len() int {
	sl.RLock()
	size := sl.L.Len()
	sl.RUnlock()
	return size
}

func (sl *SafeList[T]) PushFrontIfNotFull(v T, maxSize int) bool {
	sl.Lock()
	defer sl.Unlock()
	if sl.L.Len() >= maxSize {
		return false
	}
	sl.L.PushFront(v)
	return true
}

func (sl *SafeList[T]) PushFrontNIfNotFull(vs []T, maxSize int) bool {
	if len(vs) == 0 {
		return true
	}
	sl.Lock()
	defer sl.Unlock()
	if sl.L.Len()+len(vs) > maxSize {
		return false
	}
	for _, item := range vs {
		sl.L.PushFront(item)
	}
	return true
}

// SafeListLimited is SafeList with Limited Size
type SafeListLimited[T any] struct {
	maxSize int
	SL      *SafeList[T]
}

func NewSafeListLimited[T any](maxSize int) *SafeListLimited[T] {
	return &SafeListLimited[T]{SL: NewSafeList[T](), maxSize: maxSize}
}

func (sll *SafeListLimited[T]) PushFront(v T) bool {
	return sll.SL.PushFrontIfNotFull(v, sll.maxSize)
}

func (sll *SafeListLimited[T]) PushFrontN(vs []T) bool {
	return sll.SL.PushFrontNIfNotFull(vs, sll.maxSize)
}

func (sll *SafeListLimited[T]) PopBack() *T {
	return sll.SL.PopBack()
}

func (sll *SafeListLimited[T]) PopBackN(n int) []T {
	return sll.SL.PopBackN(n)
}

func (sll *SafeListLimited[T]) PopBackAll() []T {
	return sll.SL.PopBackAll()
}

func (sll *SafeListLimited[T]) RemoveAll() {
	sll.SL.RemoveAll()
}

func (sll *SafeListLimited[T]) Len() int {
	return sll.SL.Len()
}
