package cfg

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"
)

type ConfigFormat string

const (
	YamlFormat ConfigFormat = "yaml"
	TomlFormat ConfigFormat = "toml"
	JsonFormat ConfigFormat = "json"
)

type ConfigWithFormat struct {
	Config   string       `json:"config"`
	Format   ConfigFormat `json:"format"`
	checkSum string       `json:"-"`
}

func (cwf *ConfigWithFormat) CheckSum() string {
	return cwf.checkSum
}

func (cwf *ConfigWithFormat) SetCheckSum(checkSum string) {
	cwf.checkSum = checkSum
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
	var (
		tBuf []byte
	)

	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	files, err := file.FilesUnder(configDir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", configDir, err)
	}
	s := NewFileScanner()
	for _, fpath := range files {
		switch {
		case strings.HasSuffix(fpath, ".toml"):
			s.Read(path.Join(configDir, fpath))
			tBuf = append(tBuf, s.Data()...)
			tBuf = append(tBuf, []byte("\n")...)
		case strings.HasSuffix(fpath, ".json"):
			loaders = append(loaders, &multiconfig.JSONLoader{Path: path.Join(configDir, fpath)})
		case strings.HasSuffix(fpath, ".yaml") || strings.HasSuffix(fpath, ".yml"):
			loaders = append(loaders, &multiconfig.YAMLLoader{Path: path.Join(configDir, fpath)})
		}
		if s.Err() != nil {
			return s.Err()
		}
	}

	if len(tBuf) != 0 {
		loaders = append(loaders, &multiconfig.TOMLLoader{Reader: bytes.NewReader(tBuf)})
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}
	return m.Load(configPtr)
}

func LoadConfigs(configs []ConfigWithFormat, configPtr interface{}) error {
	var (
		tBuf, yBuf, jBuf []byte
	)
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}
	for _, c := range configs {
		switch c.Format {
		case TomlFormat:
			tBuf = append(tBuf, []byte("\n\n")...)
			tBuf = append(tBuf, []byte(c.Config)...)
		case YamlFormat:
			yBuf = append(yBuf, []byte(c.Config)...)
		case JsonFormat:
			jBuf = append(jBuf, []byte(c.Config)...)
		}
	}

	if len(tBuf) != 0 {
		loaders = append(loaders, &multiconfig.TOMLLoader{Reader: bytes.NewReader(tBuf)})
	}
	if len(yBuf) != 0 {
		loaders = append(loaders, &multiconfig.YAMLLoader{Reader: bytes.NewReader(yBuf)})
	}
	if len(jBuf) != 0 {
		loaders = append(loaders, &multiconfig.JSONLoader{Reader: bytes.NewReader(jBuf)})
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}
	return m.Load(configPtr)
}

func LoadSingleConfig(c ConfigWithFormat, configPtr interface{}) error {
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	switch c.Format {
	case TomlFormat:
		loaders = append(loaders, &multiconfig.TOMLLoader{Reader: bytes.NewReader([]byte(c.Config))})
	case YamlFormat:
		loaders = append(loaders, &multiconfig.YAMLLoader{Reader: bytes.NewReader([]byte(c.Config))})
	case JsonFormat:
		loaders = append(loaders, &multiconfig.JSONLoader{Reader: bytes.NewReader([]byte(c.Config))})

	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}
	return m.Load(configPtr)
}
