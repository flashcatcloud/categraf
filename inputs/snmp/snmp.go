package snmp

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
)

const inputName = `snmp`

type Translator interface {
	SnmpTranslate(oid string) (
		mibName string, oidNum string, oidText string,
		conversion string,
		err error,
	)

	SnmpTable(oid string) (
		mibName string, oidNum string, oidText string,
		fields []Field,
		err error,
	)

	SnmpFormatEnum(oid string, value interface{}, full bool) (
		formatted string,
		err error,
	)

	SetDebugMode(bool)
}

type ClientConfig struct {
	// Timeout to wait for a response.
	Timeout config.Duration `toml:"timeout"`
	Retries int             `toml:"retries"`
	// Values: 1, 2, 3
	Version              uint8 `toml:"version"`
	UnconnectedUDPSocket bool  `toml:"unconnected_udp_socket"`
	// Path to mib files
	Path []string `toml:"path"`
	// Translator implementation
	Translator string `toml:"translator"`

	// Parameters for Version 1 & 2
	Community string `toml:"community"`

	// Parameters for Version 2 & 3
	MaxRepetitions uint32 `toml:"max_repetitions"`

	// Parameters for Version 3
	ContextName string `toml:"context_name"`
	// Values: "noAuthNoPriv", "authNoPriv", "authPriv"
	SecLevel string `toml:"sec_level"`
	SecName  string `toml:"sec_name"`
	// Values: "MD5", "SHA", "". Default: ""
	AuthProtocol string `toml:"auth_protocol"`
	AuthPassword string `toml:"auth_password"`
	// Values: "DES", "AES", "". Default: ""
	PrivProtocol string `toml:"priv_protocol"`
	PrivPassword string `toml:"priv_password"`
	EngineID     string `toml:"-"`
	EngineBoots  uint32 `toml:"-"`
	EngineTime   uint32 `toml:"-"`

	// AppOpts
	AppOpts map[string]interface{} `toml:"app_opts"`
	MaxOids int                    `toml:"max_oids"`
}

// Snmp holds the configuration for the plugin.
type Snmp struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`

	Mappings map[string]map[string]string `toml:"mappings"`
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Snmp{}
	})
}

func (s *Snmp) Clone() inputs.Input {
	return &Snmp{}
}

func (s *Snmp) Name() string {
	return inputName
}

func (s *Snmp) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		if len(s.Instances[i].Mappings) == 0 {
			s.Instances[i].Mappings = s.Mappings
		} else {
			m := make(map[string]map[string]string)
			for k, v := range s.Mappings {
				m[k] = v
			}
			for k, v := range s.Instances[i].Mappings {
				m[k] = v
			}
			s.Instances[i].Mappings = m
		}
		ret[i] = s.Instances[i]
	}
	return ret
}

func (s *Snmp) Drop() {
	for _, i := range s.Instances {
		i.Drop()
	}
}
