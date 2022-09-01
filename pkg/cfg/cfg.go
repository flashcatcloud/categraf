package cfg

import (
	"fmt"
	"path"
	"strings"

	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"
)

type ConfigFormat string

const (
	YamlFormat ConfigFormat = "YamlFormat"
	TomlFormat ConfigFormat = "TomlFormat"
	JsonFormat ConfigFormat = "JsonFormat"
)

type ConfigWithFormat struct {
	Config string
	Format ConfigFormat
}

func GuessFormat(fpath string) ConfigFormat {
	if strings.HasSuffix(fpath, ".json") {
		return JsonFormat
	}
	if strings.HasSuffix(fpath, ".yaml") || strings.HasSuffix(fpath, ".yml") {
		return YamlFormat
	}
	return TomlFormat
}

func LoadConfigByDir(configDir string, configPtr interface{}) error {
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	files, err := file.FilesUnder(configDir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", configDir, err)
	}

	for _, fpath := range files {
		if strings.HasSuffix(fpath, ".toml") {
			loaders = append(loaders, &multiconfig.TOMLLoader{Path: path.Join(configDir, fpath)})
		}
		if strings.HasSuffix(fpath, ".json") {
			loaders = append(loaders, &multiconfig.JSONLoader{Path: path.Join(configDir, fpath)})
		}
		if strings.HasSuffix(fpath, ".yaml") || strings.HasSuffix(fpath, ".yml") {
			loaders = append(loaders, &multiconfig.YAMLLoader{Path: path.Join(configDir, fpath)})
		}
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}

	return m.Load(configPtr)
}

func LoadConfigs(configs []ConfigWithFormat, configPtr interface{}) error {
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}
	for _, c := range configs {
		switch c.Format {
		case TomlFormat:
			loaders = append(loaders, &multiconfig.TOMLLoader{Reader: strings.NewReader(c.Config)})
		case YamlFormat:
			loaders = append(loaders, &multiconfig.YAMLLoader{Reader: strings.NewReader(c.Config)})
		case JsonFormat:
			loaders = append(loaders, &multiconfig.JSONLoader{Reader: strings.NewReader(c.Config)})
		}
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}
	return m.Load(configPtr)
}
