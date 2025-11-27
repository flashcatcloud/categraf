package snmp_zabbix

import (
	"context"
	"log"
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"flashcat.cloud/categraf/types"
)

type ItemScheduler struct {
	intervals map[time.Duration][]*ScheduledItem
	collector *SNMPCollector
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}

	ctx   context.Context
	slist *types.SampleList

	// 发现项索引（只索引 IsDiscovered=true 的项）
	discovered map[string]*ScheduledItem // id -> scheduled

	labelCache       *LabelCache
	runningIntervals map[time.Duration]bool // 记录已启动 runner 的 interval
}

type ScheduledItem struct {
	Item     MonitorItem
	LastRun  time.Time
	NextRun  time.Time
	Interval time.Duration
	// 可加上 ID 做调试
}

func NewItemScheduler(collector *SNMPCollector, labelCache *LabelCache) *ItemScheduler {
	rand.Seed(time.Now().UnixNano())
	return &ItemScheduler{
		intervals:  make(map[time.Duration][]*ScheduledItem),
		collector:  collector,
		stopCh:     make(chan struct{}),
		discovered: make(map[string]*ScheduledItem),
		labelCache: labelCache,

		runningIntervals: make(map[time.Duration]bool),
	}
}

func (s *ItemScheduler) startRunnerIfNeeded(iv time.Duration) {
	log.Printf("I! starting runner for interval %v", iv)
	if s.slist == nil || !s.running || s.ctx == nil {
		return
	}
	if s.runningIntervals[iv] {
		return
	}
	// 标记已启动再放锁，避免竞态下重复启动
	s.runningIntervals[iv] = true
	ctx := s.ctx
	slist := s.slist
	go s.runInterval(ctx, iv, slist)
}

func (s *ItemScheduler) itemID(it MonitorItem) string {
	// 假设 Key 在展开后包含索引，足以唯一；如需更稳妥可拼上 OID
	if it.Key != "" {
		return it.Agent + "|" + it.Key
	}
	return it.Agent + "|" + it.OID
}

func (s *ItemScheduler) AddItem(item MonitorItem) {
	interval := item.Delay
	if interval == 0 {
		interval = 60 * time.Second
	}

	s.mu.Lock()
	scheduledItem := &ScheduledItem{
		Item:     item,
		Interval: interval,
		NextRun:  time.Now().Add(jitter(interval)),
	}
	items := s.intervals[interval]
	prevLen := len(items)
	items = append(items, scheduledItem)
	s.intervals[interval] = items

	if item.IsDiscovered {
		id := s.itemID(item)
		s.discovered[id] = scheduledItem
	}

	running := s.running
	ctx := s.ctx
	slist := s.slist
	s.mu.Unlock()

	if running && slist != nil {
		s.mu.Lock()
		s.startRunnerIfNeeded(interval)
		s.mu.Unlock()
	}

	if running && slist != nil && prevLen == 0 {
		go s.checkAndExecuteItems(ctx, time.Now(), items, slist)
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
	for interval := range s.intervals {
		s.startRunnerIfNeeded(interval)
	}
}

func (s *ItemScheduler) runInterval(ctx context.Context, interval time.Duration, slist *types.SampleList) {
	stop := s.stopCh

	s.mu.RLock()
	currentItems := s.intervals[interval]
	s.mu.RUnlock()
	if len(currentItems) > 0 {
		s.checkAndExecuteItems(ctx, time.Now(), currentItems, slist)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case now := <-ticker.C:
			s.mu.RLock()
			currentItems := s.intervals[interval]
			s.mu.RUnlock()
			if len(currentItems) > 0 {
				s.checkAndExecuteItems(ctx, now, currentItems, slist)
			}
		}
	}
}

func (s *ItemScheduler) checkAndExecuteItems(ctx context.Context, now time.Time, items []*ScheduledItem, slist *types.SampleList) {
	var readyItems []MonitorItem

	s.mu.Lock()
	log.Println("group item ready length:", len(items), "time:", now.String())
	for _, item := range items {
		if !item.NextRun.After(now.Add(jitterMagnitude(item.Interval))) {
			readyItems = append(readyItems, item.Item)
			item.LastRun = now
			item.NextRun = now.Add(item.Interval).Add(jitter(item.Interval))
		}
	}
	s.mu.Unlock()

	if len(readyItems) > 0 {
		go s.executeItems(ctx, readyItems, slist)
	}
}

func (s *ItemScheduler) executeItems(ctx context.Context, items []MonitorItem, slist *types.SampleList) {
	agentItems := make(map[string][]MonitorItem)
	for _, item := range items {
		agentItems[item.Agent] = append(agentItems[item.Agent], item)
	}

	var wg sync.WaitGroup
	for agent, agentItemList := range agentItems {
		wg.Add(1)
		go func(agent string, items []MonitorItem) {
			defer wg.Done()
			s.collectItemsForAgent(ctx, agent, items, slist)
		}(agent, agentItemList)
	}
	wg.Wait()
}

func (s *ItemScheduler) collectItemsForAgent(ctx context.Context, agent string, items []MonitorItem, slist *types.SampleList) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("E! [CRITICAL] collection goroutine for agent %s panicked: %v\n%s",
				agent,
				r,
				debug.Stack(),
			)
		}
	}()

	if err := s.collector.CollectItems(ctx, items, slist); err != nil {
		log.Printf("Failed to collect items for agent %s: %v\n", agent, err)
		return
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
	s.runningIntervals = make(map[time.Duration]bool)
}

// 从某个 interval 切片中移除指定 ScheduledItem（按指针比较）
func (s *ItemScheduler) removeFromIntervalSlice(interval time.Duration, target *ScheduledItem) {
	items := s.intervals[interval]
	for i := 0; i < len(items); i++ {
		if items[i] == target {
			items = append(items[:i], items[i+1:]...)
			i--
		}
	}
	s.intervals[interval] = items
}

func (s *ItemScheduler) UpdateDiscoveredDiff(ruleKey string, newItems []MonitorItem, immediateOnExistingInterval bool) {
	now := time.Now()

	// 预处理：归一化 delay、标记 IsDiscovered
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

	var (
		intervalsToStart   []time.Duration
		intervalsToCheck   []time.Duration
		intervalOldLen     = make(map[time.Duration]int)
		intervalTouchedNew = make(map[time.Duration]bool) // 标记哪些 interval 有新增
	)

	s.mu.Lock()
	for iv, items := range s.intervals {
		intervalOldLen[iv] = len(items)
	}

	// 1) 删除：只处理属于当前 ruleKey 的旧项
	oldItemsForRule := make(map[string]*ScheduledItem)
	for id, sch := range s.discovered {
		if sch.Item.DiscoveryRuleKey == ruleKey {
			oldItemsForRule[id] = sch
		}
	}
	for id, sch := range oldItemsForRule {
		if _, ok := newIdx[id]; !ok {
			if sch.Item.IsLabelProvider {
				s.labelCache.DeleteLabel(sch.Item.Agent, sch.Item.DiscoveryRuleKey, sch.Item.DiscoveryIndex, sch.Item.LabelKey)
			}
			s.removeFromIntervalSlice(sch.Interval, sch)
			delete(s.discovered, id)
		}
	}

	// 2) 新增或更新
	for id, newItem := range newIdx {
		if sch, ok := s.discovered[id]; ok {
			// 更新
			oldInterval := sch.Interval
			newInterval := newItem.Delay

			if newInterval != oldInterval {
				s.removeFromIntervalSlice(oldInterval, sch)
				s.intervals[newInterval] = append(s.intervals[newInterval], sch)
				sch.Interval = newInterval
				intervalTouchedNew[newInterval] = true
			}

			if newItem.OID != sch.Item.OID || newInterval != oldInterval {
				sch.NextRun = now.Add(jitter(newInterval))
			}

			sch.Item = newItem
		} else {
			// 新增
			iv := newItem.Delay
			scheduled := &ScheduledItem{
				Item:     newItem,
				Interval: iv,
				NextRun:  now.Add(jitter(iv)),
			}
			s.intervals[iv] = append(s.intervals[iv], scheduled)
			s.discovered[id] = scheduled
			intervalTouchedNew[iv] = true
		}
	}

	for iv, items := range s.intervals {
		oldLen := intervalOldLen[iv]
		newLen := len(items)
		if oldLen == 0 && newLen > 0 {
			intervalsToStart = append(intervalsToStart, iv)
		} else if immediateOnExistingInterval && intervalTouchedNew[iv] {
			intervalsToCheck = append(intervalsToCheck, iv)
		}
	}

	running := s.running
	ctx := s.ctx
	slist := s.slist
	s.mu.Unlock()

	// 启动 interval 协程
	if running && slist != nil {
		s.mu.Lock()
		for _, iv := range intervalsToStart {
			s.startRunnerIfNeeded(iv)
		}
		s.mu.Unlock()
		// 可选立即检查（仅当 interval 原本已存在）
		for _, iv := range intervalsToCheck {
			s.mu.RLock()
			currentItems := s.intervals[iv]
			s.mu.RUnlock()
			if len(currentItems) > 0 {
				go s.checkAndExecuteItems(ctx, now, currentItems, slist)
			}
		}
	}
}
func jitterMagnitude(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}

	// 1. 计算抖动的最大幅度（例如，原始时长的 1%）
	jm := d / 100
	if jm <= 0 {
		return 0
	}
	if jm > 30*time.Second {
		jm = 30 * time.Second
	}
	return jm
}

// jitter 返回一个在 [-d/100, +d/100] 范围内的随机持续时间。
// 这用于给计划任务增加少量随机性，以防止多个任务在完全相同的时间点执行。
// 使用正负抖动可以避免调度周期稳定地错过下一个 Ticker 的问题。
func jitter(d time.Duration) time.Duration {
	jm := jitterMagnitude(d)

	// 2. 生成一个 [0, 2 * magnitude) 范围内的随机数
	n := rand.Int63n(2 * int64(jm))

	// 3. 将其平移，得到一个 [-magnitude, +magnitude) 范围内的值
	offset := n - int64(jm)

	return time.Duration(offset)
}
