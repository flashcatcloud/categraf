package inputs

import (
	"crypto/tls"
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
)

const inputFilePrefix = "input."

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

func NewProvider(c *config.ConfigType, reloadFunc func()) (Provider, error) {
	log.Println("I! use input provider: ", c.Global.Providers)

	providers := make([]Provider, 0, len(c.Global.Providers))
	for _, p := range c.Global.Providers {
		switch p {
		case "HttpRemoteProvider":
			provider, err := newHttpRemoteProvider(c, reloadFunc)
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
	return "ProviderManager"
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
			log.Printf("E! provider manager, LoadConfig of %s err: %s\n", p.Name(), err)
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
			log.Printf("E! provider manager, GetInputs of %s error, skip\n", p.Name())
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
			log.Printf("E! provider manager, failed to get config of %s from %s, error: %s\n", inputName, p.Name(), err)
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
	return "LocalProvider"
}

// StartReloader 内部可以检查是否有配置的变更,如果有变更,则可以手动执行reloadFunc来重启插件
func (lp *LocalProvider) StartReloader() {
	return
}

func (lp *LocalProvider) StopReloader() {
	return
}

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
	for _, input := range lp.inputNames {
		inputs = append(inputs, input)
	}
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

	cwf := make([]cfg.ConfigWithFormat, 0, 1)
	for _, f := range files {
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

type HttpRemoteProvider struct {
	sync.RWMutex

	RemoteUrl             string
	Headers               map[string]string
	AuthUsername          string
	AuthPassword          string
	TlsInsecureSkipVerify bool

	Timeout        int
	ReloadInterval int

	client     *http.Client
	ch         chan struct{}
	reloadFunc func()

	configMap map[string]cfg.ConfigWithFormat
	version   string
}

type httpRemoteProviderResponse struct {
	// version is signature/md5 of current Config, server side should deal with the Version calculate
	Version string `json:"version"`

	// ConfigMap (InputName -> Config), if version is identical, server side can set Config to nil
	Configs map[string]cfg.ConfigWithFormat `json:"configs"`
}

func (hrp *HttpRemoteProvider) Name() string {
	return "HttpRemoteProvider"
}

func newHttpRemoteProvider(c *config.ConfigType, reloadFunc func()) (*HttpRemoteProvider, error) {
	if c.HttpRemoteProviderConfig == nil {
		return nil, fmt.Errorf("no http remote provider config found")
	}

	httpRemoteProvider := &HttpRemoteProvider{
		RemoteUrl:             c.HttpRemoteProviderConfig.RemoteUrl,
		Headers:               c.HttpRemoteProviderConfig.Headers,
		AuthUsername:          c.HttpRemoteProviderConfig.AuthUsername,
		AuthPassword:          c.HttpRemoteProviderConfig.AuthPassword,
		TlsInsecureSkipVerify: c.HttpRemoteProviderConfig.TlsInsecureSkipVerify,
		Timeout:               c.HttpRemoteProviderConfig.Timeout,
		ReloadInterval:        c.HttpRemoteProviderConfig.ReloadInterval,
		ch:                    make(chan struct{}),
		reloadFunc:            reloadFunc,
	}
	if err := httpRemoteProvider.check(); err != nil {
		return nil, err
	}
	return httpRemoteProvider, nil
}

func (hrp *HttpRemoteProvider) check() error {
	if hrp.Timeout <= 0 {
		hrp.Timeout = 5
	}

	if hrp.ReloadInterval <= 0 {
		hrp.ReloadInterval = 120
	}

	if !strings.HasPrefix(hrp.RemoteUrl, "http") {
		return fmt.Errorf("http remote provider: bad remote url config: %s", hrp.RemoteUrl)
	}

	if strings.HasPrefix(hrp.RemoteUrl, "https") && hrp.TlsInsecureSkipVerify {
		hrp.client = &http.Client{
			Timeout: time.Duration(hrp.Timeout) * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	} else {
		hrp.client = &http.Client{
			Timeout: time.Duration(hrp.Timeout) * time.Second,
		}
	}
	return nil
}

func (hrp *HttpRemoteProvider) doReq() (*httpRemoteProviderResponse, error) {
	req, err := http.NewRequest("GET", hrp.RemoteUrl, nil)
	if err != nil {
		log.Println("E! http remote provider: build reload config request error", err)
		return nil, err
	}

	for k, v := range hrp.Headers {
		if k == "Host" {
			req.Host = v
		}
		req.Header.Add(k, v)
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
		log.Println("E! http remote provider: request reload config error", err)
		return nil, err
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("E! http remote provider: request reload config error", err)
		return nil, err
	}

	confResp := &httpRemoteProviderResponse{}
	err = json.Unmarshal(respData, confResp)
	if err != nil {
		log.Println("E! http remote provider: unmarshal result error", err)
		return nil, err
	}
	return confResp, nil
}

func (hrp *HttpRemoteProvider) LoadConfig() (bool, error) {
	//var confResp *httpRemoteProviderResponse

	log.Println("E! http remote provider: start reload config from remote", hrp.RemoteUrl)

	confResp, err := hrp.doReq()
	if err != nil {
		return false, err
	}

	// if config version is identical, means config is not changed
	if confResp.Version == hrp.version {
		return false, nil
	}
	// if config is nil, may some error occurs in server side, ignore this instead of deleting all configs
	if confResp.Configs == nil {
		log.Println("W! http remote provider: received config is empty")
		return false, nil
	}

	// delete empty entries
	for k, v := range confResp.Configs {
		if len(v.Config) == 0 {
			delete(confResp.Configs, k)
		}
	}

	news, updates, deletes := compareConfig(hrp.configMap, confResp.Configs)
	if len(news) > 0 {
		log.Println("I! http remote provider: new inputs", news)
	}
	if len(updates) > 0 {
		log.Println("I! http remote provider: updated inputs", updates)
	}
	if len(deletes) > 0 {
		log.Println("I! http remote provider: deleted inputs", deletes)
	}

	changed := len(news)+len(updates)+len(deletes) > 0
	if changed {
		hrp.Lock()
		defer hrp.Unlock()
		hrp.configMap = confResp.Configs
		hrp.version = confResp.Version
	}
	return changed, nil
}

func (hrp *HttpRemoteProvider) StartReloader() {
	go func() {
		for {
			select {
			case <-time.After(time.Duration(hrp.ReloadInterval) * time.Second):
				changed, err := hrp.LoadConfig()
				if err != nil {
					continue
				}
				if changed {
					hrp.reloadFunc()
				}
			case <-hrp.ch:
				return
			}
		}
	}()
	return
}

func (hrp *HttpRemoteProvider) StopReloader() {
	hrp.ch <- struct{}{}
}

func (hrp *HttpRemoteProvider) GetInputs() ([]string, error) {
	hrp.RLock()
	defer hrp.RUnlock()

	inputs := make([]string, 0, len(hrp.configMap))
	for k, _ := range hrp.configMap {
		inputs = append(inputs, k)
	}
	return inputs, nil
}

func (hrp *HttpRemoteProvider) GetInputConfig(inputKey string) ([]cfg.ConfigWithFormat, error) {
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

	for knew, _ := range cnew {
		if _, has := cold[knew]; !has {
			news = append(news, knew)
		}
	}
	return
}
