package service

import "sync"

type service struct {
	name          string
	isPortOpenned bool
	isService     bool
}

var (
	set map[string]service = map[string]service{
		"zabbix":      service{name: "zabbix", isPortOpenned: true, isService: true},
		"rocketmq":    service{name: "rocketmq", isPortOpenned: true, isService: true},
		"snmp_zabbix": service{name: "snmp_zabbix", isPortOpenned: false, isService: true},
		"snmp_trap":   service{name: "snmp_trap", isPortOpenned: false, isService: false},
	}
	lock = sync.Mutex{}
)

func IsServiceInput(inputKey string) bool {
	lock.Lock()
	defer lock.Unlock()
	if svc, ok := set[inputKey]; ok {
		return svc.isService
	}
	return false
}

func CanRunMultipleInstances(inputKey string) bool {
	lock.Lock()
	defer lock.Unlock()
	if svc, ok := set[inputKey]; ok {
		return !svc.isPortOpenned
	}
	return true
}

func Register(inputKey string, isPortOpened, isService bool) {
	lock.Lock()
	defer lock.Unlock()
	if svc, ok := set[inputKey]; ok {
		svc.isPortOpenned = isPortOpened
		svc.isService = isService
	} else {
		set[inputKey] = service{name: inputKey, isPortOpenned: isPortOpened, isService: isService}
	}
}
