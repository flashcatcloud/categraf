package cfg

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"regexp"
	"strings"
	"text/template"

	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"
)

var tplRe = regexp.MustCompile("\\.(toml|json|yaml|yml)\\.tpl")

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
		if strings.HasSuffix(fpath, "yaml") || strings.HasSuffix(fpath, "yml") {
			loaders = append(loaders, &multiconfig.YAMLLoader{Path: path.Join(configDir, fpath)})
		}
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}

	return m.Load(configPtr)
}

func renderConfig(content string, ctx map[string]interface{}) (io.Reader, error) {
	tpl, err := template.New("x").Funcs(funcMap()).Parse(content)
	buf := bytes.NewBuffer(make([]byte, 0, 2*len(content)))
	err = tpl.Execute(buf, ctx)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func LoadTemplates(configDir string, ctx map[string]interface{}, configPtr interface{}) error {
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	files, err := file.FilesUnder(configDir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", configDir, err)
	}

	hasTpl := false
	for _, fpath := range files {
		if !tplRe.MatchString(fpath) {
			continue
		}

		hasTpl = true
		bs, err := ioutil.ReadFile(path.Join(configDir, fpath))
		if err != nil {
			continue
		}
		cr, err := renderConfig(string(bs), ctx)
		if err != nil {
			continue
		}

		if strings.HasSuffix(fpath, "toml.tpl") {
			loaders = append(loaders, &multiconfig.TOMLLoader{Reader: cr})
		}
		if strings.HasSuffix(fpath, "json.tpl") {
			loaders = append(loaders, &multiconfig.JSONLoader{Reader: cr})
		}
		if strings.HasSuffix(fpath, "yaml.tpl") || strings.HasSuffix(fpath, "yml.tpl") {
			loaders = append(loaders, &multiconfig.YAMLLoader{Reader: cr})
		}
	}

	// if no tpl file provided, use context as full config
	if !hasTpl {
		loaders = append(loaders, &multiconfig.TOMLLoader{Reader: strings.NewReader("{{ . | toToml }}")})
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}

	return m.Load(configPtr)
}
