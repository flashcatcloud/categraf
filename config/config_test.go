package config

import "testing"

func TestResolveLogSettingsUsesConfigVerbosity(t *testing.T) {
	logCfg := Log{
		Level:     "info",
		Verbosity: 10,
	}

	debugMode, debugLevel, err := resolveLogSettings(logCfg, 0, false, false, false)
	if err != nil {
		t.Fatalf("resolveLogSettings returned error: %v", err)
	}

	if !debugMode {
		t.Fatalf("expected debug mode to be enabled when verbosity > 0")
	}

	if debugLevel != 10 {
		t.Fatalf("expected debug level 10, got %d", debugLevel)
	}
}

func TestResolveLogSettingsMapsDebugLevelToVerbosityOne(t *testing.T) {
	logCfg := Log{
		Level: "debug",
	}

	debugMode, debugLevel, err := resolveLogSettings(logCfg, 0, false, false, false)
	if err != nil {
		t.Fatalf("resolveLogSettings returned error: %v", err)
	}

	if !debugMode {
		t.Fatalf("expected debug mode to be enabled for debug level")
	}

	if debugLevel != 1 {
		t.Fatalf("expected debug level 1 for debug log level, got %d", debugLevel)
	}
}

func TestResolveLogSettingsCliOverridesConfig(t *testing.T) {
	logCfg := Log{
		Level:     "debug",
		Verbosity: 10,
	}

	debugMode, debugLevel, err := resolveLogSettings(logCfg, 2, false, true, false)
	if err != nil {
		t.Fatalf("resolveLogSettings returned error: %v", err)
	}

	if !debugMode {
		t.Fatalf("expected debug mode to stay enabled when cli debug level is set")
	}

	if debugLevel != 2 {
		t.Fatalf("expected cli debug level 2 to win, got %d", debugLevel)
	}
}

func TestResolveLogSettingsRejectsUnknownLevel(t *testing.T) {
	_, _, err := resolveLogSettings(Log{Level: "verbose"}, 0, false, false, false)
	if err == nil {
		t.Fatal("expected invalid log level to return error")
	}
}
