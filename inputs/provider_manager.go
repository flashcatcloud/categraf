package inputs

import (
	"fmt"
	"log"
	"strings"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
)

const inputFilePrefix = "input."

type InputOperation interface {
	RegisterInput(string, []cfg.ConfigWithFormat)
	DeregisterInput(string, string)
}

// FormatInputName providerName + '.' + inputKey
func FormatInputName(provider, inputKey string) string {
	return provider + "." + inputKey
}

// ParseInputName parse name into providerName and inputName
func ParseInputName(name string) (string, string) {
	data := strings.SplitN(name, ".", 2)
	if len(data) == 0 {
		return "", ""
	}
	if len(data) == 1 {
		return "", data[0]
	}
	return data[0], data[1]
}

// Provider InputProvider的抽象，可以实现此抽象来提供个性化的插件配置能力，如从远端定时读取配置等
type Provider interface {
	// Name 用于给input加前缀使用
	Name() string

	// StartReloader Provider初始化后会调用此方法
	// 可以根据需求实现定时加载配置的逻辑
	StartReloader()

	StopReloader()

	// LoadConfig 加载配置的方法，如果配置改变，返回true；提供给 StartReloader 以及 HUP信号的Reload使用
	LoadConfig() (bool, error)

	// GetInputs 获取当前Provider提供了哪些插件
	GetInputs() ([]string, error)

	// GetInputConfig 获取input的配置，注意处理时先判断配置是否在provider中，如果在provider并且读取错误再返回error
	GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error)

	// 加载 input 的配置
	LoadInputConfig([]cfg.ConfigWithFormat, Input) (map[string]Input, error)
}

func NewProvider(c *config.ConfigType, op InputOperation) (Provider, error) {
	log.Println("I! use input provider:", c.Global.Providers)
	// 不添加provider配置 则默认使用local
	// 兼容老版本
	if len(c.Global.Providers) == 0 {
		c.Global.Providers = append(c.Global.Providers, "local")
	}
	providers := make([]Provider, 0, len(c.Global.Providers))
	for _, p := range c.Global.Providers {
		name := strings.ToLower(p)
		switch name {
		case "http":
			provider, err := newHTTPProvider(c, op)
			if err != nil {
				return nil, err
			}
			providers = append(providers, provider)
		default:
			provider, err := newLocalProvider(c)
			if err != nil {
				return nil, err
			}
			providers = append(providers, provider)
		}
	}

	return &ProviderManager{
		providers: providers,
	}, nil
}

// ProviderManager combines multiple Provider's config together
type ProviderManager struct {
	providers []Provider
}

func (pm *ProviderManager) Name() string {
	return "pm"
}

func (pm *ProviderManager) StartReloader() {
	for _, p := range pm.providers {
		p.StartReloader()
	}
}

func (pm *ProviderManager) StopReloader() {
	for _, p := range pm.providers {
		p.StopReloader()
	}
}

func (pm *ProviderManager) LoadConfig() (bool, error) {
	changed := false
	for _, p := range pm.providers {
		ok, err := p.LoadConfig()
		if err != nil {
			log.Printf("E! provider manager, LoadConfig of %s err: %s", p.Name(), err)
		} else {
			changed = changed || ok
		}
	}
	return changed, nil
}

// GetInputs 返回带有provider前缀的inputName
func (pm *ProviderManager) GetInputs() ([]string, error) {
	inputs := make([]string, 0, 40)
	for _, p := range pm.providers {
		pInputs, err := p.GetInputs()
		if err != nil {
			log.Printf("E! provider manager, GetInputs of %s error: %v, skip", p.Name(), err)
			continue
		}
		for _, inputKey := range pInputs {
			inputs = append(inputs, FormatInputName(p.Name(), inputKey))
		}
	}

	return inputs, nil
}

// GetInputConfig 寻找匹配的Provider，从中查找input
func (pm *ProviderManager) GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error) {
	cwf := make([]cfg.ConfigWithFormat, 0, len(pm.providers))
	providerName, inputKey := ParseInputName(inputName)
	for _, p := range pm.providers {
		// 没有匹配，说明input不是该provider提供的
		if providerName != p.Name() {
			continue
		}

		pcwf, err := p.GetInputConfig(inputKey)
		if err != nil {
			log.Printf("E! provider manager, failed to get config of %s from %s, error: %s", inputName, p.Name(), err)
			continue
		}

		cwf = append(cwf, pcwf...)
	}

	if len(cwf) == 0 {
		return nil, fmt.Errorf("provider manager, failed to get config of %s", inputName)
	}

	return cwf, nil
}

func (pm *ProviderManager) LoadInputConfig(configs []cfg.ConfigWithFormat, input Input) (map[string]Input, error) {
	// 从配置中获取provider
	inputs := make(map[string]Input)
	for _, p := range pm.providers {
		is, err := p.LoadInputConfig(configs, input)
		if err != nil {
			return nil, err
		}
		for s, i := range is {
			inputs[s] = i
		}
	}

	return inputs, nil
}
