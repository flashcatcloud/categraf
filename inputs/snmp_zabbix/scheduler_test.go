package snmp_zabbix

import (
	"container/heap"
	"context"
	"testing"
	"time"

	"flashcat.cloud/categraf/types"
)

func TestScheduler_CalcScatteredNextRun(t *testing.T) {
	s := NewItemScheduler(nil, nil)
	interval := 60 * time.Second

	// 1. Deterministic check
	agent := "192.168.1.1"
	t1 := s.calcScatteredNextRun(agent, interval)
	t2 := s.calcScatteredNextRun(agent, interval)
	
	// They should be roughly equal (modulo execution time difference, but offset refers to consistent slots)
	// The function uses time.Now(), so exact equality depends on execution speed.
	// But the Offset (t % interval) logic inside is what matters.
	// Actually implementation is Now + (Hash % Interval). 
	// So (t1 - Now) should be close to (t2 - Now). 
	// Let's verify the OFFSET is consistent. (We can't easily access private method output directly without exposing or copy-paste logic).
	// But we can check stability.
	
	if t1.Sub(t2).Abs() > time.Second {
		t.Errorf("Expected deterministic next run, got large diff: %v vs %v", t1, t2)
	}

	// 2. Scattering check
	// Generate 100 agents, check if their offsets are distributed
	// agents := make([]string, 100) (unused)

	offsets := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		// Mock unique IPs
		agent := string(rune(i)) // simple
		next := s.calcScatteredNextRun(agent, interval)
		// offset = (Next - Now) % Interval approximately
		// But s.calcScatteredNextRun adds consistent offset to Now.
		// We can infer offset by running many and checking distribution.
		offsets[i] = next.Sub(time.Now())
	}
	
	// Sort offsets to see distribution
	// Just check standard deviation or spread?
	// Simple check: minimal colliding.
	uniqueOffsets := make(map[int]bool)
	for _, o := range offsets {
		sec := int(o.Seconds())
		uniqueOffsets[sec] = true
	}
	
	// With 100 agents and 60s interval, we expect high collisions if not scattered, wide spread if scattered.
	// Actually hash modulo 60 gives 60 slots. With 100 items, at least 40 collisions (Pigeonhole).
	// But we should use >> 1 unique slots.
	if len(uniqueOffsets) < 10 {
		t.Errorf("Expected distributed offsets, but got only %d unique seconds slots out of 100 agents", len(uniqueOffsets))
	}
}

func TestScheduler_HeapOrdering(t *testing.T) {
	s := NewItemScheduler(nil, nil)
	
	now := time.Now()
	
	// Add 3 tasks with different delays (which result in diff NextRun)
	// We manually inject for test
	t1 := &ScheduledTask{Agent: "A", NextRun: now.Add(10 * time.Second)}
	t2 := &ScheduledTask{Agent: "B", NextRun: now.Add(5 * time.Second)}
	t3 := &ScheduledTask{Agent: "C", NextRun: now.Add(15 * time.Second)}
	
	// Use heap.Push to maintain invariant
	heap.Push(&s.pq, t1)
	heap.Push(&s.pq, t2)
	heap.Push(&s.pq, t3)
	
	// Pop order should be: B (5s), A (10s), C (15s)
	
	p1 := heap.Pop(&s.pq).(*ScheduledTask)
	if p1.Agent != "B" {
		t.Errorf("Expected first pop to be B, got %s", p1.Agent)
	}
	
	p2 := heap.Pop(&s.pq).(*ScheduledTask)
	if p2.Agent != "A" {
		t.Errorf("Expected second pop to be A, got %s", p2.Agent)
	}
	
	p3 := heap.Pop(&s.pq).(*ScheduledTask)
	if p3.Agent != "C" {
		t.Errorf("Expected third pop to be C, got %s", p3.Agent)
	}
}

func TestScheduler_UpdateDiscoveredDiff_Logic(t *testing.T) {
	// Mock Collector? Not needed for logic test if we don't Execute.
	s := NewItemScheduler(nil, nil)
	
	ruleKey := "rule1"
	
	// 1. Add New Items
	item1 := MonitorItem{Agent: "1.1.1.1", Delay: 60*time.Second, Key: "k1", OID: ".1"}
	item2 := MonitorItem{Agent: "1.1.1.1", Delay: 60*time.Second, Key: "k2", OID: ".2"} // Same Agent/Interval
	item3 := MonitorItem{Agent: "2.2.2.2", Delay: 60*time.Second, Key: "k1", OID: ".1"} // Diff Agent
	
	s.UpdateDiscoveredDiff(ruleKey, []MonitorItem{item1, item2, item3}, false, 0, 0)
	
	// Verify TaskMap
	// Should have 2 tasks: "1.1.1.1|60s" and "2.2.2.2|60s"
	if len(s.taskMap) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(s.taskMap))
	}
	
	k1 := s.taskKey(item1.Agent, item1.Delay)
	task1 := s.taskMap[k1]
	if len(task1.Items) != 2 {
		t.Errorf("Expected 2 items in task 1, got %d", len(task1.Items))
	}
	
	// 2. Update - Remove item2
	// Simulating it disappeared from LLD
	s.UpdateDiscoveredDiff(ruleKey, []MonitorItem{item1, item3}, false, 0, 0)
	
	// Check task1 items
	if len(task1.Items) != 1 {
		t.Errorf("Expected 1 item in task 1 after removal, got %d", len(task1.Items))
	}
	if task1.Items[0].Key != "k1" {
		t.Errorf("Wrong item remaining: %s", task1.Items[0].Key)
	}
	
	// 3. Update - Change Interval for item3
	item3New := item3
	item3New.Delay = 120 * time.Second
	
	s.UpdateDiscoveredDiff(ruleKey, []MonitorItem{item1, item3New}, false, 0, 0)
	
	// Task for 2.2.2.2|60s should be gone or empty
	kOld := s.taskKey("2.2.2.2", 60*time.Second)
	if _, exists := s.taskMap[kOld]; exists {
		t.Error("Old task for 2.2.2.2|60s should be removed (clean up logic might be lazy, but empty task should be removed from map in strict impl)")
		// In my implementation, removeItemFromTask calls delete if empty.
	}
	
	kNew := s.taskKey("2.2.2.2", 120*time.Second)
	if _, exists := s.taskMap[kNew]; !exists {
		t.Error("New task for 2.2.2.2|120s missing")
	}
}

type MockCollector struct {}
func (m *MockCollector) CollectItems(ctx context.Context, items []MonitorItem, slist *types.SampleList) error {
	return nil
}
