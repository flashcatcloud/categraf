package http_response

import (
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func (c *HTTPResponse) LogsConfig() []*logsconfig.LogsConfig {
	return nil
}
