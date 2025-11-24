package install

import (
	"github.com/kardianos/service"
)

const (
	// freebsd的服务名中间不能有"-"
	ServiceName = "categraf"

	SysvScript = `#!/bin/sh
#
# PROVIDE: {{.Name}}
# REQUIRE: networking syslog
# KEYWORD:
# Add the following lines to /etc/rc.conf to enable the {{.Name}}:
#
# {{.Name}}_enable="YES"
#
. /etc/rc.subr
name="{{.Name}}"
rcvar="{{.Name}}_enable"
command="{{.Path}}"
pidfile="/var/run/$name.pid"
start_cmd="/opt/categraf/categraf -configs /opt/categraf/conf"
load_rc_config $name
run_rc_command "$1"
`
)

var (
	serviceConfig = &service.Config{
		// 服务显示名称
		Name: ServiceName,
		// 服务名称
		DisplayName: "categraf",
		// 服务描述
		Description: "Opensource telemetry collector",
		Option: service.KeyValue{
			"SysvScript": SysvScript,
		},
	}
)

func ServiceConfig(userMode bool) *service.Config {
	return serviceConfig
}
