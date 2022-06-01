package cfg

import (
	"fmt"
	"path"
	"strings"

	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"
)

func LoadConfigs(configDir string, configPtr interface{}) error {
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	files, err := file.FilesUnder(configDir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", configDir, err)
	}

	for _, fpath := range files {
		// logs.toml 单独解析
		if fpath == "logs.toml" {
			continue
		}
		if strings.HasSuffix(fpath, "toml") {
			loaders = append(loaders, &multiconfig.TOMLLoader{Path: path.Join(configDir, fpath)})
		}
		if strings.HasSuffix(fpath, "json") {
			loaders = append(loaders, &multiconfig.JSONLoader{Path: path.Join(configDir, fpath)})
		}
		if strings.HasSuffix(fpath, "yaml") {
			loaders = append(loaders, &multiconfig.YAMLLoader{Path: path.Join(configDir, fpath)})
		}
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}

	return m.Load(configPtr)
}

func LoadConfig(configFile string, configPtr interface{}) error {
	var (
		loader multiconfig.Loader
	)

	if strings.HasSuffix(configFile, "toml") {
		loader = &multiconfig.TOMLLoader{Path: configFile}
	}
	if strings.HasSuffix(configFile, "json") {
		loader = &multiconfig.JSONLoader{Path: configFile}
	}
	if strings.HasSuffix(configFile, "yaml") || strings.HasSuffix(configFile, "yml") {
		loader = &multiconfig.YAMLLoader{Path: configFile}
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loader),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}

	return m.Load(configPtr)
}
