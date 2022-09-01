package inputs

import (
	"fmt"
	"path"
	"strings"

	"github.com/toolkits/pkg/file"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/cfg"
)

const inputFilePrefix = "input."

type Provider interface {
	StartReloader(reloadFunc func())
	GetInputs() ([]string, error)
	GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error)
}

func NewProvider(provideName string, c *config.ConfigType) Provider {
	switch provideName {
	default:
		return newLocalProvider(c)
	}
}

type LocalProvider struct {
	configDir string
}

func newLocalProvider(c *config.ConfigType) *LocalProvider {
	return &LocalProvider{
		configDir: c.ConfigDir,
	}
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
