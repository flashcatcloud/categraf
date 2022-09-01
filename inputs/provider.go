package inputs

import (
	"encoding/json"
	"errors"
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
)

const inputFilePrefix = "input."

type Provider interface {
	StartReloader(reloadFunc func())
	GetInputs() ([]string, error)
	GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error)
}

func NewProvider(c *config.ConfigType) (Provider, error) {
	logger.Info("use input provider: ", c.Global.Provider)
	switch c.Global.Provider {
	case "HttpRemoteProvider":
		return newHttpRemoteProvider(c)
	default:
		return newLocalProvider(c)
	}
}

type LocalProvider struct {
	configDir string
}

func newLocalProvider(c *config.ConfigType) (*LocalProvider, error) {
	return &LocalProvider{
		configDir: c.ConfigDir,
	}, nil
}

// StartReloader 内部可以检查是否有配置的变更,如果有变更,则可以手动执行reloadFunc来重启插件
func (lp *LocalProvider) StartReloader(reloadFunc func()) {
	return
}

func (lp *LocalProvider) GetInputs() ([]string, error) {
	dirs, err := file.DirsUnder(lp.configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get dirs under %s : %v", config.Config.ConfigDir, err)
	}

	count := len(dirs)
	if count == 0 {
		return dirs, nil
	}

	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if strings.HasPrefix(dirs[i], inputFilePrefix) {
			names = append(names, dirs[i][len(inputFilePrefix):])
		}
	}
	return names, nil
}

func (lp *LocalProvider) GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error) {
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
	Tags           map[string]string
	ConfigFormat   cfg.ConfigFormat
	ReloadInterval int

	configMap map[string][]string
}

func newHttpRemoteProvider(c *config.ConfigType) (*HttpRemoteProvider, error) {
	if c.HttpRemoteProviderConfig == nil {
		return nil, fmt.Errorf("no http remote provider config found")
	}

	tags := c.HttpRemoteProviderConfig.Tags
	if tags == nil {
		tags = make(map[string]string)
	}
	if _, has := tags["host"]; !has {
		tags["host"] = c.GetHostname()
	}

	httpRemoteProvider := &HttpRemoteProvider{
		RemoteUrl:      c.HttpRemoteProviderConfig.RemoteUrl,
		Tags:           tags,
		ConfigFormat:   c.HttpRemoteProviderConfig.ConfigFormat,
		ReloadInterval: c.HttpRemoteProviderConfig.ReloadInterval,
	}

	return httpRemoteProvider, nil
}

func (hrp *HttpRemoteProvider) reload() (changed bool) {
	changed = false
	logger.Info("http remote provider: start reload config from remote ", hrp.RemoteUrl)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	req, err := http.NewRequest("GET", hrp.RemoteUrl, nil)
	if err != nil {
		logger.Error("http remote provider: build reload config request error ", err)
	}

	// build query parameters
	q := req.URL.Query()
	for k, v := range hrp.Tags {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("http remote provider: request reload config error ", err)
		return
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if len(respData) == 0 {
		err = errors.New("empty response")
	}
	if err != nil {
		logger.Error("http remote provider: request reload config error ", err)
		return
	}

	newConfig := make(map[string][]string)
	err = json.Unmarshal(respData, &newConfig)
	if err != nil {
		logger.Error("http remote provider: unmarshal result error ", err)
		return
	}

	// delete empty entries
	for k, v := range newConfig {
		if len(v) == 0 {
			delete(newConfig, k)
		}
	}

	news, updates, deletes := compareConfig(hrp.configMap, newConfig)
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
		hrp.configMap = newConfig
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
		cwf := make([]cfg.ConfigWithFormat, 0, len(conf))
		for _, c := range conf {
			cwf = append(cwf, cfg.ConfigWithFormat{
				Format: hrp.ConfigFormat,
				Config: c,
			})
		}
		return cwf, nil
	}
	return nil, fmt.Errorf("input %s not exist in http remote provider", inputName)
}

// compareConfig 比较新旧两个配置的差异
func compareConfig(cold, cnew map[string][]string) (news, updates, deletes []string) {
	news = make([]string, 0, len(cnew))
	updates = make([]string, 0, len(cnew))
	deletes = make([]string, 0, len(cnew))

	for kold, vold := range cold {
		if vnew, has := cnew[kold]; has {
			if !reflect.DeepEqual(vold, vnew) {
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
