package inputs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/choice"
)

const inputFilePrefix = "input."

// Provider InputProvider的抽象，可以实现此抽象来提供个性化的插件配置能力，如从远端定时读取配置等
type Provider interface {
	// StartReloader Provider初始化后会调用此方法
	// 可以根据需求实现定时加载配置的逻辑，注意对于远程拉取配置的Provider，先执行一次同步的拉取操作，再用goroutine定时请求
	StartReloader(reloadFunc func())

	// GetInputs 获取当前Provider提供了哪些插件
	GetInputs() ([]string, error)

	// GetInputConfig 获取input的配置，注意处理时先判断配置是否在provider中，如果在provider并且读取错误再返回error
	GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error)
}

func NewProvider(c *config.ConfigType) (Provider, error) {
	logger.Info("use input provider: ", c.Global.Providers)

	providers := make([]Provider, 0, len(c.Global.Providers))
	for _, p := range c.Global.Providers {
		switch p {
		case "HttpRemoteProvider":
			provider, err := newHttpRemoteProvider(c)
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

func (pm *ProviderManager) StartReloader(reloadFunc func()) {
	for _, p := range pm.providers {
		p.StartReloader(reloadFunc)
	}
}

func (pm *ProviderManager) GetInputs() ([]string, error) {
	inputSet := make(map[string]struct{})
	for _, p := range pm.providers {
		pInputs, err := p.GetInputs()
		if err != nil {
			logger.Warningf("provider manager, GetInputs of %s error, skip\n", reflect.TypeOf(p))
			continue
		}
		for _, input := range pInputs {
			inputSet[input] = struct{}{}
		}
	}

	inputs := make([]string, 0, len(inputSet))
	for input, _ := range inputSet {
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func (pm *ProviderManager) GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error) {
	cwf := make([]cfg.ConfigWithFormat, 0, len(pm.providers))
	for _, p := range pm.providers {
		pcwf, err := p.GetInputConfig(inputName)
		if err != nil {
			logger.Warningf("provider manager, failed to get config of %s from %s, error: %s", inputName, reflect.TypeOf(p), err)
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
	configDir  string
	inputNames []string
}

func newLocalProvider(c *config.ConfigType) (*LocalProvider, error) {
	dirs, err := file.DirsUnder(c.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get dirs under %s : %v", config.Config.ConfigDir, err)
	}

	names := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if strings.HasPrefix(dir, inputFilePrefix) {
			names = append(names, dir[len(inputFilePrefix):])
		}
	}
	return &LocalProvider{
		configDir:  c.ConfigDir,
		inputNames: names,
	}, nil
}

// StartReloader 内部可以检查是否有配置的变更,如果有变更,则可以手动执行reloadFunc来重启插件
func (lp *LocalProvider) StartReloader(reloadFunc func()) {
	return
}

func (lp *LocalProvider) GetInputs() ([]string, error) {
	return lp.inputNames, nil
}

func (lp *LocalProvider) GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error) {
	// 插件配置不在这个provider中
	if !choice.Contains(inputName, lp.inputNames) {
		return nil, nil
	}

	files, err := file.FilesUnder(path.Join(lp.configDir, inputFilePrefix+inputName))
	if err != nil {
		return nil, fmt.Errorf("failed to list files under: %s : %v", lp.configDir, err)
	}

	cwf := make([]cfg.ConfigWithFormat, 0, 1)
	for _, f := range files {
		c, err := file.ReadBytes(path.Join(lp.configDir, inputFilePrefix+inputName, f))
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

	RemoteUrl      string
	Headers        map[string]string
	AuthUsername   string
	AuthPassword   string
	ConfigFormat   cfg.ConfigFormat
	ReloadInterval int

	configMap map[string]string
	version   string
}

type httpRemoteProviderResponse struct {
	// version is signature/md5 of current Config, server side should deal with the Version calculate
	Version string `json:"version"`

	// ConfigMap (InputName -> ConfigContent), if version is identical, server side can set Config to nil
	Config map[string]string `json:"config"`
}

func newHttpRemoteProvider(c *config.ConfigType) (*HttpRemoteProvider, error) {
	if c.HttpRemoteProviderConfig == nil {
		return nil, fmt.Errorf("no http remote provider config found")
	}

	httpRemoteProvider := &HttpRemoteProvider{
		RemoteUrl:      c.HttpRemoteProviderConfig.RemoteUrl,
		Headers:        c.HttpRemoteProviderConfig.Headers,
		AuthUsername:   c.HttpRemoteProviderConfig.AuthUsername,
		AuthPassword:   c.HttpRemoteProviderConfig.AuthPassword,
		ConfigFormat:   c.HttpRemoteProviderConfig.ConfigFormat,
		ReloadInterval: c.HttpRemoteProviderConfig.ReloadInterval,
	}

	return httpRemoteProvider, nil
}

func (hrp *HttpRemoteProvider) doReq() (confResp *httpRemoteProviderResponse, err error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	req, err := http.NewRequest("GET", hrp.RemoteUrl, nil)
	if err != nil {
		logger.Error("http remote provider: build reload config request error ", err)
		return
	}
	for k, v := range hrp.Headers {
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

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("http remote provider: request reload config error ", err)
		return
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("http remote provider: request reload config error ", err)
		return
	}

	confResp = &httpRemoteProviderResponse{}
	err = json.Unmarshal(respData, confResp)
	if err != nil {
		logger.Error("http remote provider: unmarshal result error ", err)
		return
	}
	return
}

func (hrp *HttpRemoteProvider) reload() (changed bool) {
	changed = false
	logger.Info("http remote provider: start reload config from remote ", hrp.RemoteUrl)

	confResp, err := hrp.doReq()
	if err != nil {
		return
	}

	// if config version is identical, means config is not changed
	if confResp.Version == hrp.version {
		return
	}
	// if config is nil, may some error occurs in server side, ignore this instead of deleting all configs
	if confResp.Config == nil {
		logger.Warning("http remote provider: received config is empty")
		return
	}

	// delete empty entries
	for k, v := range confResp.Config {
		if len(v) == 0 {
			delete(confResp.Config, k)
		}
	}

	news, updates, deletes := compareConfig(hrp.configMap, confResp.Config)
	if len(news) > 0 {
		logger.Info("http remote provider: new inputs ", news)
	}
	if len(updates) > 0 {
		logger.Info("http remote provider: updated inputs ", updates)
	}
	if len(deletes) > 0 {
		logger.Info("http remote provider: deleted inputs ", deletes)
	}

	changed = len(news)+len(updates)+len(deletes) > 0
	if changed {
		hrp.Lock()
		defer hrp.Unlock()
		hrp.configMap = confResp.Config
		hrp.version = confResp.Version
	}
	return
}

func (hrp *HttpRemoteProvider) StartReloader(reloadFunc func()) {
	// sync load remote config
	hrp.reload()

	go func() {
		for {
			time.Sleep(time.Duration(hrp.ReloadInterval) * time.Second)
			if hrp.reload() {
				reloadFunc()
			}
		}
	}()
	return
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

func (hrp *HttpRemoteProvider) GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error) {
	hrp.RLock()
	defer hrp.RUnlock()

	if conf, has := hrp.configMap[inputName]; has {
		return []cfg.ConfigWithFormat{
			{
				Format: hrp.ConfigFormat,
				Config: conf,
			},
		}, nil
	}
	return nil, nil
}

// compareConfig 比较新旧两个配置的差异
func compareConfig(cold, cnew map[string]string) (news, updates, deletes []string) {
	news = make([]string, 0, len(cnew))
	updates = make([]string, 0, len(cnew))
	deletes = make([]string, 0, len(cnew))

	for kold, vold := range cold {
		if vnew, has := cnew[kold]; has {
			if vold != vnew {
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
