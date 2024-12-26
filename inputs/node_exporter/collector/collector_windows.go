package collector

func paramsInit(params map[string]string) {
	pathInit(params)
	ntpCollectorInit(params)
	runitCollectorInit(params)
	supervisordCollectorInit(params)
	textFileCollectorInit(params)
	fileCollectorInit(params)
	crontabCollectorInit(params)
}
