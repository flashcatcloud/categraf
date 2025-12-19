package snmp_zabbix

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"flashcat.cloud/categraf/writer"
)

const (
	inputName = "snmp_zabbix"
)

type SnmpZabbix struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`

	Mappings map[string]map[string]string `toml:"mappings"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &SnmpZabbix{}
	})
}

func (s *SnmpZabbix) Init() error {
	return nil
}

func (s *SnmpZabbix) Clone() inputs.Input {
	return &SnmpZabbix{}
}

func (s *SnmpZabbix) Name() string {
	return inputName
}

func (s *SnmpZabbix) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	inputLabels := s.GetLabels()
	if s.DebugMod {
		log.Printf("D!, snmp_zabbix input labels:%+v", inputLabels)
	}
	for i := 0; i < len(s.Instances); i++ {
		if len(s.Instances[i].Labels) == 0 {
			s.Instances[i].Labels = inputLabels
		} else {
			for k, v := range inputLabels {
				if _, has := s.Instances[i].Labels[k]; !has {
					s.Instances[i].Labels[k] = v
				}
			}
		}
		if len(s.Instances[i].Mappings) == 0 {
			s.Instances[i].Mappings = s.Mappings
		} else {
			m := make(map[string]map[string]string)
			for k, v := range s.Mappings {
				m[k] = v
			}
			for k, v := range s.Instances[i].Mappings {
				m[k] = v
			}
			s.Instances[i].Mappings = m
		}
		ret[i] = s.Instances[i]
	}
	return ret
}

func (s *SnmpZabbix) Drop() {
	for _, i := range s.Instances {
		i.Drop()
	}
}

func (s *SnmpZabbix) Start(slist *types.SampleList) error {
	var err error
	for _, i := range s.Instances {
		err = i.Start(slist)
		if err != nil {
			return err
		}
	}
	return nil
}

type Instance struct {
	config.InstanceConfig

	Agents         []string      `toml:"agents"`
	Version        int           `toml:"version"`
	Community      string        `toml:"community"`
	Username       string        `toml:"username"`
	AuthPassword   string        `toml:"auth_password"`
	AuthProtocol   string        `toml:"auth_protocol"`
	PrivPassword   string        `toml:"priv_password"`
	PrivProtocol   string        `toml:"priv_protocol"`
	SecurityLevel  string        `toml:"security_level"`
	ContextName    string        `toml:"context_name"`
	Port           int           `toml:"port"`
	Timeout        time.Duration `toml:"timeout"`
	Retries        int           `toml:"retries"`
	MaxRepetitions int           `toml:"max_repetitions"`

	UnconnectedUDPSocket bool `toml:"unconnected_udp_socket"`
	// Zabbix兼容配置
	TemplateFiles   []string     `toml:"template_files"`
	EnableDiscovery bool         `toml:"enable_discovery"`
	Items           []ItemConfig `toml:"items"`

	Field []FieldConfig `toml:"field"` // 兼容老的http下发配置
	// 内部组件
	config    *Config
	template  *ZabbixTemplate
	discovery *DiscoveryEngine
	collector *SNMPCollector
	client    *SNMPClientManager

	labelCache         *LabelCache
	discoveryScheduler *DiscoveryScheduler

	// 状态管理
	lastDiscovery   time.Time
	discoveredItems map[string][]MonitorItem
	mu              sync.RWMutex

	Mappings map[string]map[string]string `toml:"mappings"`

	scheduler   *ItemScheduler
	minInterval time.Duration
	stop        chan struct{}

	TemplateFileContents map[string]string `toml:"template_file_contents"`

	// 兼容n9e下发给snmp的配置
	SecLevel string `toml:"sec_level"`
	SecName  string `toml:"sec_name"`

	DisableUp     bool `toml:"disable_up"`
	DisableICMPUp bool `toml:"disable_icmp_up"`
	DisableSnmpUp bool `toml:"disable_snmp_up"`

	HealthcheckInterval time.Duration `toml:"healthcheck_interval"`
	HealthcheckTimeout  time.Duration `toml:"healthcheck_timeout"`
	HealthcheckRetries  int           `toml:"healthcheck_retries"`
}

type FieldConfig struct {
	Oid   string `toml:"oid"`
	Name  string `toml:"name"`
	IsTag bool   `toml:"is_tag"`
}

type ItemConfig struct {
	Key           string `toml:"key"`
	OID           string `toml:"oid"`
	Type          string `toml:"type"`
	DiscoveryRule string `toml:"discovery_rule"`
	Name          string `toml:"name"`
	Units         string `toml:"units"`

	Delay time.Duration `toml:"delay"`

	IsDiscoveryBased bool `toml:"-"`
	MacroTemplate    bool `toml:"-"` // 是否包含宏模板
}

var _ inputs.Input = new(SnmpZabbix)
var _ inputs.InstancesGetter = new(SnmpZabbix)
var _ inputs.SampleGatherer = new(Instance)

func (s *Instance) Description() string {
	return "SNMP input plugin with Zabbix compatibility"
}

func (s *Instance) Init() error {
	if len(s.TemplateFiles) != 0 || len(s.TemplateFileContents) != 0 {
		s.EnableDiscovery = true
	}

	if !s.EnableDiscovery {
		if s.DebugMod {
			log.Printf("D! snmp_zabbix discovery disabled")
		}
		// return nil
	}
	if len(s.TemplateFiles) == 0 && len(s.TemplateFileContents) == 0 && len(s.Items) == 0 {
		if s.DebugMod {
			log.Printf("D!, there are no template files, no template_file_contents, and no items defined")
		}
		return types.ErrInstancesEmpty
	}
	if s.stop == nil {
		s.stop = make(chan struct{})
	}
	// 设置默认值
	if s.Version == 0 {
		s.Version = 2
	}
	if s.Port == 0 {
		s.Port = 161
	}
	if s.Timeout == 0 {
		s.Timeout = 5 * time.Second
	}
	if s.Retries == 0 {
		s.Retries = 1
	}
	if s.MaxRepetitions == 0 {
		s.MaxRepetitions = 10
	}
	// 兼容 n9e 下发给snmp的配置 v3
	if len(s.SecLevel) != 0 && len(s.SecurityLevel) == 0 {
		s.SecurityLevel = s.SecLevel
	}
	if len(s.SecName) != 0 && len(s.Username) == 0 {
		s.Username = s.SecName
	}
	if s.HealthcheckInterval == 0 {
		s.HealthcheckInterval = 30 * time.Second
	}
	if s.HealthcheckTimeout == 0 {
		s.HealthcheckTimeout = 5 * time.Second
	}
	if s.HealthcheckRetries == 0 {
		s.HealthcheckRetries = 3
	}

	// 初始化配置
	var err error
	s.config, err = NewConfig(s)
	if err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// 初始化SNMP客户端管理器
	s.client = NewSNMPClientManager(s.config)
	s.client.ForceHealthCheck()
	var mergedTemplate *ZabbixTemplate

	// 加载Zabbix模板
	if len(s.TemplateFiles) != 0 {
		mergedTemplate, err = LoadAndMergeTemplates(s.TemplateFiles)
		if err != nil {
			log.Printf("E! failed to load template file %v: %v", s.TemplateFiles, err)
		}
	}

	if len(s.TemplateFileContents) > 0 {
		// 为了保证合并顺序的确定性，对 map 的 key 进行排序
		keys := make([]string, 0, len(s.TemplateFileContents))
		for k := range s.TemplateFileContents {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			content := s.TemplateFileContents[key]
			templateToMerge, pErr := ParseTemplateFromContent([]byte(content))
			if pErr != nil {
				log.Printf("E! failed to parse template content for key '%s': %v", key, pErr)
				continue // 跳过解析失败的模板
			}

			// 使用新的合并函数
			mergedTemplate = MergeTemplates(mergedTemplate, templateToMerge)
		}
	}
	s.template = mergedTemplate

	s.labelCache = NewLabelCache()
	// 初始化发现引擎
	if s.template != nil {
		s.template.flattenTemplateData()
		s.discovery = NewDiscoveryEngine(s.client, s.template)
		s.discoveryScheduler = NewDiscoveryScheduler(s.discovery)
		s.discoveryScheduler.SetDiscoveryCallback(func(agent string, rule DiscoveryRule, items []MonitorItem) {
			s.handleDiscoveryComplete(agent, rule, items)
		})
	}

	// 初始化收集器
	s.collector = NewSNMPCollector(s.client, s.config, s.labelCache, s.Labels, s.Mappings)

	s.discoveredItems = make(map[string][]MonitorItem)

	s.scheduler = NewItemScheduler(s.collector, s.labelCache)

	return nil
}

func (s *Instance) handleDiscoveryComplete(agent string, rule DiscoveryRule, items []MonitorItem) {
	// 过滤掉未展开宏或空OID的项目
	var filtered []MonitorItem
	for _, item := range items {
		if strings.Contains(item.OID, "{#") {
			log.Printf("W! item OID contains unexpanded macro: key=%s, oid=%s", item.Key, item.OID)
			continue
		}
		if item.OID == "" {
			log.Printf("W! item has empty OID: key=%s", item.Key)
			continue
		}
		item.IsDiscovered = true
		filtered = append(filtered, item)
	}

	log.Printf("Discovery rule '%s' for agent %s produced %d valid items (filtered from %d)",
		rule.Key, agent, len(filtered), len(items))

	// 使用差量更新到ItemScheduler
	if s.scheduler != nil && s.scheduler.running {
		s.scheduler.UpdateDiscoveredDiff(rule.Key, filtered, true)
	}
}

func (s *Instance) Gather(_ *types.SampleList) {}

func (s *Instance) getStaticItems() []MonitorItem {
	var items []MonitorItem
	for _, agent := range s.config.Agents {
		agentAddr := s.config.GetAgentAddress(agent)
		for _, item := range s.Items {
			if item.DiscoveryRule == "" {
				monitorItem := MonitorItem{
					Key:   item.Key,
					OID:   item.OID,
					Type:  item.Type,
					Name:  item.Name,
					Units: item.Units,
					Delay: item.Delay,
					Agent: agentAddr,
				}
				items = append(items, monitorItem)
			}
		}
	}

	return items
}

func (s *Instance) up(slist *types.SampleList) {
	if s.DisableUp || s.DisableSnmpUp {
		return
	}
	report := s.client.GetHealthReport()

	for _, detail := range report.ClientDetails {
		var upValue int
		if detail.Healthy {
			upValue = 1
		} else {
			upValue = 0
		}

		tags := map[string]string{
			"snmp_agent": detail.Agent,
			"snmp_host":  getHostFromAgentStr(detail.Agent),
			"plugin":     inputName,
		}

		sample := types.NewSample("", "snmp_zabbix_up", upValue, tags).SetTime(time.Now())

		slist.PushFront(sample)
	}
}

func (s *Instance) Start(_ *types.SampleList) error {
	baseCtx := context.Background()

	slist := types.NewSampleList()
	if !s.scheduler.running {
		s.scheduler.Start(baseCtx, slist)

		// 添加所有静态配置的items
		staticItems := s.getStaticItems()
		for _, item := range staticItems {
			if !s.discovery.containsMacros(item.Key) && !s.discovery.containsMacros(item.OID) {
				s.scheduler.AddItem(item)
			}
		}

		// Add static items from template
		templateItems := s.getTemplateStaticItems()
		for _, item := range templateItems {
			s.scheduler.AddItem(item)
		}
	}

	if s.EnableDiscovery && s.template != nil {
		var agents []string
		for _, agent := range s.config.Agents {
			agents = append(agents, s.config.GetAgentAddress(agent))
		}

		// 从模板加载发现规则
		s.discoveryScheduler.LoadFromTemplate(agents, s.template)
		// 启动发现调度器
		s.discoveryScheduler.Start(baseCtx)

		log.Printf("Discovery scheduler started with template rules")
	}

	go func(slist *types.SampleList) {
		processTicker := time.NewTicker(time.Second)
		defer processTicker.Stop()
		healthTicker := time.NewTicker(time.Second * 30)
		defer healthTicker.Stop()

		for {
			select {
			case <-healthTicker.C:
				s.up(slist)
			case <-processTicker.C:
				sl := s.Process(slist)
				arr := sl.PopBackAll()
				writer.WriteSamples(arr)
			case <-s.stop:
				return
			}
		}
	}(slist)
	return nil
}

func (s *Instance) Stop() {
	if s == nil {
		return
	}
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	if s.scheduler != nil {
		s.scheduler.Stop()
		s.scheduler = nil
	}
	if s.discoveryScheduler != nil {
		s.discoveryScheduler.Stop()
		s.discoveryScheduler = nil
	}
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
}

func (s *Instance) Drop() {
	s.Stop()
}

func (s *Instance) expandMacros(text string) string {
	if s.template != nil {
		return s.template.ExpandMacros(text, nil)
	}
	return text
}

func (s *Instance) getTemplateStaticItems() []MonitorItem {
	if s.template == nil {
		return nil
	}

	var items []MonitorItem
	templateItems := s.template.GetSNMPItems()

	for _, agent := range s.config.Agents {
		agentAddr := s.config.GetAgentAddress(agent)

		for _, tmplItem := range templateItems {
			// Check if item is disabled
			if tmplItem.Status == "DISABLED" || tmplItem.Status == "1" {
				continue
			}

			key := s.expandMacros(tmplItem.Key)
			oid := s.expandMacros(tmplItem.SNMPOID)
			name := s.expandMacros(tmplItem.Name)
			desc := s.expandMacros(tmplItem.Description)

			// Static items should not contain discovery macros {#MACRO}
			if strings.Contains(key, "{#") || strings.Contains(oid, "{#") {
				if s.DebugMod {
					log.Printf("W! skipping template item with discovery macros: key=%s, oid=%s", key, oid)
				}
				continue
			}

			tags := make(map[string]string)
			for _, t := range tmplItem.Tags {
				tags[t.Tag] = t.Value
			}

			valueType := ConvertZabbixValueType(tmplItem.ValueType)
			monitorItem := MonitorItem{
				Key:             key,
				OID:             oid,
				Type:            "snmp",
				Name:            name,
				Units:           tmplItem.Units,
				Delay:           parseZabbixDelay(tmplItem.Delay),
				Agent:           agentAddr,
				ValueType:       valueType,
				Description:     desc,
				Preprocessing:   tmplItem.Preprocessing,
				Tags:            tags,
				IsDiscovered:    false,
				IsLabelProvider: false,
			}

			// Set IsLabelProvider for CHAR/TEXT value types
			switch tmplItem.ValueType {
			case "CHAR", "1", "TEXT", "4":
				monitorItem.IsLabelProvider = true
				monitorItem.LabelKey = extractLabelKey(key)
			}

			items = append(items, monitorItem)
		}
	}
	return items
}