package inputs

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/toolkits/pkg/file"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
	"flashcat.cloud/categraf/pkg/choice"
)

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

func (lp *LocalProvider) LoadInputConfig(configs []cfg.ConfigWithFormat, input Input) (map[string]Input, error) {
	err := cfg.LoadConfigs(configs, input)
	if err != nil {
		return nil, err
	}
	return map[string]Input{
		"default": input,
	}, nil
}
