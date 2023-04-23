package inputs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/tls"
)

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
	q.Add("agent_hostname", config.Config.GetHostname())
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

func (hrp *HTTPProvider) LoadInputConfig(configs []cfg.ConfigWithFormat, input Input) ([]Input, error) {
	inputs := make([]Input, 0, len(configs))
	for _, c := range configs {
		nInput := input.Clone()
		err := cfg.LoadSingleConfig(c, nInput)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, nInput)
	}
	return inputs, nil
}
