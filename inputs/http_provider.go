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
	"flashcat.cloud/categraf/pkg/set"
	"flashcat.cloud/categraf/pkg/tls"
)

// HTTPProvider provider a mechanism to get config from remote http server at a fixed interval
// If input config is changed, the provider will reload the input without reload whole agent
type (
	HTTPProvider struct {
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

		configMap map[string]map[string]*cfg.ConfigWithFormat
		version   string

		cache *innerCache
		add   *innerCache
		del   *innerCache
	}
	innerCache struct {
		lock   *sync.RWMutex
		record map[string]map[string]cfg.ConfigWithFormat
	}
)

func newInnerCache() *innerCache {
	return &innerCache{
		lock:   &sync.RWMutex{},
		record: make(map[string]map[string]cfg.ConfigWithFormat),
	}
}

func (ic *innerCache) get(inputName string) (map[string]cfg.ConfigWithFormat, bool) {
	ic.lock.RLock()
	defer ic.lock.RUnlock()

	m, has := ic.record[inputName]
	return m, has
}

func (ic innerCache) put(inputName string, config cfg.ConfigWithFormat) {
	ic.lock.Lock()
	defer ic.lock.Unlock()
	if ic.record[inputName] == nil {
		ic.record[inputName] = make(map[string]cfg.ConfigWithFormat)
	}
	ic.record[inputName][config.CheckSum()] = config
}

func (ic *innerCache) del(inputName string, sum string) {
	ic.lock.Lock()
	defer ic.lock.Unlock()
	if len(sum) == 0 {
		delete(ic.record, inputName)
		return
	}
	if _, ok := ic.record[inputName]; ok {
		delete(ic.record[inputName], sum)
	}
}

func (ic *innerCache) iter() map[string]map[string]cfg.ConfigWithFormat {
	ic.lock.RLock()
	defer ic.lock.RUnlock()
	return ic.record
}

func (ic *innerCache) len() int {
	ic.lock.Lock()
	defer ic.lock.Unlock()
	return len(ic.record)
}

type httpProviderResponse struct {
	// version is signature/md5 of current Config, server side should deal with the Version calculate
	Version string `json:"version"`

	// ConfigMap (InputName -> Config), if version is identical, server side can set Config to nil
	Configs map[string]map[string]*cfg.ConfigWithFormat `json:"configs"`
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
		cache:          newInnerCache(),
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
	for k, v := range config.GlobalLabels() {
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

	// set checksum for each config
	newCfg := make(map[string]map[string]*cfg.ConfigWithFormat)
	for k := range confResp.Configs {
		lk := strings.TrimPrefix(strings.ToLower(k), "input.")
		if _, ok := newCfg[lk]; !ok {
			newCfg[lk] = make(map[string]*cfg.ConfigWithFormat)
		}
		for kk, vv := range confResp.Configs[k] {
			vv.SetCheckSum(kk)
			newCfg[lk][kk] = vv
		}
	}
	confResp.Configs = newCfg

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
	log.Printf("I! remote version:%s, current version:%s", confResp.Version, hrp.version)

	// delete empty entries
	for k, v := range confResp.Configs {
		if len(v) == 0 {
			delete(confResp.Configs, k)
		}
	}

	hrp.caculateDiff(confResp.Configs)
	changed := hrp.add.len()+hrp.del.len() > 0
	if changed {
		hrp.Lock()
		hrp.configMap = confResp.Configs
		hrp.version = confResp.Version
		hrp.Unlock()
	}

	return changed, nil
}

func (hrp *HTTPProvider) serviceInput(inputKey string) bool {
	switch inputKey {
	case "zabbix":
		return true
	}
	return false
}

func (hrp *HTTPProvider) preStop(inputKey string) error {
	if hrp.serviceInput(inputKey) {
		if dcm, ok := hrp.del.get(inputKey); ok {
			for sum := range dcm {
				hrp.op.DeregisterInput(FormatInputName(hrp.Name(), inputKey), sum)
			}
			time.Sleep(time.Duration(1) * time.Second)
		}
	}
	return nil
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
					if hrp.add.len() > 0 {
						log.Println("I! http provider: new or updated inputs:", hrp.add)
						for inputKey, cm := range hrp.add.iter() {
							hrp.preStop(inputKey)
							for _, conf := range cm {
								hrp.op.RegisterInput(FormatInputName(hrp.Name(), inputKey), []cfg.ConfigWithFormat{conf})
							}
						}
					}

					if hrp.del.len() > 0 {
						log.Println("I! http provider: deleted inputs:", hrp.del)
						for inputKey, cm := range hrp.del.iter() {
							if hrp.serviceInput(inputKey) {
								continue
							}
							for sum := range cm {
								hrp.op.DeregisterInput(FormatInputName(hrp.Name(), inputKey), sum)
							}
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

	cfgs := make([]cfg.ConfigWithFormat, 0, len(hrp.configMap[inputKey]))
	if configs, has := hrp.configMap[inputKey]; has {
		for _, v := range configs {
			cfgs = append(cfgs, *v)
		}
		return cfgs, nil
	}

	return nil, nil
}

func (hrp *HTTPProvider) caculateDiff(newConfigs map[string]map[string]*cfg.ConfigWithFormat) {
	hrp.add = newInnerCache()
	hrp.del = newInnerCache()
	cache := newInnerCache()
	for inputKey, configs := range newConfigs {
		for _, inputConfig := range configs {
			if config.Config.DebugMode {
				log.Println("D!: inputKey:", inputKey, "config sum:", inputConfig.CheckSum())
			}
			cache.put(inputKey, *inputConfig)
		}
	}

	for inputKey, configMap := range cache.iter() {
		if oldConfigMap, has := hrp.cache.get(inputKey); has {
			new := set.NewWithLoad[string, cfg.ConfigWithFormat](configMap)
			old := set.NewWithLoad[string, cfg.ConfigWithFormat](oldConfigMap)
			add, _, del := new.Diff(old)
			for sum := range add {
				if config.Config.DebugMode {
					log.Println("D!: add config:", inputKey, "config sum:", sum)
				}
				hrp.add.put(inputKey, configMap[sum])
			}
			for sum := range del {
				if config.Config.DebugMode {
					log.Println("D!: delete config:", inputKey, "config sum:", sum)
				}
				hrp.del.put(inputKey, oldConfigMap[sum])
			}
		} else {
			for _, inputConfig := range configMap {
				if config.Config.DebugMode {
					log.Println("D!: add config:", inputKey, "config sum:", inputConfig.CheckSum())
				}
				hrp.add.put(inputKey, inputConfig)
			}
		}
	}

	for inputKey, configMap := range hrp.cache.iter() {
		if _, has := cache.get(inputKey); !has {
			for _, inputConfig := range configMap {
				if config.Config.DebugMode {
					log.Println("D!: delete config:", inputKey, "config sum:", inputConfig.CheckSum())
				}
				hrp.del.put(inputKey, inputConfig)
			}
		}
	}
	if hrp.add.len()+hrp.del.len() > 0 {
		hrp.Lock()
		hrp.cache = cache
		hrp.Unlock()
	}

}

func (hrp *HTTPProvider) LoadInputConfig(configs []cfg.ConfigWithFormat, input Input) (map[string]Input, error) {
	inputs := make(map[string]Input)
	for _, c := range configs {
		nInput := input.Clone()
		err := cfg.LoadSingleConfig(c, nInput)
		if err != nil {
			log.Println("E! load http config error:", err)
			if config.Config.DebugMode {
				log.Printf("D! config:%+v load error:%s", c, err)
			}
			continue
		}
		inputs[c.CheckSum()] = nInput
	}
	return inputs, nil
}
