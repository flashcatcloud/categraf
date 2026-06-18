package main

import (
	"log"
	"path"
	"sync"
	"time"

	"flashcat.cloud/categraf/agent"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/pkg/reloadwatcher"
	"flashcat.cloud/categraf/writer"
)

const configReloadDebounce = time.Second

type managedAgent interface {
	Start()
	Stop()
}

type configWatcher interface {
	Close() error
}

type agentRuntimeDeps struct {
	loadConfig   func() (*config.ConfigType, error)
	buildWriters func(*config.ConfigType) (map[string]writer.Writer, error)
	applyConfig  func(*config.ConfigType)
	applyWriters func(map[string]writer.Writer)
	newAgent     func() (managedAgent, error)
	initLog      func(string)
	startWatcher func(string, time.Duration, func()) (configWatcher, error)
}

type agentRuntime struct {
	mu      sync.Mutex
	agent   managedAgent
	watcher configWatcher
	deps    agentRuntimeDeps
}

func newDefaultAgentRuntime(ag *agent.Agent, configDir string, debugLevel int, debugMode, testMode bool, interval int64, inputFilters string) *agentRuntime {
	return &agentRuntime{
		agent: ag,
		deps: agentRuntimeDeps{
			loadConfig: func() (*config.ConfigType, error) {
				return config.LoadConfig(configDir, debugLevel, debugMode, testMode, interval, inputFilters)
			},
			buildWriters: func(conf *config.ConfigType) (map[string]writer.Writer, error) {
				return writer.BuildWriters(conf.Writers)
			},
			applyConfig: func(conf *config.ConfigType) {
				config.Config = conf
			},
			applyWriters: writer.ApplyWriters,
			newAgent: func() (managedAgent, error) {
				return agent.NewAgent()
			},
			initLog: initLog,
			startWatcher: func(target string, debounce time.Duration, onChange func()) (configWatcher, error) {
				return reloadwatcher.Start(target, debounce, onChange)
			},
		},
	}
}

func (rt *agentRuntime) Start() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.agent.Start()
	rt.reconcileWatcherLocked()
}

func (rt *agentRuntime) Stop() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.stopWatcherLocked()
	rt.agent.Stop()
}

func (rt *agentRuntime) Reload(reason string) error {
	return rt.reload(reason)
}

func (rt *agentRuntime) reload(reason string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	log.Println("I! agent runtime reloading:", reason)
	nextConfig, err := rt.deps.loadConfig()
	if err != nil {
		return err
	}

	nextWriters, err := rt.deps.buildWriters(nextConfig)
	if err != nil {
		return err
	}

	currentConfig := config.Config
	currentAgent := rt.agent
	rt.deps.applyConfig(nextConfig)

	nextAgent, err := rt.deps.newAgent()
	if err != nil {
		rt.deps.applyConfig(currentConfig)
		return err
	}

	currentAgent.Stop()
	rt.deps.applyWriters(nextWriters)
	rt.agent = nextAgent
	rt.deps.initLog(nextConfig.Log.FileName)
	rt.agent.Start()
	rt.reconcileWatcherLocked()
	log.Println("I! agent runtime reloaded:", reason)
	return nil
}

func (rt *agentRuntime) reconcileWatcherLocked() {
	if config.Config == nil || !config.Config.Global.ReloadOnChange {
		rt.stopWatcherLocked()
		return
	}
	if rt.watcher != nil {
		return
	}

	target := path.Join(config.Config.ConfigDir, "config.toml")
	watcher, err := rt.deps.startWatcher(target, configReloadDebounce, func() {
		if err := rt.Reload("config.toml changed"); err != nil {
			log.Println("E! reload config.toml failed:", err)
		}
	})
	if err != nil {
		log.Println("E! watch config.toml failed:", err)
		return
	}
	rt.watcher = watcher
	log.Println("I! watching config.toml for changes:", target)
}

func (rt *agentRuntime) stopWatcherLocked() {
	if rt.watcher == nil {
		return
	}
	if err := rt.watcher.Close(); err != nil {
		log.Println("E! stop config.toml watcher failed:", err)
	}
	rt.watcher = nil
}
