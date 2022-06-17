package config

import (
	"fmt"
	"path"
	"strings"

	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"
)

type Discovery interface {
	// Inputs list all inputs from configuration discovery service.
	Inputs() ([]string, error)
	// Load loads the configuration from configuration discovery service.
	Load(in Input) error
}

// Input only need Prefix API in configuration discovery service.
type Input interface {
	Prefix() string
}

const inputFilePrefix = "input."

// FileDiscovery implements configuration discovery service.
type FileDiscovery struct {
	dir string
}

func NewFileDiscovery(dir string) Discovery {
	return &FileDiscovery{
		dir: dir,
	}
}

func (c *FileDiscovery) Inputs() ([]string, error) {
	dirs, err := file.DirsUnder(c.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get dirs under %s : %v", c.dir, err)
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

func (c *FileDiscovery) Load(in Input) error {
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	dir := path.Join(c.dir, inputFilePrefix+in.Prefix())
	files, err := file.FilesUnder(dir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", dir, err)
	}

	for _, fpath := range files {
		if strings.HasSuffix(fpath, "toml") {
			loaders = append(loaders, &multiconfig.TOMLLoader{Path: path.Join(dir, fpath)})
		}
		if strings.HasSuffix(fpath, "json") {
			loaders = append(loaders, &multiconfig.JSONLoader{Path: path.Join(dir, fpath)})
		}
		if strings.HasSuffix(fpath, "yaml") {
			loaders = append(loaders, &multiconfig.YAMLLoader{Path: path.Join(dir, fpath)})
		}
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}

	return m.Load(in)
}
