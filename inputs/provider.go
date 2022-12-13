package inputs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/toolkits/pkg/file"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/choice"
	"flashcat.cloud/categraf/pkg/tls"
)

const inputFilePrefix = "input."

type InputOperation interface {
	RegisterInput(string, []cfg.ConfigWithFormat)
	DeregisterInput(string)
	ReregisterInput(string, []cfg.ConfigWithFormat)
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
	for _, p := range pm.providers {
		_, err := p.LoadConfig()
		if err != nil {
			log.Printf("E! provider manager, LoadConfig of %s err: %s", p.Name(), err)
		}
	}
	return false, nil
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

type LocalProvider struct {
	sync.RWMutex

	configDir  string
	inputNames []string
}

func newLocalProvider(c *config.ConfigType) (*LocalProvider, error) {
	return &LocalProvider{
		configDir: c.ConfigDir,
	}, nil
}

func (lp *LocalProvider) Name() string {
	return "local"
}

// StartReloader 内部可以检查是否有配置的变更,如果有变更,则可以手动执行reloadFunc来重启插件
func (lp *LocalProvider) StartReloader() {}

func (lp *LocalProvider) StopReloader() {}

func (lp *LocalProvider) LoadConfig() (bool, error) {
	dirs, err := file.DirsUnder(lp.configDir)
	if err != nil {
		return false, fmt.Errorf("failed to get dirs under %s : %v", config.Config.ConfigDir, err)
	}

	names := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if strings.HasPrefix(dir, inputFilePrefix) {
			names = append(names, dir[len(inputFilePrefix):])
		}
	}

	lp.Lock()
	lp.inputNames = names
	lp.Unlock()

	return false, nil
}

func (lp *LocalProvider) GetInputs() ([]string, error) {
	lp.RLock()
	defer lp.RUnlock()

	inputs := make([]string, 0, len(lp.inputNames))
	inputs = append(inputs, lp.inputNames...)
	return inputs, nil
}

func (lp *LocalProvider) GetInputConfig(inputKey string) ([]cfg.ConfigWithFormat, error) {
	// 插件配置不在这个provider中
	lp.RLock()
	if !choice.Contains(inputKey, lp.inputNames) {
		lp.RUnlock()
		return nil, nil
	}
	lp.RUnlock()

	files, err := file.FilesUnder(path.Join(lp.configDir, inputFilePrefix+inputKey))
	if err != nil {
		return nil, fmt.Errorf("failed to list files under: %s : %v", lp.configDir, err)
	}

	cwf := make([]cfg.ConfigWithFormat, 0, len(files))
	for _, f := range files {
		if !(strings.HasSuffix(f, ".yaml") ||
			strings.HasSuffix(f, ".yml") ||
			strings.HasSuffix(f, ".json") ||
			strings.HasSuffix(f, ".toml")) {
			continue
		}
		c, err := file.ReadBytes(path.Join(lp.configDir, inputFilePrefix+inputKey, f))
		if err != nil {
			return nil, err
		}
		cwf = append(cwf, cfg.ConfigWithFormat{
			Config: string(c),
			Format: cfg.GuessFormat(f),
		})
	}

	return cwf, nil
}

// HTTPProvider provider a mechanism to get config from remote http server at a fixed interval
// If input config is changed, the provider will reload the input without reload whole agent
type HTTPProvider struct {
	sync.RWMutex

	RemoteUrl    string
	Headers      []string
	AuthUsername string
	AuthPassword string

	Timeout        int
	ReloadInterval int

	tls.ClientConfig
	client *http.Client
	stopCh chan struct{}
	op     InputOperation

	configMap map[string]cfg.ConfigWithFormat
	version   string

	compareNewsCache    []string
	compareUpdatesCache []string
	compareDeletesCache []string
}

type httpProviderResponse struct {
	// version is signature/md5 of current Config, server side should deal with the Version calculate
	Version string `json:"version"`

	// ConfigMap (InputName -> Config), if version is identical, server side can set Config to nil
	Configs map[string]cfg.ConfigWithFormat `json:"configs"`
}

func (hrp *HTTPProvider) Name() string {
	return "http"
}

func newHTTPProvider(c *config.ConfigType, op InputOperation) (*HTTPProvider, error) {
	if c.HTTPProviderConfig == nil {
		return nil, fmt.Errorf("no http provider config found")
	}

	provider := &HTTPProvider{
		RemoteUrl:      c.HTTPProviderConfig.RemoteUrl,
		Headers:        c.HTTPProviderConfig.Headers,
		AuthUsername:   c.HTTPProviderConfig.AuthUsername,
		AuthPassword:   c.HTTPProviderConfig.AuthPassword,
		ClientConfig:   c.HTTPProviderConfig.ClientConfig,
		Timeout:        c.HTTPProviderConfig.Timeout,
		ReloadInterval: c.HTTPProviderConfig.ReloadInterval,
		stopCh:         make(chan struct{}, 1),
		op:             op,
	}

	if err := provider.check(); err != nil {
		return nil, err
	}

	return provider, nil
}

func (hrp *HTTPProvider) check() error {
	if hrp.Timeout <= 0 {
		hrp.Timeout = 5
	}

	if hrp.ReloadInterval <= 0 {
		hrp.ReloadInterval = 120
	}

	if !strings.HasPrefix(hrp.RemoteUrl, "http") {
		return fmt.Errorf("http provider: bad remote url config: %s", hrp.RemoteUrl)
	}

	tlsc, err := hrp.TLSConfig()
	if err != nil {
		return err
	}

	hrp.client = &http.Client{
		Timeout: time.Duration(hrp.Timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsc,
		},
	}

	return nil
}

func (hrp *HTTPProvider) doReq() (*httpProviderResponse, error) {
	req, err := http.NewRequest("GET", hrp.RemoteUrl, nil)
	if err != nil {
		log.Println("E! http provider: build reload config request error:", err)
		return nil, err
	}

	for i := 0; i < len(hrp.Headers); i += 2 {
		req.Header.Add(hrp.Headers[i], hrp.Headers[i+1])
		if hrp.Headers[i] == "Host" {
			req.Host = hrp.Headers[i+1]
		}
	}

	if hrp.AuthUsername != "" || hrp.AuthPassword != "" {
		req.SetBasicAuth(hrp.AuthUsername, hrp.AuthPassword)
	}

	// build query parameters
	q := req.URL.Query()
	for k, v := range config.Config.Global.Labels {
		q.Add(k, v)
	}
	q.Add("timestamp", fmt.Sprint(time.Now().Unix()))
	q.Add("version", hrp.version)
	req.URL.RawQuery = q.Encode()

	resp, err := hrp.client.Do(req)
	if err != nil {
		log.Println("E! http provider: request reload config error:", err)
		return nil, err
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("E! http provider: request reload config error:", err)
		return nil, err
	}

	confResp := &httpProviderResponse{}
	err = json.Unmarshal(respData, confResp)
	if err != nil {
		log.Println("E! http provider: unmarshal result error:", err)
		return nil, err
	}
	return confResp, nil
}

func (hrp *HTTPProvider) LoadConfig() (bool, error) {
	log.Println("I! http provider: start reload config from remote:", hrp.RemoteUrl)

	confResp, err := hrp.doReq()
	if err != nil {
		log.Printf("W! http provider: request remote err: [%+v]", err)
		return false, err
	}

	// if config version is identical, means config is not changed
	if confResp.Version == hrp.version {
		return false, nil
	}

	// if config is nil, may some error occurs in server side, ignore this instead of deleting all configs
	if confResp.Configs == nil {
		log.Println("W! http provider: received config is empty")
		return false, nil
	}

	// delete empty entries
	for k, v := range confResp.Configs {
		if len(v.Config) == 0 {
			delete(confResp.Configs, k)
		}
	}

	hrp.compareNewsCache, hrp.compareUpdatesCache, hrp.compareDeletesCache = compareConfig(hrp.configMap, confResp.Configs)
	changed := len(hrp.compareNewsCache)+len(hrp.compareUpdatesCache)+len(hrp.compareDeletesCache) > 0
	if changed {
		hrp.Lock()
		defer hrp.Unlock()
		hrp.configMap = confResp.Configs
		hrp.version = confResp.Version
	}

	return changed, nil
}

func (hrp *HTTPProvider) StartReloader() {
	go func() {
		for {
			select {
			case <-time.After(time.Duration(hrp.ReloadInterval) * time.Second):
				changed, err := hrp.LoadConfig()
				if err != nil {
					continue
				}
				if changed {
					if len(hrp.compareNewsCache) > 0 {
						log.Println("I! http provider: new inputs:", hrp.compareNewsCache)
						for _, newInput := range hrp.compareNewsCache {
							hrp.op.RegisterInput(FormatInputName(hrp.Name(), newInput), []cfg.ConfigWithFormat{hrp.configMap[newInput]})
						}
					}

					if len(hrp.compareUpdatesCache) > 0 {
						log.Println("I! http provider: updated inputs:", hrp.compareUpdatesCache)
						for _, updatedInput := range hrp.compareUpdatesCache {
							hrp.op.ReregisterInput(FormatInputName(hrp.Name(), updatedInput), []cfg.ConfigWithFormat{hrp.configMap[updatedInput]})
						}
					}

					if len(hrp.compareDeletesCache) > 0 {
						log.Println("I! http provider: deleted inputs:", hrp.compareDeletesCache)
						for _, deletedInput := range hrp.compareDeletesCache {
							hrp.op.DeregisterInput(FormatInputName(hrp.Name(), deletedInput))
						}
					}
				}
			case <-hrp.stopCh:
				return
			}
		}
	}()
}

func (hrp *HTTPProvider) StopReloader() {
	hrp.stopCh <- struct{}{}
}

func (hrp *HTTPProvider) GetInputs() ([]string, error) {
	hrp.RLock()
	defer hrp.RUnlock()

	inputs := make([]string, 0, len(hrp.configMap))
	for k := range hrp.configMap {
		inputs = append(inputs, k)
	}

	return inputs, nil
}

func (hrp *HTTPProvider) GetInputConfig(inputKey string) ([]cfg.ConfigWithFormat, error) {
	hrp.RLock()
	defer hrp.RUnlock()

	if conf, has := hrp.configMap[inputKey]; has {
		return []cfg.ConfigWithFormat{conf}, nil
	}

	return nil, nil
}

// compareConfig 比较新旧两个配置的差异
func compareConfig(cold, cnew map[string]cfg.ConfigWithFormat) (news, updates, deletes []string) {
	news = make([]string, 0, len(cnew))
	updates = make([]string, 0, len(cnew))
	deletes = make([]string, 0, len(cnew))

	for kold, vold := range cold {
		if vnew, has := cnew[kold]; has {
			if vold.Config != vnew.Config || vold.Format != vnew.Format {
				updates = append(updates, kold)
			}
		} else {
			deletes = append(deletes, kold)
		}
	}

	for knew := range cnew {
		if _, has := cold[knew]; !has {
			news = append(news, knew)
		}
	}

	return
}
