package snmp_zabbix

import (
	"time"
)

// ScheduledTask represents a group of items to be collected from a single agent at a specific interval.
// It serves as the element in the Priority Queue.
type ScheduledTask struct {
	Agent    string
	Interval time.Duration
	NextRun  time.Time // Priority Key

	// Payload: Items belonging to this (Agent, Interval) group
	Items []MonitorItem

	// Heap Index implementation (managed by heap.Interface)
	index int
}

// ItemHeap implements heap.Interface for []*ScheduledTask
type ItemHeap []*ScheduledTask

func (pq ItemHeap) Len() int { return len(pq) }

func (pq ItemHeap) Less(i, j int) bool {
	// Min-Heap: The item with the earliest NextRun comes first.
	return pq[i].NextRun.Before(pq[j].NextRun)
}

func (pq ItemHeap) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *ItemHeap) Push(x interface{}) {
	n := len(*pq)
	item := x.(*ScheduledTask)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *ItemHeap) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}
