package util

import (
	"strings"

	coreconfig "flashcat.cloud/categraf/config"
)

func Debug() bool {
	if coreconfig.Config.DebugMode && strings.Contains(coreconfig.Config.InputFilters, "logs-agent") {
		return true
	}
	return false
}
