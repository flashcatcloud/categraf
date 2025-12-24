package snmp_zabbix

import (
	"container/heap"
	"context"
	"hash/fnv"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"flashcat.cloud/categraf/types"
)

type ItemScheduler struct {
	// pq is the Priority Queue (Min-Heap) of tasks, ordered by NextRun time.
	pq ItemHeap
	// taskMap allows O(1) lookup of tasks by key "Agent|Interval" for dynamic updates logic.
	// Key format: "Agent|IntervalNanoseconds"
	taskMap map[string]*ScheduledTask

	collector *SNMPCollector
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}

	ctx   context.Context
	slist *types.SampleList

	// discovered tracks individual items for LLD maintenance (TTL, etc).
	// Key: "Agent|ItemKey"
	discovered map[string]*ScheduledItem

	labelCache *LabelCache
}

// ScheduledItem holds metadata for a single item within LLD logic.
// Unlike the old scheduler, this is mostly for tracking state (Lost, Disabled)
// and is NOT the unit of scheduling. The unit of scheduling is ScheduledTask.
type ScheduledItem struct {
	Item       MonitorItem
	Interval   time.Duration
	IsLost     bool
	LostSince  time.Time
	IsDisabled bool
	DeleteTTL  time.Duration
	DisableTTL time.Duration
}

func NewItemScheduler(collector *SNMPCollector, labelCache *LabelCache) *ItemScheduler {
	return &ItemScheduler{
		pq:         make(ItemHeap, 0),
		taskMap:    make(map[string]*ScheduledTask),
		collector:  collector,
		stopCh:     make(chan struct{}),
		discovered: make(map[string]*ScheduledItem),
		labelCache: labelCache,
	}
}

func (s *ItemScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
}

func (s *ItemScheduler) Start(ctx context.Context, slist *types.SampleList) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	if s.stopCh == nil {
		s.stopCh = make(chan struct{})
	}
	s.running = true
	s.ctx = ctx
	s.slist = slist

	go s.runLoop(ctx)
	go s.runMaintainLoop(ctx)
}

// runLoop is the main event loop that processes the Heap.
func (s *ItemScheduler) runLoop(ctx context.Context) {
	// Re-check interval
	const idleWait = 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}

		s.mu.Lock()
		if s.pq.Len() == 0 {
			s.mu.Unlock()
			time.Sleep(idleWait)
			continue
		}

		// Peek at the first item
		task := s.pq[0]
		now := time.Now()

		if task.NextRun.After(now) {
			// Not ready yet
			wait := task.NextRun.Sub(now)
			// Cap the wait time to avoid long sleeps blocking shutdown (though we check stopCh above)
			if wait > 5*time.Second {
				wait = 5 * time.Second
			}
			s.mu.Unlock()
			time.Sleep(wait)
			continue
		}

		// Ready to execute
		// Pop it, update time, push it back (or remove if empty)
		if len(task.Items) == 0 {
			heap.Pop(&s.pq)
			delete(s.taskMap, s.taskKey(task.Agent, task.Interval))
			s.mu.Unlock()
			continue
		}

		// Execute
		// Because we pop and push, we must ensure concurrency safety.
		// We make a COPY of items to pass to the collector goroutine.
		itemsToCollect := make([]MonitorItem, len(task.Items))
		copy(itemsToCollect, task.Items)

		// Update NextRun and fix heap
		// Simple logic: NextRun += Interval.

		if task.Interval <= 0 {
			task.Interval = time.Minute
		}

		if task.NextRun.Before(now) {
			diff := now.Sub(task.NextRun)
			cycles := diff / task.Interval
			if cycles > 0 {
				task.NextRun = task.NextRun.Add(cycles * task.Interval)
			}
			if task.NextRun.Before(now) {
				task.NextRun = task.NextRun.Add(task.Interval)
			}
		}

		heap.Fix(&s.pq, task.index)

		s.mu.Unlock()

		// Async Execute
		go s.executeTask(ctx, task.Agent, itemsToCollect)
	}
}

func (s *ItemScheduler) executeTask(ctx context.Context, agent string, items []MonitorItem) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("E! [CRITICAL] collection goroutine for agent %s panicked: %v\n%s", agent, r, debug.Stack())
		}
	}()

	if s.slist == nil {
		return
	}

	if err := s.collector.CollectItems(ctx, items, s.slist); err != nil {
		log.Printf("Failed to collect items for agent %s: %v\n", agent, err)
	}
}

// taskKey generates a unique key for grouping tasks
func (s *ItemScheduler) taskKey(agent string, interval time.Duration) string {
	return agent + "|" + interval.String()
}

func (s *ItemScheduler) itemID(it MonitorItem) string {
	if it.Key != "" {
		return it.Agent + "|" + it.Key
	}
	return it.Agent + "|" + it.OID
}

// calcScatteredNextRun calculates the initial run time based on consistent hashing
func (s *ItemScheduler) calcScatteredNextRun(agent string, interval time.Duration) time.Time {
	if interval <= 0 {
		interval = time.Minute
	}
	h := fnv.New64a()
	h.Write([]byte(agent))
	// Hash is deterministic.
	// Offset is in [0, Interval)
	// We cast to int64 (Duration) which is safe as interval is usually < 290 years
	offset := time.Duration(h.Sum64() % uint64(interval))

	now := time.Now()
	// Align to the start of the current interval, then add the offset.
	// If that time is already in the past, advance to the next interval.
	base := now.Truncate(interval)
	nextRun := base.Add(offset)
	if nextRun.Before(now) {
		nextRun = nextRun.Add(interval)
	}
	return nextRun
}

// UpdateDiscoveredDiff handles LLD updates.
// It groups new items by (Agent, Interval) and syncs them with the Heap.
func (s *ItemScheduler) UpdateDiscoveredDiff(ruleKey string, newItems []MonitorItem, immediateOnExistingInterval bool, deleteTTL, disableTTL time.Duration) {
	now := time.Now()

	// 1. Normalize and Index New Items
	for i := range newItems {
		if newItems[i].Delay == 0 {
			newItems[i].Delay = 60 * time.Second
		}
		newItems[i].IsDiscovered = true
		newItems[i].DiscoveryRuleKey = ruleKey
	}

	newIdx := make(map[string]MonitorItem, len(newItems))
	for _, it := range newItems {
		newIdx[s.itemID(it)] = it
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 2. Identify Lost Items
	// Iterate valid existing items for this rule
	for id, sch := range s.discovered {
		if sch.Item.DiscoveryRuleKey == ruleKey {
			if _, ok := newIdx[id]; !ok {
				// Lost Logic
				if deleteTTL == 0 { // Immediately delete
					s.removeItemFromTask(sch)
					delete(s.discovered, id)
					if sch.Item.IsLabelProvider {
						s.labelCache.DeleteLabel(sch.Item.Agent, sch.Item.DiscoveryRuleKey, sch.Item.DiscoveryIndex, sch.Item.LabelKey)
					}
					continue
				}

				if !sch.IsLost {
					sch.IsLost = true
					sch.LostSince = now
					log.Printf("I! item marked as lost: %s", id)
				}
				sch.DeleteTTL = deleteTTL
				sch.DisableTTL = disableTTL

				if !sch.IsDisabled && disableTTL == 0 {
					s.removeItemFromTask(sch)
					sch.IsDisabled = true
					log.Printf("I! item disabled immediately: %s", id)
				}
			}
		}
	}

	// 3. Update / Insert New Items
	for id, newItem := range newIdx {
		sch, exists := s.discovered[id]

		// Determine target group key
		targetKey := s.taskKey(newItem.Agent, newItem.Delay)

		if exists {
			// Recover logic
			if sch.IsLost {
				sch.IsLost = false
				sch.LostSince = time.Time{}
				log.Printf("I! item recovered: %s", id)
			}

			wasDisabled := sch.IsDisabled
			if wasDisabled {
				sch.IsDisabled = false
			}

			oldKey := s.taskKey(sch.Item.Agent, sch.Interval)
			if oldKey != targetKey || wasDisabled {
				if !wasDisabled {
					s.removeItemFromTask(sch)
				}
				s.addItemToTask(targetKey, newItem)
			} else {
				s.updateItemInTask(sch, newItem)
			}
			sch.Item = newItem
			sch.Interval = newItem.Delay
		} else {
			sch = &ScheduledItem{
				Item:     newItem,
				Interval: newItem.Delay,
			}
			s.discovered[id] = sch

			s.addItemToTask(targetKey, newItem)
		}
	}
}

// addItemToTask adds an item to the TaskGroup. Creates group if needed.
func (s *ItemScheduler) addItemToTask(key string, item MonitorItem) {
	task, exists := s.taskMap[key]
	if !exists {
		task = &ScheduledTask{
			Agent:    item.Agent,
			Interval: item.Delay,
			Items:    []MonitorItem{item},
			NextRun:  s.calcScatteredNextRun(item.Agent, item.Delay),
		}
		heap.Push(&s.pq, task)
		s.taskMap[key] = task
	} else {
		task.Items = append(task.Items, item)
		// Heap position doesn't change when adding items
	}
}

// removeItemFromTask removes a specific item from its TaskGroup.
func (s *ItemScheduler) removeItemFromTask(sch *ScheduledItem) {
	key := s.taskKey(sch.Item.Agent, sch.Interval)
	task, exists := s.taskMap[key]
	if !exists {
		return
	}

	// Find and remove
	for i, it := range task.Items {
		if s.itemID(it) == s.itemID(sch.Item) {
			// Copy remove (stable) to preserve order for debugging consistency
			copy(task.Items[i:], task.Items[i+1:])
			task.Items = task.Items[:len(task.Items)-1]

			if len(task.Items) == 0 {
				heap.Remove(&s.pq, task.index)
				delete(s.taskMap, key)
			}
			break
		}
	}
}

// updateItemInTask updates the item data in the TaskGroup
func (s *ItemScheduler) updateItemInTask(sch *ScheduledItem, newItem MonitorItem) {
	key := s.taskKey(sch.Item.Agent, sch.Interval)
	task, exists := s.taskMap[key]
	if !exists {
		return
	}

	for i, it := range task.Items {
		if s.itemID(it) == s.itemID(sch.Item) {
			task.Items[i] = newItem
			return
		}
	}
}

func (s *ItemScheduler) runMaintainLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.maintainItemStates()
		}
	}
}

func (s *ItemScheduler) maintainItemStates() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, sch := range s.discovered {
		if !sch.IsLost {
			continue
		}
		elapsed := now.Sub(sch.LostSince)

		// Delete
		if sch.DeleteTTL != 0 && elapsed > sch.DeleteTTL {
			s.removeItemFromTask(sch)
			delete(s.discovered, id)
			if sch.Item.IsLabelProvider {
				s.labelCache.DeleteLabel(sch.Item.Agent, sch.Item.DiscoveryRuleKey, sch.Item.DiscoveryIndex, sch.Item.LabelKey)
			}
			continue
		}

		// Disable
		if !sch.IsDisabled && sch.DisableTTL != 0 && elapsed > sch.DisableTTL {
			s.removeItemFromTask(sch)
			sch.IsDisabled = true
		}
	}
}

func (s *ItemScheduler) AddItem(item MonitorItem) {
	if item.Delay == 0 {
		item.Delay = time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	k := s.taskKey(item.Agent, item.Delay)
	s.addItemToTask(k, item)
}

func (s *ItemScheduler) CollectInternalMetrics(slist *types.SampleList) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	for _, sch := range s.discovered {
		tags := map[string]string{
			"agent":          sch.Item.Agent,
			"item_key":       sch.Item.Key,
			"discovery_rule": sch.Item.DiscoveryRuleKey,
			"oid":            sch.Item.OID,
		}

		state := 0
		if sch.IsLost {
			if sch.IsDisabled {
				state = 2
			} else {
				state = 1
			}
		}
		slist.PushFront(types.NewSample("", "snmp_zabbix_item_discovery_state", state, tags))

		if sch.IsLost {
			elapsed := now.Sub(sch.LostSince)
			if !sch.IsDisabled && sch.DisableTTL != 0 {
				rem := sch.DisableTTL - elapsed
				if rem < 0 {
					rem = 0
				}
				dtags := copyTags(tags)
				dtags["action"] = "disable"
				slist.PushFront(types.NewSample("", "snmp_zabbix_item_remaining_seconds", rem.Seconds(), dtags))
			}
			if sch.DeleteTTL != 0 {
				rem := sch.DeleteTTL - elapsed
				if rem < 0 {
					rem = 0
				}
				dtags := copyTags(tags)
				dtags["action"] = "delete"
				slist.PushFront(types.NewSample("", "snmp_zabbix_item_remaining_seconds", rem.Seconds(), dtags))
			}
		}
	}
}

// Util function defined again to avoid dependency cycle if moved?
// Actually it was local in this file.
func copyTags(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src)+1)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
