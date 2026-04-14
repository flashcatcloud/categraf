package snmp_zabbix

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// DiscoveryScheduler 管理所有发现规则的调度
type DiscoveryScheduler struct {
	intervals map[time.Duration][]*ScheduledDiscovery
	engine    *DiscoveryEngine
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	ctx       context.Context

	// 用于跟踪每个agent-rule组合的调度
	ruleIndex map[string]*ScheduledDiscovery // "agent|ruleKey" -> scheduled

	runningIntervals map[time.Duration]bool

	// 回调函数，当发现完成时调用
	onDiscoveryComplete func(agent string, rule DiscoveryRule, items []MonitorItem)
}

// ScheduledDiscovery 表示一个调度的发现规则
type ScheduledDiscovery struct {
	Rule      DiscoveryRule
	Agent     string
	LastRun   time.Time
	NextRun   time.Time
	Interval  time.Duration
	LastError error

	// 统计信息
	RunCount     int
	SuccessCount int
	ErrorCount   int
}

func NewDiscoveryScheduler(engine *DiscoveryEngine) *DiscoveryScheduler {
	return &DiscoveryScheduler{
		intervals:        make(map[time.Duration][]*ScheduledDiscovery),
		engine:           engine,
		stopCh:           make(chan struct{}),
		ruleIndex:        make(map[string]*ScheduledDiscovery),
		runningIntervals: make(map[time.Duration]bool),
	}
}

// SetDiscoveryCallback 设置发现完成的回调函数
func (s *DiscoveryScheduler) SetDiscoveryCallback(callback func(agent string, rule DiscoveryRule, items []MonitorItem)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDiscoveryComplete = callback
}

// AddDiscoveryRule 添加一个发现规则到调度器
func (s *DiscoveryScheduler) AddDiscoveryRule(agent string, rule DiscoveryRule) {
	// 解析delay
	interval := parseZabbixDelay(rule.Delay)
	if interval == 0 {
		interval = 3600 * time.Second // 默认1小时
	}

	s.mu.Lock()

	// 创建调度项 - 初始NextRun设为当前时间，确保能立即执行
	scheduled := &ScheduledDiscovery{
		Rule:     rule,
		Agent:    agent,
		Interval: interval,
		NextRun:  time.Now(), // 改为当前时间，确保立即执行
	}

	// 生成索引key
	indexKey := s.ruleKey(agent, rule.Key)

	// 检查是否已存在
	if existing, exists := s.ruleIndex[indexKey]; exists {
		// 更新已存在的规则
		s.removeFromIntervalSlice(existing.Interval, existing)
		scheduled.RunCount = existing.RunCount
		scheduled.SuccessCount = existing.SuccessCount
		scheduled.ErrorCount = existing.ErrorCount
		scheduled.LastRun = existing.LastRun
		// 如果已经运行过，保持原有的NextRun计算
		if !existing.LastRun.IsZero() {
			scheduled.NextRun = existing.NextRun
		}
	}

	// 添加到对应的interval组
	s.intervals[interval] = append(s.intervals[interval], scheduled)
	s.ruleIndex[indexKey] = scheduled

	// 如果是新的interval且调度器正在运行，启动对应的runner
	needStart := s.running && !s.runningIntervals[interval]
	if needStart {
		s.runningIntervals[interval] = true
	}

	s.mu.Unlock()

	// 在锁外启动runner
	if needStart {
		go s.runInterval(s.ctx, interval)
	}
}

// Start 启动调度器
func (s *DiscoveryScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}

	s.running = true
	s.ctx = ctx
	s.stopCh = make(chan struct{})

	// 为每个interval启动runner
	for interval := range s.intervals {
		if !s.runningIntervals[interval] {
			s.runningIntervals[interval] = true
			go s.runInterval(ctx, interval)
		}
	}

	klog.InfoS("discovery scheduler started", "intervals", len(s.intervals))
}

// Stop 停止调度器
func (s *DiscoveryScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopCh)
	s.runningIntervals = make(map[time.Duration]bool)

	klog.InfoS("discovery scheduler stopped")
}

func (s *DiscoveryScheduler) runInterval(ctx context.Context, interval time.Duration) {
	klog.InfoS("starting discovery runner", "interval", interval)

	// 立即执行一次发现
	s.mu.RLock()
	currentRules := make([]*ScheduledDiscovery, len(s.intervals[interval]))
	copy(currentRules, s.intervals[interval])
	s.mu.RUnlock()

	if len(currentRules) > 0 {
		// 设置 NextRun 为当前时间，确保立即执行
		now := time.Now()
		s.mu.Lock()
		for _, rule := range currentRules {
			if rule.NextRun.After(now) {
				rule.NextRun = now
			}
		}
		s.mu.Unlock()

		// 立即执行
		s.checkAndExecuteRules(ctx, now, currentRules)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.InfoS("discovery runner stopped: context done", "interval", interval)
			return
		case <-s.stopCh:
			klog.InfoS("discovery runner stopped", "interval", interval)
			return
		case now := <-ticker.C:
			s.mu.RLock()
			currentRules := make([]*ScheduledDiscovery, len(s.intervals[interval]))
			copy(currentRules, s.intervals[interval])
			s.mu.RUnlock()

			if len(currentRules) > 0 {
				s.checkAndExecuteRules(ctx, now, currentRules)
			}
		}
	}
}

// checkAndExecuteRules 检查并执行到期的发现规则
func (s *DiscoveryScheduler) checkAndExecuteRules(ctx context.Context, now time.Time, rules []*ScheduledDiscovery) {
	var readyRules []*ScheduledDiscovery

	s.mu.Lock()
	for _, rule := range rules {
		// 检查是否到执行时间
		if !rule.NextRun.After(now.Add(jitterMagnitude(rule.Interval))) {
			readyRules = append(readyRules, rule)
			rule.LastRun = now
			// 计算下次运行时间，添加少量jitter避免同时执行
			rule.NextRun = now.Add(rule.Interval).Add(jitter(rule.Interval))
			klog.InfoS("scheduled discovery rule", "rule", rule.Rule.Key, "agent", rule.Agent, "next_run", rule.NextRun)
		} else {
			klog.V(1).InfoS("discovery rule not ready yet", "rule", rule.Rule.Key, "agent", rule.Agent, "next_run", rule.NextRun, "now", now)
		}
	}
	s.mu.Unlock()

	if len(readyRules) == 0 {
		klog.V(1).InfoS("no discovery rules ready to execute", "time", now)
		return
	}

	klog.InfoS("executing discovery rules", "count", len(readyRules), "time", now)

	// 并发执行发现规则
	var wg sync.WaitGroup
	for _, scheduled := range readyRules {
		wg.Add(1)
		go func(sd *ScheduledDiscovery) {
			defer wg.Done()
			s.executeDiscovery(ctx, sd)
		}(scheduled)
	}

	// 等待所有发现完成，但设置超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		klog.InfoS("all discovery rules completed")
	case <-time.After(5 * time.Minute): // 5分钟超时
		klog.Warning("discovery execution timeout, some rules may not have completed")
	}
}

// executeDiscovery 执行单个发现规则
func (s *DiscoveryScheduler) executeDiscovery(ctx context.Context, scheduled *ScheduledDiscovery) {
	startTime := time.Now()

	// 更新运行计数
	s.mu.Lock()
	scheduled.RunCount++
	s.mu.Unlock()

	klog.InfoS("executing discovery rule", "rule", scheduled.Rule.Key, "agent", scheduled.Agent, "run_count", scheduled.RunCount)

	// 执行发现
	discoveries, err := s.engine.ExecuteDiscovery(ctx, scheduled.Agent, scheduled.Rule)

	s.mu.Lock()
	if err != nil {
		scheduled.LastError = err
		scheduled.ErrorCount++
		s.mu.Unlock()
		klog.ErrorS(err, "discovery rule failed", "rule", scheduled.Rule.Key, "agent", scheduled.Agent)
		return
	}

	scheduled.LastError = nil
	scheduled.SuccessCount++
	s.mu.Unlock()

	klog.InfoS("discovery rule completed", "rule", scheduled.Rule.Key, "agent", scheduled.Agent, "items", len(discoveries), "duration", time.Since(startTime))

	// 应用item prototypes生成监控项
	items := s.engine.ApplyItemPrototypes(discoveries, scheduled.Rule)

	// 设置agent
	for i := range items {
		items[i].Agent = scheduled.Agent
	}

	// 调用回调函数
	s.mu.RLock()
	callback := s.onDiscoveryComplete
	s.mu.RUnlock()

	if callback != nil {
		callback(scheduled.Agent, scheduled.Rule, items)
	}
}

// LoadFromTemplate 从模板加载所有发现规则
func (s *DiscoveryScheduler) LoadFromTemplate(agents []string, template *ZabbixTemplate) {
	if template == nil {
		klog.Warning("no template provided for discovery scheduler")
		return
	}

	addedCount := 0
	skippedCount := 0
	for _, agent := range agents {
		for _, rule := range template.DiscoveryRules {
			// 只处理SNMP类型的发现规则
			if itemType := ConvertZabbixItemType(rule.Type); itemType != "snmp" {
				klog.Warningf("skipping non-SNMP discovery rule '%s' (type: %s -> %s)", rule.Key, rule.Type, itemType)
				continue
			}
			klog.InfoS("adding SNMP discovery rule", "rule", rule.Key, "delay", rule.Delay, "agent", agent)

			s.AddDiscoveryRule(agent, rule)
			addedCount++
		}
	}

	klog.InfoS("loaded discovery rules from template", "added", addedCount, "skipped", skippedCount)
}

// removeFromIntervalSlice 从interval切片中移除指定的调度项
func (s *DiscoveryScheduler) removeFromIntervalSlice(interval time.Duration, target *ScheduledDiscovery) {
	items := s.intervals[interval]
	for i := 0; i < len(items); i++ {
		if items[i] == target {
			items = append(items[:i], items[i+1:]...)
			i--
		}
	}
	s.intervals[interval] = items

	// 如果该interval已经没有规则了，清理它
	if len(items) == 0 {
		delete(s.intervals, interval)
	}
}

// ruleKey 生成规则的唯一标识
func (s *DiscoveryScheduler) ruleKey(agent, ruleKey string) string {
	return fmt.Sprintf("%s|%s", agent, ruleKey)
}

// GetStatistics 获取调度器统计信息
func (s *DiscoveryScheduler) GetStatistics() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["running"] = s.running
	stats["total_rules"] = len(s.ruleIndex)
	stats["interval_count"] = len(s.intervals)

	// 按interval统计
	intervalStats := make(map[string]int)
	for interval, rules := range s.intervals {
		intervalStats[interval.String()] = len(rules)
	}
	stats["intervals"] = intervalStats

	// 统计成功/失败
	totalRuns := 0
	totalSuccess := 0
	totalErrors := 0

	for _, scheduled := range s.ruleIndex {
		totalRuns += scheduled.RunCount
		totalSuccess += scheduled.SuccessCount
		totalErrors += scheduled.ErrorCount
	}

	stats["total_runs"] = totalRuns
	stats["total_success"] = totalSuccess
	stats["total_errors"] = totalErrors

	return stats
}

// GetRuleStatus 获取特定规则的状态
func (s *DiscoveryScheduler) GetRuleStatus(agent, ruleKey string) (map[string]interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.ruleKey(agent, ruleKey)
	scheduled, exists := s.ruleIndex[key]
	if !exists {
		return nil, false
	}

	status := make(map[string]interface{})
	status["agent"] = scheduled.Agent
	status["rule_key"] = scheduled.Rule.Key
	status["rule_name"] = scheduled.Rule.Name
	status["interval"] = scheduled.Interval.String()
	status["last_run"] = scheduled.LastRun
	status["next_run"] = scheduled.NextRun
	status["run_count"] = scheduled.RunCount
	status["success_count"] = scheduled.SuccessCount
	status["error_count"] = scheduled.ErrorCount

	if scheduled.LastError != nil {
		status["last_error"] = scheduled.LastError.Error()
	}

	return status, true
}
