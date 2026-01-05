package util

import (
	keyset "flashcat.cloud/categraf/set/key"
	"strings"

	coreconfig "flashcat.cloud/categraf/config"
)

func Debug() bool {
	if coreconfig.Config.DebugMode && strings.Contains(coreconfig.Config.InputFilters, keyset.LogsAgent) {
		return true
	}
	return false
}