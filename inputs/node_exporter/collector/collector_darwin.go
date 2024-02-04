package collector

func paramsInit(params map[string]string) {
	pathInit(params)
	diskstatsCollectorInit(params)
	filesystemCollectorInit(params)
	netDevCollectorInit(params)
	ntpCollectorInit(params)
	powerSupplyClassCollectorInit(params)
	runitCollectorInit(params)
	supervisordCollectorInit(params)
	textFileCollectorInit(params)
	fileCollectorInit(params)
	crontabCollectorInit(params)
}
