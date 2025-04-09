package install

import (
	"github.com/kardianos/service"
)

const (
	ServiceName = "categraf"
)

var (
	serviceConfig = &service.Config{
		// 服务显示名称
		Name: ServiceName,
		// 服务名称
		DisplayName: "categraf",
		// 服务描述
		Description: "Opensource telemetry collector",
	}
)

func ServiceConfig(userMode bool) *service.Config {
	return serviceConfig
}
