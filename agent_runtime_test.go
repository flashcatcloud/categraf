package main

import (
	"errors"
	"testing"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/writer"
)

type fakeRuntimeAgent struct {
	starts int
	stops  int
}

func (a *fakeRuntimeAgent) Start() {
	a.starts++
}

func (a *fakeRuntimeAgent) Stop() {
	a.stops++
}

func TestAgentRuntimeReloadStartsNewAgentAfterSuccessfulConfigLoad(t *testing.T) {
	oldConfig := config.Config
	defer func() {
		config.Config = oldConfig
	}()

	currentConfig := &config.ConfigType{Log: config.Log{FileName: "stdout"}}
	nextConfig := &config.ConfigType{Log: config.Log{FileName: "stderr"}}
	config.Config = currentConfig

	oldAgent := &fakeRuntimeAgent{}
	nextAgent := &fakeRuntimeAgent{}
	initLogFile := ""
	appliedWriters := false

	rt := &agentRuntime{
		agent: oldAgent,
		deps: agentRuntimeDeps{
			loadConfig: func() (*config.ConfigType, error) {
				return nextConfig, nil
			},
			buildWriters: func(*config.ConfigType) (map[string]writer.Writer, error) {
				return map[string]writer.Writer{}, nil
			},
			applyConfig: func(c *config.ConfigType) {
				config.Config = c
			},
			applyWriters: func(map[string]writer.Writer) {
				appliedWriters = true
			},
			newAgent: func() (managedAgent, error) {
				return nextAgent, nil
			},
			initLog: func(file string) {
				initLogFile = file
			},
		},
	}

	if err := rt.reload("test"); err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if oldAgent.stops != 1 {
		t.Fatalf("old agent stops = %d, want 1", oldAgent.stops)
	}
	if nextAgent.starts != 1 {
		t.Fatalf("next agent starts = %d, want 1", nextAgent.starts)
	}
	if rt.agent != nextAgent {
		t.Fatal("runtime did not install next agent")
	}
	if config.Config != nextConfig {
		t.Fatal("runtime did not install next config")
	}
	if !appliedWriters {
		t.Fatal("runtime did not apply writers")
	}
	if initLogFile != "stderr" {
		t.Fatalf("initLog file = %q, want stderr", initLogFile)
	}
}

func TestAgentRuntimeReloadKeepsOldAgentAndConfigWhenNewAgentFails(t *testing.T) {
	oldConfig := config.Config
	defer func() {
		config.Config = oldConfig
	}()

	currentConfig := &config.ConfigType{Log: config.Log{FileName: "stdout"}}
	nextConfig := &config.ConfigType{Log: config.Log{FileName: "stderr"}}
	config.Config = currentConfig

	oldAgent := &fakeRuntimeAgent{}
	appliedWriters := false

	rt := &agentRuntime{
		agent: oldAgent,
		deps: agentRuntimeDeps{
			loadConfig: func() (*config.ConfigType, error) {
				return nextConfig, nil
			},
			buildWriters: func(*config.ConfigType) (map[string]writer.Writer, error) {
				return map[string]writer.Writer{}, nil
			},
			applyConfig: func(c *config.ConfigType) {
				config.Config = c
			},
			applyWriters: func(map[string]writer.Writer) {
				appliedWriters = true
			},
			newAgent: func() (managedAgent, error) {
				return nil, errors.New("new agent failed")
			},
			initLog: func(string) {},
		},
	}

	if err := rt.reload("test"); err == nil {
		t.Fatal("reload error = nil, want error")
	}
	if oldAgent.stops != 0 {
		t.Fatalf("old agent stops = %d, want 0", oldAgent.stops)
	}
	if rt.agent != oldAgent {
		t.Fatal("runtime replaced old agent after failed reload")
	}
	if config.Config != currentConfig {
		t.Fatal("runtime did not restore current config after failed reload")
	}
	if appliedWriters {
		t.Fatal("runtime applied writers after failed reload")
	}
}
