package manager

import (
	"fmt"
)

const (
	ecsNamespace = "acs_ecs_dashboard"
)

func (m *Manager) EcsKey(instanceID string) string {
	return fmt.Sprintf("%s||%s", ecsNamespace, instanceID)
}
