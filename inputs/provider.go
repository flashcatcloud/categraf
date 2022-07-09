package inputs

import (
	"fmt"
	"path"
	"strings"

	"github.com/toolkits/pkg/file"

	"flashcat.cloud/categraf/pkg/cfg"
)

const inputFilePrefix = "input."

// InputProvider allow you customize the way to initialize Input plugin
// it is possible to initialize Input with config from network or other source by implement it
type InputProvider interface {
	ListInputNames() ([]string, error)
	GetInput(creator Creator, inputName string) Input
}

type DirConfigInputProvider struct {
	ConfigDir string
}

func (p *DirConfigInputProvider) ListInputNames() ([]string, error) {
	dirs, err := file.DirsUnder(p.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get dirs under %s : %v", p.ConfigDir, err)
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

func (p *DirConfigInputProvider) GetInput(creator Creator, inputName string) Input {
	inp := creator()
	// set configurations for input instance
	cfg.LoadConfigs(path.Join(p.ConfigDir, inputFilePrefix+inputName), inp)
	return inp
}

// TemplateInputProvider render plugin config with template + context, which will simplify the cost of configure plugins
type TemplateInputProvider struct {
	ConfigDir  string
	ContextMap map[string]map[string]interface{}
}

func (p *TemplateInputProvider) ListInputNames() ([]string, error) {
	inputs := make([]string, 0, len(p.ContextMap))
	for inputName, ctx := range p.ContextMap {
		if enable, has := ctx["enable"]; has && enable.(bool) {
			inputs = append(inputs, inputName)
		}
	}

	return inputs, nil
}

func (p *TemplateInputProvider) GetInput(creator Creator, inputName string) Input {
	inp := creator()
	ctx := p.ContextMap[inputName]
	cfg.LoadTemplates(path.Join(p.ConfigDir, inputFilePrefix+inputName), ctx, inp)
	return inp
}
