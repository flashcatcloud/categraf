package snmp_trap

import (
	"flashcat.cloud/categraf/pkg/snmp"
)

type gosmiTranslator struct {
}

func (t *gosmiTranslator) lookup(oid string) (snmp.MibEntry, error) {
	return snmp.TrapLookup(oid)
}

func newGosmiTranslator(paths []string) (*gosmiTranslator, error) {
	err := snmp.LoadMibsFromPath(paths, &snmp.GosmiMibLoader{})
	if err == nil {
		return &gosmiTranslator{}, nil
	}
	return nil, err
}
