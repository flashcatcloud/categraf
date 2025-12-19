package snmp_zabbix

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/Knetic/govaluate"
	"gopkg.in/yaml.v3"
)

// Zabbix 现代YAML模板结构（6.0+版本）
type ZabbixTemplate struct {
	ZabbixExport ZabbixExport `yaml:"zabbix_export"`

	// 展平后的数据，用于内部处理
	Items          []TemplateItem
	DiscoveryRules []DiscoveryRule
	Macros         []Macro
	Format         string
}

type ZabbixExport struct {
	Version    string      `yaml:"version"`
	Date       string      `yaml:"date"`
	Groups     []Group     `yaml:"groups,omitempty"`
	Templates  []Template  `yaml:"templates,omitempty"`
	Hosts      []Host      `yaml:"hosts,omitempty"`
	ValueMaps  []ValueMap  `yaml:"valuemaps,omitempty"`
	MediaTypes []MediaType `yaml:"mediatypes,omitempty"`
}

type Group struct {
	UUID string `yaml:"uuid,omitempty"`
	Name string `yaml:"name"`
}

type Template struct {
	UUID           string          `yaml:"uuid,omitempty"`
	Template       string          `yaml:"template"`
	Name           string          `yaml:"name"`
	Description    string          `yaml:"description,omitempty"`
	Groups         []GroupRef      `yaml:"groups,omitempty"`
	Items          []TemplateItem  `yaml:"items,omitempty"`
	DiscoveryRules []DiscoveryRule `yaml:"discovery_rules,omitempty"`
	Triggers       []Trigger       `yaml:"triggers,omitempty"`
	Graphs         []Graph         `yaml:"graphs,omitempty"`
	HttpTests      []HttpTest      `yaml:"httptests,omitempty"`
	Macros         []Macro         `yaml:"macros,omitempty"`
	Tags           []Tag           `yaml:"tags,omitempty"`
	ValueMaps      []ValueMapRef   `yaml:"valuemaps,omitempty"`
}

type GroupRef struct {
	Name string `yaml:"name"`
}

type ValueMapRef struct {
	Name string `yaml:"name"`
}

type Host struct {
	UUID           string            `yaml:"uuid,omitempty"`
	Host           string            `yaml:"host"`
	Name           string            `yaml:"name"`
	Description    string            `yaml:"description,omitempty"`
	Status         string            `yaml:"status,omitempty"`
	Groups         []GroupRef        `yaml:"groups,omitempty"`
	Interfaces     []Interface       `yaml:"interfaces,omitempty"`
	Items          []TemplateItem    `yaml:"items,omitempty"`
	DiscoveryRules []DiscoveryRule   `yaml:"discovery_rules,omitempty"`
	Triggers       []Trigger         `yaml:"triggers,omitempty"`
	Graphs         []Graph           `yaml:"graphs,omitempty"`
	Macros         []Macro           `yaml:"macros,omitempty"`
	Tags           []Tag             `yaml:"tags,omitempty"`
	Inventory      map[string]string `yaml:"inventory,omitempty"`
}

type Interface struct {
	UUID    string           `yaml:"uuid,omitempty"`
	Type    string           `yaml:"type"`
	Main    string           `yaml:"main"`
	UseIP   string           `yaml:"useip"`
	IP      string           `yaml:"ip"`
	DNS     string           `yaml:"dns"`
	Port    string           `yaml:"port"`
	Details InterfaceDetails `yaml:"details,omitempty"`
}

type InterfaceDetails struct {
	Version        string `yaml:"version,omitempty"`
	Community      string `yaml:"community,omitempty"`
	SecurityName   string `yaml:"securityname,omitempty"`
	SecurityLevel  string `yaml:"securitylevel,omitempty"`
	AuthPassphrase string `yaml:"authpassphrase,omitempty"`
	PrivPassphrase string `yaml:"privpassphrase,omitempty"`
	AuthProtocol   string `yaml:"authprotocol,omitempty"`
	PrivProtocol   string `yaml:"privprotocol,omitempty"`
	ContextName    string `yaml:"contextname,omitempty"`
}

type TemplateItem struct {
	UUID            string            `yaml:"uuid,omitempty"`
	Name            string            `yaml:"name"`
	Type            string            `yaml:"type"`
	Key             string            `yaml:"key"`
	ValueType       string            `yaml:"value_type"`
	Units           string            `yaml:"units,omitempty"`
	History         string            `yaml:"history,omitempty"`
	Trends          string            `yaml:"trends,omitempty"`
	Status          string            `yaml:"status,omitempty"`
	Description     string            `yaml:"description,omitempty"`
	Delay           string            `yaml:"delay,omitempty"`
	Timeout         string            `yaml:"timeout,omitempty"`
	URL             string            `yaml:"url,omitempty"`
	QueryFields     []QueryField      `yaml:"query_fields,omitempty"`
	Parameters      []Parameter       `yaml:"parameters,omitempty"`
	Headers         map[string]string `yaml:"headers,omitempty"`
	PostsHTTP       string            `yaml:"posts,omitempty"`
	StatusCodes     string            `yaml:"status_codes,omitempty"`
	FollowRedirects string            `yaml:"follow_redirects,omitempty"`
	RetrieveMode    string            `yaml:"retrieve_mode,omitempty"`
	RequestMethod   string            `yaml:"request_method,omitempty"`
	OutputFormat    string            `yaml:"output_format,omitempty"`
	AllowTraps      string            `yaml:"allow_traps,omitempty"`
	TrappersHosts   string            `yaml:"trappers,omitempty"`
	SNMPOID         string            `yaml:"snmp_oid,omitempty"`
	Interface       InterfaceRef      `yaml:"interface,omitempty"`
	InventoryLink   string            `yaml:"inventory_link,omitempty"`
	Applications    []string          `yaml:"applications,omitempty"`
	Valuemap        ValueMapRef       `yaml:"valuemap,omitempty"`
	Logtimefmt      string            `yaml:"logtimefmt,omitempty"`
	Preprocessing   []PreprocessStep  `yaml:"preprocessing,omitempty"`
	JMXEndpoint     string            `yaml:"jmx_endpoint,omitempty"`
	MasterItem      MasterItemRef     `yaml:"master_item,omitempty"`
	Tags            []Tag             `yaml:"tags,omitempty"`
	Triggers        []Trigger         `yaml:"triggers,omitempty"`
}

type QueryField struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type Parameter struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type InterfaceRef struct {
	Ref string `yaml:"ref"`
}

type MasterItemRef struct {
	Key string `yaml:"key"`
}

type DiscoveryRule struct {
	UUID              string             `yaml:"uuid,omitempty"`
	Name              string             `yaml:"name"`
	Type              string             `yaml:"type"`
	Key               string             `yaml:"key"`
	Delay             string             `yaml:"delay,omitempty"`
	Status            string             `yaml:"status,omitempty"`
	Description       string             `yaml:"description,omitempty"`
	ItemPrototypes    []ItemPrototype    `yaml:"item_prototypes,omitempty"`
	TriggerPrototypes []TriggerPrototype `yaml:"trigger_prototypes,omitempty"`
	GraphPrototypes   []GraphPrototype   `yaml:"graph_prototypes,omitempty"`
	HostPrototypes    []HostPrototype    `yaml:"host_prototypes,omitempty"`
	Filter            DiscoveryFilter    `yaml:"filter,omitempty"`
	Lifetime          string             `yaml:"lifetime,omitempty"`
	Enabled           string             `yaml:"enabled,omitempty"`
	SNMPOID           string             `yaml:"snmp_oid,omitempty"`
	Interface         InterfaceRef       `yaml:"interface,omitempty"`
	Timeout           string             `yaml:"timeout,omitempty"`
	URL               string             `yaml:"url,omitempty"`
	QueryFields       []QueryField       `yaml:"query_fields,omitempty"`
	Parameters        []Parameter        `yaml:"parameters,omitempty"`
	Headers           map[string]string  `yaml:"headers,omitempty"`
	PostsHTTP         string             `yaml:"posts,omitempty"`
	StatusCodes       string             `yaml:"status_codes,omitempty"`
	FollowRedirects   string             `yaml:"follow_redirects,omitempty"`
	RetrieveMode      string             `yaml:"retrieve_mode,omitempty"`
	RequestMethod     string             `yaml:"request_method,omitempty"`
	OutputFormat      string             `yaml:"output_format,omitempty"`
	JMXEndpoint       string             `yaml:"jmx_endpoint,omitempty"`
	MasterItem        MasterItemRef      `yaml:"master_item,omitempty"`
	Preprocessing     []PreprocessStep   `yaml:"preprocessing,omitempty"`
	LLDMacroPaths     []LLDMacroPath     `yaml:"lld_macro_paths,omitempty"`
	Overrides         []Override         `yaml:"overrides,omitempty"`
}

type ItemPrototype struct {
	UUID              string             `yaml:"uuid,omitempty"`
	Name              string             `yaml:"name"`
	Type              string             `yaml:"type"`
	Key               string             `yaml:"key"`
	ValueType         string             `yaml:"value_type"`
	Units             string             `yaml:"units,omitempty"`
	History           string             `yaml:"history,omitempty"`
	Trends            string             `yaml:"trends,omitempty"`
	Status            string             `yaml:"status,omitempty"`
	Description       string             `yaml:"description,omitempty"`
	Delay             string             `yaml:"delay,omitempty"`
	Timeout           string             `yaml:"timeout,omitempty"`
	URL               string             `yaml:"url,omitempty"`
	QueryFields       []QueryField       `yaml:"query_fields,omitempty"`
	Parameters        []Parameter        `yaml:"parameters,omitempty"`
	Headers           map[string]string  `yaml:"headers,omitempty"`
	PostsHTTP         string             `yaml:"posts,omitempty"`
	StatusCodes       string             `yaml:"status_codes,omitempty"`
	FollowRedirects   string             `yaml:"follow_redirects,omitempty"`
	RetrieveMode      string             `yaml:"retrieve_mode,omitempty"`
	RequestMethod     string             `yaml:"request_method,omitempty"`
	OutputFormat      string             `yaml:"output_format,omitempty"`
	AllowTraps        string             `yaml:"allow_traps,omitempty"`
	TrappersHosts     string             `yaml:"trappers,omitempty"`
	SNMPOID           string             `yaml:"snmp_oid,omitempty"`
	Interface         InterfaceRef       `yaml:"interface,omitempty"`
	InventoryLink     string             `yaml:"inventory_link,omitempty"`
	Applications      []string           `yaml:"applications,omitempty"`
	Valuemap          ValueMapRef        `yaml:"valuemap,omitempty"`
	Logtimefmt        string             `yaml:"logtimefmt,omitempty"`
	Preprocessing     []PreprocessStep   `yaml:"preprocessing,omitempty"`
	JMXEndpoint       string             `yaml:"jmx_endpoint,omitempty"`
	MasterItem        MasterItemRef      `yaml:"master_item,omitempty"`
	Tags              []Tag              `yaml:"tags,omitempty"`
	TriggerPrototypes []TriggerPrototype `yaml:"trigger_prototypes,omitempty"`
	DiscoverKey       string             `yaml:"discover,omitempty"`
}

type TriggerPrototype struct {
	UUID               string              `yaml:"uuid,omitempty"`
	Expression         string              `yaml:"expression"`
	RecoveryMode       string              `yaml:"recovery_mode,omitempty"`
	RecoveryExpression string              `yaml:"recovery_expression,omitempty"`
	Name               string              `yaml:"name"`
	OpData             string              `yaml:"opdata,omitempty"`
	URL                string              `yaml:"url,omitempty"`
	Status             string              `yaml:"status,omitempty"`
	Priority           string              `yaml:"priority"`
	Description        string              `yaml:"description,omitempty"`
	Type               string              `yaml:"type,omitempty"`
	ManualClose        string              `yaml:"manual_close,omitempty"`
	Dependencies       []TriggerDependency `yaml:"dependencies,omitempty"`
	Tags               []Tag               `yaml:"tags,omitempty"`
	CorrelationMode    string              `yaml:"correlation_mode,omitempty"`
	CorrelationTag     string              `yaml:"correlation_tag,omitempty"`
	EventName          string              `yaml:"event_name,omitempty"`
}

type Trigger struct {
	UUID               string              `yaml:"uuid,omitempty"`
	Expression         string              `yaml:"expression"`
	RecoveryMode       string              `yaml:"recovery_mode,omitempty"`
	RecoveryExpression string              `yaml:"recovery_expression,omitempty"`
	Name               string              `yaml:"name"`
	OpData             string              `yaml:"opdata,omitempty"`
	URL                string              `yaml:"url,omitempty"`
	Status             string              `yaml:"status,omitempty"`
	Priority           string              `yaml:"priority"`
	Description        string              `yaml:"description,omitempty"`
	Type               string              `yaml:"type,omitempty"`
	ManualClose        string              `yaml:"manual_close,omitempty"`
	Dependencies       []TriggerDependency `yaml:"dependencies,omitempty"`
	Tags               []Tag               `yaml:"tags,omitempty"`
	CorrelationMode    string              `yaml:"correlation_mode,omitempty"`
	CorrelationTag     string              `yaml:"correlation_tag,omitempty"`
	EventName          string              `yaml:"event_name,omitempty"`
}

type TriggerDependency struct {
	Name               string `yaml:"name"`
	Expression         string `yaml:"expression"`
	RecoveryExpression string `yaml:"recovery_expression,omitempty"`
}

type GraphPrototype struct {
	UUID           string      `yaml:"uuid,omitempty"`
	Name           string      `yaml:"name"`
	Width          string      `yaml:"width,omitempty"`
	Height         string      `yaml:"height,omitempty"`
	YaxisMin       string      `yaml:"yaxismin,omitempty"`
	YaxisMax       string      `yaml:"yaxismax,omitempty"`
	ShowWorkPeriod string      `yaml:"show_work_period,omitempty"`
	ShowTriggers   string      `yaml:"show_triggers,omitempty"`
	Type           string      `yaml:"type,omitempty"`
	ShowLegend     string      `yaml:"show_legend,omitempty"`
	Show3D         string      `yaml:"show_3d,omitempty"`
	PercentLeft    string      `yaml:"percent_left,omitempty"`
	PercentRight   string      `yaml:"percent_right,omitempty"`
	YminType       string      `yaml:"ymin_type,omitempty"`
	YminItemKey    string      `yaml:"ymin_item_key,omitempty"`
	YmaxType       string      `yaml:"ymax_type,omitempty"`
	YmaxItemKey    string      `yaml:"ymax_item_key,omitempty"`
	GraphItems     []GraphItem `yaml:"graph_items,omitempty"`
}

type Graph struct {
	UUID           string      `yaml:"uuid,omitempty"`
	Name           string      `yaml:"name"`
	Width          string      `yaml:"width,omitempty"`
	Height         string      `yaml:"height,omitempty"`
	YaxisMin       string      `yaml:"yaxismin,omitempty"`
	YaxisMax       string      `yaml:"yaxismax,omitempty"`
	ShowWorkPeriod string      `yaml:"show_work_period,omitempty"`
	ShowTriggers   string      `yaml:"show_triggers,omitempty"`
	Type           string      `yaml:"type,omitempty"`
	ShowLegend     string      `yaml:"show_legend,omitempty"`
	Show3D         string      `yaml:"show_3d,omitempty"`
	PercentLeft    string      `yaml:"percent_left,omitempty"`
	PercentRight   string      `yaml:"percent_right,omitempty"`
	YminType       string      `yaml:"ymin_type,omitempty"`
	YminItemKey    string      `yaml:"ymin_item_key,omitempty"`
	YmaxType       string      `yaml:"ymax_type,omitempty"`
	YmaxItemKey    string      `yaml:"ymax_item_key,omitempty"`
	GraphItems     []GraphItem `yaml:"graph_items,omitempty"`
}

type HostPrototype struct {
	UUID             string               `yaml:"uuid,omitempty"`
	Host             string               `yaml:"host"`
	Name             string               `yaml:"name"`
	Status           string               `yaml:"status,omitempty"`
	Discover         string               `yaml:"discover,omitempty"`
	CustomInterfaces string               `yaml:"custom_interfaces,omitempty"`
	GroupLinks       []GroupLink          `yaml:"group_links,omitempty"`
	GroupPrototypes  []GroupPrototype     `yaml:"group_prototypes,omitempty"`
	Interfaces       []InterfacePrototype `yaml:"interfaces,omitempty"`
	Templates        []TemplateLink       `yaml:"templates,omitempty"`
	Macros           []Macro              `yaml:"macros,omitempty"`
	Tags             []Tag                `yaml:"tags,omitempty"`
	InventoryMode    string               `yaml:"inventory_mode,omitempty"`
}

type GroupLink struct {
	Group GroupRef `yaml:"group"`
}

type GroupPrototype struct {
	Name string `yaml:"name"`
}

type InterfacePrototype struct {
	Type    string           `yaml:"type"`
	Main    string           `yaml:"main"`
	UseIP   string           `yaml:"useip"`
	IP      string           `yaml:"ip"`
	DNS     string           `yaml:"dns"`
	Port    string           `yaml:"port"`
	Details InterfaceDetails `yaml:"details,omitempty"`
}

type TemplateLink struct {
	Name string `yaml:"name"`
}

type GraphItem struct {
	SortOrder string  `yaml:"sortorder,omitempty"`
	Color     string  `yaml:"color"`
	Type      string  `yaml:"type,omitempty"`
	YaxisSide string  `yaml:"yaxisside,omitempty"`
	CalcFnc   string  `yaml:"calc_fnc,omitempty"`
	DrawType  string  `yaml:"drawtype,omitempty"`
	Item      ItemRef `yaml:"item"`
}

type ItemRef struct {
	Host string `yaml:"host"`
	Key  string `yaml:"key"`
}

type DiscoveryFilter struct {
	EvalType   string            `yaml:"evaltype,omitempty"`
	Formula    string            `yaml:"formula,omitempty"`
	Conditions []FilterCondition `yaml:"conditions,omitempty"`
}

type FilterCondition struct {
	Macro     string `yaml:"macro"`
	Value     string `yaml:"value"`
	Operator  string `yaml:"operator,omitempty"`
	FormulaID string `yaml:"formulaid,omitempty"`
}

type LLDMacroPath struct {
	LLDMacro string `yaml:"lld_macro"`
	Path     string `yaml:"path"`
}

type Override struct {
	Name       string              `yaml:"name"`
	Step       string              `yaml:"step"`
	Stop       string              `yaml:"stop,omitempty"`
	Filter     DiscoveryFilter     `yaml:"filter,omitempty"`
	Operations []OverrideOperation `yaml:"operations,omitempty"`
}

type OverrideOperation struct {
	OperationType string            `yaml:"operationobject"`
	Operator      string            `yaml:"operator,omitempty"`
	Value         string            `yaml:"value,omitempty"`
	Status        string            `yaml:"status,omitempty"`
	Discover      string            `yaml:"discover,omitempty"`
	Period        string            `yaml:"period,omitempty"`
	History       string            `yaml:"history,omitempty"`
	Trends        string            `yaml:"trends,omitempty"`
	Severity      string            `yaml:"severity,omitempty"`
	Tags          []Tag             `yaml:"tags,omitempty"`
	Templates     []TemplateLink    `yaml:"templates,omitempty"`
	Inventory     map[string]string `yaml:"inventory,omitempty"`
}

type Macro struct {
	Macro       string `yaml:"macro"`
	Value       string `yaml:"value"`
	Type        string `yaml:"type,omitempty"`
	Description string `yaml:"description,omitempty"`
}

type Tag struct {
	Tag   string `yaml:"tag"`
	Value string `yaml:"value,omitempty"`
}

type PreprocessStep struct {
	Type               string   `yaml:"type"`
	Parameters         []string `yaml:"parameters,omitempty"`
	ErrorHandler       string   `yaml:"error_handler,omitempty"`
	ErrorHandlerParams string   `yaml:"error_handler_params,omitempty"`
}

type ValueMap struct {
	UUID     string         `yaml:"uuid,omitempty"`
	Name     string         `yaml:"name"`
	Mappings []ValueMapping `yaml:"mappings"`
}

type ValueMapping struct {
	Value    string `yaml:"value"`
	NewValue string `yaml:"newvalue"`
	Type     string `yaml:"type,omitempty"`
}

type MediaType struct {
	UUID             string            `yaml:"uuid,omitempty"`
	Name             string            `yaml:"name"`
	Type             string            `yaml:"type"`
	Status           string            `yaml:"status,omitempty"`
	Parameters       []MediaParameter  `yaml:"parameters,omitempty"`
	Script           string            `yaml:"script,omitempty"`
	Timeout          string            `yaml:"timeout,omitempty"`
	ProcessTags      string            `yaml:"process_tags,omitempty"`
	ShowEventMenu    string            `yaml:"show_event_menu,omitempty"`
	EventMenuURL     string            `yaml:"event_menu_url,omitempty"`
	EventMenuName    string            `yaml:"event_menu_name,omitempty"`
	Description      string            `yaml:"description,omitempty"`
	MessageTemplates []MessageTemplate `yaml:"message_templates,omitempty"`
}

type MediaParameter struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type MessageTemplate struct {
	EventSource string `yaml:"eventsource"`
	Recovery    string `yaml:"recovery"`
	Subject     string `yaml:"subject"`
	Message     string `yaml:"message"`
}

type HttpTest struct {
	UUID           string         `yaml:"uuid,omitempty"`
	Name           string         `yaml:"name"`
	Delay          string         `yaml:"delay"`
	Attempts       string         `yaml:"attempts,omitempty"`
	Agent          string         `yaml:"agent,omitempty"`
	HttpProxy      string         `yaml:"http_proxy,omitempty"`
	Variables      []HttpVariable `yaml:"variables,omitempty"`
	Headers        []HttpHeader   `yaml:"headers,omitempty"`
	Status         string         `yaml:"status,omitempty"`
	Authentication string         `yaml:"authentication,omitempty"`
	HttpUser       string         `yaml:"http_user,omitempty"`
	HttpPassword   string         `yaml:"http_password,omitempty"`
	VerifyPeer     string         `yaml:"verify_peer,omitempty"`
	VerifyHost     string         `yaml:"verify_host,omitempty"`
	SslCertFile    string         `yaml:"ssl_cert_file,omitempty"`
	SslKeyFile     string         `yaml:"ssl_key_file,omitempty"`
	SslKeyPassword string         `yaml:"ssl_key_password,omitempty"`
	Steps          []HttpStep     `yaml:"steps"`
	Tags           []Tag          `yaml:"tags,omitempty"`
}

type HttpVariable struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type HttpHeader struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type HttpStep struct {
	Name            string         `yaml:"name"`
	URL             string         `yaml:"url"`
	QueryFields     []QueryField   `yaml:"query_fields,omitempty"`
	Posts           string         `yaml:"posts,omitempty"`
	Variables       []HttpVariable `yaml:"variables,omitempty"`
	Headers         []HttpHeader   `yaml:"headers,omitempty"`
	FollowRedirects string         `yaml:"follow_redirects,omitempty"`
	RetrieveMode    string         `yaml:"retrieve_mode,omitempty"`
	Timeout         string         `yaml:"timeout,omitempty"`
	Required        string         `yaml:"required,omitempty"`
	StatusCodes     string         `yaml:"status_codes,omitempty"`
}

func MergeTemplates(base *ZabbixTemplate, toMerge *ZabbixTemplate) *ZabbixTemplate {
	if base == nil {
		return toMerge
	}
	if toMerge == nil {
		return base
	}

	// 假设所有内容都合并到第一个 <template> 下
	if len(base.ZabbixExport.Templates) == 0 {
		base.ZabbixExport.Templates = append(base.ZabbixExport.Templates, Template{})
	}
	baseTmpl := &base.ZabbixExport.Templates[0]

	itemMap := make(map[string]TemplateItem)
	for _, item := range baseTmpl.Items {
		itemMap[item.Key] = item
	}

	ruleMap := make(map[string]DiscoveryRule)
	for _, rule := range baseTmpl.DiscoveryRules {
		ruleMap[rule.Key] = rule
	}

	macroMap := make(map[string]Macro)
	for _, macro := range baseTmpl.Macros {
		macroMap[macro.Macro] = macro
	}

	// 从待合并模板的每个<template>定义中提取内容
	for _, tmplPart := range toMerge.ZabbixExport.Templates {
		for _, item := range tmplPart.Items {
			itemMap[item.Key] = item // 存在则覆盖，不存在则添加
		}
		for _, rule := range tmplPart.DiscoveryRules {
			ruleMap[rule.Key] = rule
		}
		for _, macro := range tmplPart.Macros {
			macroMap[macro.Macro] = macro
		}
	}

	// 从待合并模板的每个<host>定义中提取内容
	for _, hostPart := range toMerge.ZabbixExport.Hosts {
		for _, item := range hostPart.Items {
			itemMap[item.Key] = item
		}
		for _, rule := range hostPart.DiscoveryRules {
			ruleMap[rule.Key] = rule
		}
		for _, macro := range hostPart.Macros {
			macroMap[macro.Macro] = macro
		}
	}

	// 将maps转换回slices，更新到基础模板对象中
	baseTmpl.Items = make([]TemplateItem, 0, len(itemMap))
	for _, item := range itemMap {
		baseTmpl.Items = append(baseTmpl.Items, item)
	}

	baseTmpl.DiscoveryRules = make([]DiscoveryRule, 0, len(ruleMap))
	for _, rule := range ruleMap {
		baseTmpl.DiscoveryRules = append(baseTmpl.DiscoveryRules, rule)
	}

	baseTmpl.Macros = make([]Macro, 0, len(macroMap))
	for _, macro := range macroMap {
		baseTmpl.Macros = append(baseTmpl.Macros, macro)
	}

	// 使用最后一个模板的版本和日期信息更新合并后的模板
	base.ZabbixExport.Version = toMerge.ZabbixExport.Version
	base.ZabbixExport.Date = toMerge.ZabbixExport.Date

	return base
}

func ParseTemplateFromContent(data []byte) (*ZabbixTemplate, error) {
	// 目前只支持YAML，此函数作为未来扩展XML的入口
	template, err := parseYAMLTemplate(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template from content: %w", err)
	}
	// 注意：这里不调用 flattenTemplateData，由最终调用者在所有合并完成后统一调用
	return template, nil
}

// LoadAndMergeTemplates 加载多个Zabbix模板文件并将它们合并成一个单一的模板对象。
// 合并策略为“后来居上”，即后加载的模板会覆盖先前模板中具有相同key的项。
// 如果任何一个文件加载或解析失败，函数将立即返回错误。
func LoadAndMergeTemplates(files []string) (*ZabbixTemplate, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no template files provided")
	}

	// 1. 加载第一个模板作为基础
	mergedTemplate, err := loadZabbixTemplate(files[0])
	if err != nil {
		return nil, fmt.Errorf("failed to load base template file %s: %w", files[0], err)
	}

	// 如果只有一个模板，直接展平数据并返回
	if len(files) == 1 {
		mergedTemplate.flattenTemplateData()
		return mergedTemplate, nil
	}

	// 2. 遍历剩余的模板文件并进行合并
	for _, filename := range files[1:] {
		templateToMerge, err := loadZabbixTemplate(filename)
		if err != nil {
			// 失败即退出
			return nil, fmt.Errorf("failed to load template file '%s' for merging: %w", filename, err)
		}
		// 现在所有的合并逻辑都由 MergeTemplates 函数处理
		mergedTemplate = MergeTemplates(mergedTemplate, templateToMerge)
	}

	// 3. 在所有内容合并完成后，重新“展平”数据供插件内部使用
	mergedTemplate.flattenTemplateData()

	return mergedTemplate, nil
}

// 加载Zabbix模板的主函数
func loadZabbixTemplate(filename string) (*ZabbixTemplate, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	// 主要尝试YAML格式解析
	template, err := parseYAMLTemplate(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML template: %w", err)
	}

	return template, nil
}

func parseYAMLTemplate(data []byte) (*ZabbixTemplate, error) {
	var template ZabbixTemplate
	if err := yaml.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("failed to parse YAML template: %w", err)
	}

	template.Format = "yaml"
	return &template, nil
}

func (t *ZabbixTemplate) flattenTemplateData() {
	// 清空之前的数据
	t.Items = nil
	t.DiscoveryRules = nil
	t.Macros = nil

	for _, tmpl := range t.ZabbixExport.Templates {
		// 收集所有items
		t.Items = append(t.Items, tmpl.Items...)

		// 收集所有discovery rules
		t.DiscoveryRules = append(t.DiscoveryRules, tmpl.DiscoveryRules...)

		// 收集所有macros
		t.Macros = append(t.Macros, tmpl.Macros...)
	}

	// 从主机中收集数据（如果存在）
	for _, host := range t.ZabbixExport.Hosts {
		t.Items = append(t.Items, host.Items...)
		t.DiscoveryRules = append(t.DiscoveryRules, host.DiscoveryRules...)
		t.Macros = append(t.Macros, host.Macros...)
	}
}

func (t *ZabbixTemplate) GetMacroValue(macro string) string {
	for _, m := range t.Macros {
		if m.Macro == macro {
			return m.Value
		}
	}
	return ""
}

func (t *ZabbixTemplate) ExpandMacros(text string, context map[string]string) string {
	result := text

	// 展开上下文宏 (discovery macros)
	// Note: macro keys may be stored with or without braces (e.g., "{#SNMPINDEX}" or "SNMPINDEX")
	for macro, value := range context {
		// Normalize the macro name by removing the {# prefix and } suffix if present
		normalizedMacro := macro
		if strings.HasPrefix(normalizedMacro, "{#") && strings.HasSuffix(normalizedMacro, "}") {
			normalizedMacro = strings.TrimPrefix(normalizedMacro, "{#")
			normalizedMacro = strings.TrimSuffix(normalizedMacro, "}")
		}
		result = strings.ReplaceAll(result, fmt.Sprintf("{#%s}", normalizedMacro), value)
		result = strings.ReplaceAll(result, fmt.Sprintf("{%s}", normalizedMacro), value)
	}

	// 展开模板宏 {$MACRO}
	macroRegex := regexp.MustCompile(`\{\$([^}]+)\}`)
	matches := macroRegex.FindAllStringSubmatch(result, -1)

	for _, match := range matches {
		if len(match) == 2 {
			macroName := match[1]
			fullMacroName := fmt.Sprintf("{$%s}", macroName)
			macroValue := t.GetMacroValue(fullMacroName)
			if macroValue != "" {
				result = strings.ReplaceAll(result, match[0], macroValue)
			}
		}
	}

	return result
}

func (t *ZabbixTemplate) ValidateFilter(filter DiscoveryFilter, discoveryData map[string]string) bool {
	if len(filter.Conditions) == 0 {
		return true
	}

	results := make(map[string]bool)

	for _, condition := range filter.Conditions {
		macro := condition.Macro
		expectedValue := condition.Value

		// 获取实际值
		actualValue := t.getDiscoveryMacroValue(macro, discoveryData)

		var result bool
		// 主要是字符串操作符
		// TODO 数值类和集合类
		// LESS LESS_OR_EQUALS MORE MORE_OR_EQUALS
		// IN NOT_IN BETWEEN NOT_BETWEEN EXISTS NOT_EXISTS
		switch strings.ToUpper(condition.Operator) {
		case "8", "MATCHES_REGEX": // matches (regex)
			matched, _ := regexp.MatchString(expectedValue, actualValue)
			result = matched
		case "9", "NOT_MATCHES_REGEX": // does not match (regex)
			matched, _ := regexp.MatchString(expectedValue, actualValue)
			result = !matched
		case "2", "LIKE": // like
			result = strings.Contains(actualValue, expectedValue)
		case "3", "NOT_LIKE": // does not like
			result = !strings.Contains(actualValue, expectedValue)
		case "0", "EQUALS": // equals
			result = actualValue == expectedValue
		case "1", "NOT_EQUALS": // does not equal
			result = actualValue != expectedValue
		case "":
			matched, _ := regexp.MatchString(expectedValue, actualValue)
			result = matched
		default:
			result = actualValue == expectedValue
		}

		if condition.FormulaID != "" {
			results[condition.FormulaID] = result
		} else {
			results[fmt.Sprintf("condition_%d", len(results))] = result
		}
	}

	return t.evaluateFilterResults(filter, results)
}

func (t *ZabbixTemplate) getDiscoveryMacroValue(macro string, discoveryData map[string]string) string {
	if value, exists := discoveryData[macro]; exists {
		return value
	}

	return ""
}

func (t *ZabbixTemplate) evaluateFilterResults(filter DiscoveryFilter, results map[string]bool) bool {
	if len(results) == 0 {
		return true
	}

	switch filter.EvalType {
	case "0", "": // AND/OR (默认为AND)
		if filter.Formula != "" {
			return t.evaluateFormula(filter.Formula, results)
		}
		// 默认为AND
		for _, result := range results {
			if !result {
				return false
			}
		}
		return true
	case "1", "AND": // AND
		for _, result := range results {
			if !result {
				return false
			}
		}
		return true
	case "2", "OR": // OR
		for _, result := range results {
			if result {
				return true
			}
		}
		return false
	case "3", "FORMULA": // Custom expression
		if filter.Formula != "" {
			return t.evaluateFormula(filter.Formula, results)
		}
		return true
	default:
		// 默认为AND
		for _, result := range results {
			if !result {
				return false
			}
		}
		return true
	}
}

func (t *ZabbixTemplate) evaluateFormula(formula string, results map[string]bool) bool {
	// 1. 将 Zabbix 风格的 formula (e.g., "A and B") 转换为库可识别的格式 (e.g., "A && B")
	//    govaluate 支持 "and", "or", "not" 关键字，但也支持 &&, ||, !
	expressionStr := strings.ReplaceAll(formula, " and ", " && ")
	expressionStr = strings.ReplaceAll(expressionStr, " or ", " || ")
	expressionStr = strings.ReplaceAll(expressionStr, " not ", " ! ")

	// 2. 创建一个可执行的表达式对象
	expression, err := govaluate.NewEvaluableExpression(expressionStr)
	if err != nil {
		// 如果公式本身有语法错误，记录日志并返回 false
		log.Printf("E! failed to parse formula '%s': %v", formula, err)
		return false
	}

	// 3. 准备表达式中用到的变量 (A, B, C...)
	//    需要将 map[string]bool 转换为 map[string]interface{}
	parameters := make(map[string]interface{}, len(results))
	for id, result := range results {
		parameters[id] = result
	}

	// 4. 执行表达式并传入变量
	result, err := expression.Evaluate(parameters)
	if err != nil {
		// 如果执行过程中出错（例如缺少变量），记录日志并返回 false
		log.Printf("E! failed to evaluate formula '%s' with params %v: %v", formula, parameters, err)
		return false
	}

	// 5. 将执行结果转换为布尔值
	//    govaluate 的结果是 interface{} 类型，需要进行类型断言
	resultBool, ok := result.(bool)
	if !ok {
		log.Printf("E! formula '%s' did not return a boolean value", formula)
		return false
	}

	return resultBool
}

// 类型转换函数
func ConvertZabbixValueType(valueType string) string {
	switch valueType {
	case "FLOAT", "0": // numeric float
		return "float"
	case "CHAR", "1": // character
		return "string"
	case "LOG", "2": // log
		return "string"
	case "UNSIGNED", "3": // numeric unsigned
		return "uint"
	case "TEXT", "4": // text
		return "string"
	default:
		return "string"
	}
}

func ConvertZabbixItemType(itemType string) string {
	switch itemType {
	case "ZABBIX_AGENT", "0": // Zabbix agent
		return "agent"
	case "SNMP_AGENT", "1", "4", "6": // SNMPv1/v2c/v3 agent
		return "snmp"
	case "ZABBIX_TRAPPER", "2": // Zabbix trapper
		return "trapper"
	case "SIMPLE", "3": // simple check
		return "simple"
	case "ZABBIX_ACTIVE", "11": // Zabbix agent (active)
		return "agent_active"
	case "SNMP_TRAP", "20": // SNMP trap
		return "snmp_trap"
	case "HTTP_AGENT": // HTTP agent
		return "http"
	case "IPMI": // IPMI
		return "ipmi"
	case "SSH": // SSH
		return "ssh"
	case "TELNET": // Telnet
		return "telnet"
	case "JMX": // JMX
		return "jmx"
	case "DEPENDENT": // Dependent item
		return "dependent"
	case "CALCULATED": // Calculated
		return "calculated"
	default:
		return "unknown"
	}
}

// 获取SNMP相关的items
func (t *ZabbixTemplate) GetSNMPItems() []TemplateItem {
	var snmpItems []TemplateItem
	for _, item := range t.Items {
		if ConvertZabbixItemType(item.Type) == "snmp" {
			snmpItems = append(snmpItems, item)
		}
	}
	return snmpItems
}

// 模板验证
func (t *ZabbixTemplate) Validate() error {
	if t.ZabbixExport.Version == "" {
		return fmt.Errorf("template version is required")
	}

	if len(t.ZabbixExport.Templates) == 0 && len(t.ZabbixExport.Hosts) == 0 {
		return fmt.Errorf("template must contain at least one template or host")
	}

	// 验证items
	for i, item := range t.Items {
		if item.Key == "" {
			return fmt.Errorf("item %d: key is required", i)
		}
		if item.Name == "" {
			return fmt.Errorf("item %d: name is required", i)
		}
		if item.Type == "" {
			return fmt.Errorf("item %d: type is required", i)
		}
	}

	// 验证discovery rules
	for i, rule := range t.DiscoveryRules {
		if rule.Key == "" {
			return fmt.Errorf("discovery rule %d: key is required", i)
		}
		if rule.Name == "" {
			return fmt.Errorf("discovery rule %d: name is required", i)
		}
	}

	return nil
}
