package agent

import (
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"

	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"

	_ "flashcat.cloud/categraf/inputs/redis"
)

type Agent struct {
	ConfigDir string
	DebugMode bool
}

func NewAgent(configDir, debugMode string) (*Agent, error) {
	configFile := path.Join(configDir, "config.toml")
	if !file.IsExist(configFile) {
		return nil, fmt.Errorf("configuration file(%s) not found", configFile)
	}

	debug, err := strconv.ParseBool(debugMode)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bool(%s): %v", debugMode, err)
	}

	ag := &Agent{
		ConfigDir: configDir,
		DebugMode: debug,
	}

	log.Println("I! agent.instance:", ag)

	return ag, nil
}

func (a *Agent) String() string {
	return fmt.Sprintf("<ConfigDir:%s DebugMode:%v>", a.ConfigDir, a.DebugMode)
}

func (a *Agent) Start() {
	log.Println("I! agent starting")

	StartInputs(a.ConfigDir)
}

func (a *Agent) Stop() {
	log.Println("I! agent stopping")

}

func (a *Agent) Reload() {
	log.Println("I! agent reloading")

}

// -----

type Consumer struct {
	Instance inputs.Input
	Queue    chan *types.Sample
	Quit     chan struct{}
}

var InputConsumers = map[string]*Consumer{}

func StartInputs(confd string) error {
	names, err := getInputsByDirs(confd)
	if err != nil {
		return err
	}

	if len(names) == 0 {
		log.Println("I! no inputs")
		return nil
	}

	for _, name := range names {
		creator, has := inputs.InputCreators[name]
		if !has {
			log.Println("E! input:", name, "not supported")
			continue
		}

		// construct input instance
		instance := creator()

		// set configurations for input instance
		loadConfigs(path.Join(confd, "input."+name), instance.GetPointer())

		// check configurations
		if err = instance.TidyConfig(); err != nil {
			log.Println("E! input:", name, "configurations invalid:", err)
			continue
		}

		c := &Consumer{
			Instance: instance,
			Quit:     make(chan struct{}),
			Queue:    make(chan *types.Sample, 1000000),
		}

		// start consumer goroutines
		go consume(c.Queue)

		// start collector goroutines
		instance.StartGoroutines(c.Queue)

		InputConsumers[name] = c
	}

	return nil
}

func consume(queue chan *types.Sample) {
	for s := range queue {
		fmt.Println(s.Metric)
		fmt.Println(s.Labels)
		fmt.Println(s.Timestamp)
		fmt.Println(s.Value)
	}
}

func loadConfigs(configDir string, configPtr interface{}) error {
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

	m.MustLoad(configPtr)

	return nil
}

// input dir should has prefix input.
func getInputsByDirs(confd string) ([]string, error) {
	dirs, err := file.DirsUnder(confd)
	if err != nil {
		return nil, fmt.Errorf("failed to get dirs under %s : %v", confd, err)
	}

	count := len(dirs)
	if count == 0 {
		return dirs, nil
	}

	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if strings.HasPrefix(dirs[i], "input.") {
			names = append(names, dirs[i][6:])
		}
	}

	return names, nil
}
