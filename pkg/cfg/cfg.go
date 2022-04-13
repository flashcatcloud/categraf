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
