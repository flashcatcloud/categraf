package cfg

import (
	"bytes"
	"fmt"
	"io"
	"os"
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
	Config string       `json:"config"`
	Format ConfigFormat `json:"format"`
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

func readFile(fname string) ([]byte, error) {
	file, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func LoadConfigByDir(configDir string, configPtr interface{}) error {
	var (
		tBuf, yBuf, jBuf []byte
	)

	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	files, err := file.FilesUnder(configDir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", configDir, err)
	}

	for _, fpath := range files {
		buf, err := readFile(path.Join(configDir, fpath))
		if err != nil {
			return err
		}
		switch {
		case strings.HasSuffix(fpath, "toml"):
			tBuf = append(tBuf, buf...)
		case strings.HasSuffix(fpath, "json"):
			jBuf = append(jBuf, buf...)
		case strings.HasSuffix(fpath, "yaml") || strings.HasSuffix(fpath, "yml"):
			yBuf = append(yBuf, buf...)
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
