package snmp_trap

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/snmp"
	"flashcat.cloud/categraf/types"
)

const inputName = "snmp_trap"

var defaultTimeout = config.Duration(time.Second * 5)

var sampleConfig string

type translator interface {
	lookup(oid string) (snmp.MibEntry, error)
}

type SnmpTrap struct {
	config.PluginConfig

	Instances []*Instance `toml:"instances"`
}

type TrapVarbind struct {
	Oid  string `toml:"oid"`
	Name string `toml:"name"`
}

type TrapMapping struct {
	Oid     string        `toml:"oid"`
	Name    string        `toml:"name"`
	Value   string        `toml:"value"`
	Varbind []TrapVarbind `toml:"varbind"`
}

type Instance struct {
	config.InstanceConfig

	ServiceAddress string            `toml:"service_address"`
	Timeout        config.Duration   `toml:"timeout" deprecated:"1.20.0;unused option"`
	Version        string            `toml:"version"`
	Translator     string            `toml:"translator"`
	Path           []string          `toml:"path"`
	FieldsToLabels []string          `toml:"fields_to_labels"`
	VarbindMapping map[string]string `toml:"varbind_mapping"`
	TrapMapping    []TrapMapping     `toml:"trap_mapping"`

	// Settings for version 3
	// Values: "noAuthNoPriv", "authNoPriv", "authPriv"
	SecLevel string        `toml:"sec_level"`
	SecName  config.Secret `toml:"sec_name"`
	// Values: "MD5", "SHA", "". Default: ""
	AuthProtocol string        `toml:"auth_protocol"`
	AuthPassword config.Secret `toml:"auth_password"`
	// Values: "DES", "AES", "". Default: ""
	PrivProtocol string        `toml:"priv_protocol"`
	PrivPassword config.Secret `toml:"priv_password"`

	listener *gosnmp.TrapListener
	timeFunc func() time.Time
	errCh    chan error

	makeHandlerWrapper func(gosnmp.TrapHandlerFunc) gosnmp.TrapHandlerFunc

	transl translator
	slist  *types.SampleList
}

func (a *SnmpTrap) Clone() inputs.Input {
	return &SnmpTrap{}
}

func (a *SnmpTrap) Name() string {
	return inputName
}

var _ inputs.SampleGatherer = new(Instance)
var _ inputs.Input = new(SnmpTrap)
var _ inputs.InstancesGetter = new(SnmpTrap)

func (s *Instance) Gather(slist *types.SampleList) {
	slist.PushFrontN(s.slist.PopBackAll())
	return
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &SnmpTrap{}
	})
}

func (s *Instance) SetTranslator(name string) {
	s.Translator = name
}

func (s *Instance) Init() error {
	var err error
	switch s.Translator {
	case "gosmi":
		s.transl, err = newGosmiTranslator(s.Path)
		if err != nil {
			return err
		}
	case "netsnmp", "":
		s.SetTranslator("netsnmp")
		s.transl = newNetsnmpTranslator(s.Timeout)
	}

	if err != nil {
		log.Printf("Could not get path %v", err)
	}

	if len(s.ServiceAddress) == 0 {
		return types.ErrInstancesEmpty
	}
	s.slist = types.NewSampleList()
	return s.start()
}

func (s *Instance) start() error {
	s.listener = gosnmp.NewTrapListener()
	s.listener.OnNewTrap = makeTrapHandler(s, s.slist)

	// gosnmp.Default is a pointer, using this more than once
	// has side effects
	defaults := *gosnmp.Default
	s.listener.Params = &defaults
	// s.listener.Params.Logger = gosnmp.NewLogger()

	switch s.Version {
	case "3":
		s.listener.Params.Version = gosnmp.Version3
	case "2c":
		s.listener.Params.Version = gosnmp.Version2c
	case "1":
		s.listener.Params.Version = gosnmp.Version1
	default:
		s.listener.Params.Version = gosnmp.Version2c
	}

	if s.listener.Params.Version == gosnmp.Version3 {
		s.listener.Params.SecurityModel = gosnmp.UserSecurityModel

		switch strings.ToLower(s.SecLevel) {
		case "noauthnopriv", "":
			s.listener.Params.MsgFlags = gosnmp.NoAuthNoPriv
		case "authnopriv":
			s.listener.Params.MsgFlags = gosnmp.AuthNoPriv
		case "authpriv":
			s.listener.Params.MsgFlags = gosnmp.AuthPriv
		default:
			return fmt.Errorf("unknown security level %q", s.SecLevel)
		}

		var authenticationProtocol gosnmp.SnmpV3AuthProtocol
		switch strings.ToLower(s.AuthProtocol) {
		case "md5":
			authenticationProtocol = gosnmp.MD5
		case "sha":
			authenticationProtocol = gosnmp.SHA
		// case "sha224":
		//	authenticationProtocol = gosnmp.SHA224
		// case "sha256":
		//	authenticationProtocol = gosnmp.SHA256
		// case "sha384":
		//	authenticationProtocol = gosnmp.SHA384
		// case "sha512":
		//	authenticationProtocol = gosnmp.SHA512
		case "":
			authenticationProtocol = gosnmp.NoAuth
		default:
			return fmt.Errorf("unknown authentication protocol %q", s.AuthProtocol)
		}

		var privacyProtocol gosnmp.SnmpV3PrivProtocol
		switch strings.ToLower(s.PrivProtocol) {
		case "aes":
			privacyProtocol = gosnmp.AES
		case "des":
			privacyProtocol = gosnmp.DES
		case "aes192":
			privacyProtocol = gosnmp.AES192
		case "aes192c":
			privacyProtocol = gosnmp.AES192C
		case "aes256":
			privacyProtocol = gosnmp.AES256
		case "aes256c":
			privacyProtocol = gosnmp.AES256C
		case "":
			privacyProtocol = gosnmp.NoPriv
		default:
			return fmt.Errorf("unknown privacy protocol %q", s.PrivProtocol)
		}

		secnameSecret, err := s.SecName.Get()
		if err != nil {
			return fmt.Errorf("getting secname failed: %w", err)
		}
		secname := secnameSecret.String()
		secnameSecret.Destroy()

		privPasswdSecret, err := s.PrivPassword.Get()
		if err != nil {
			return fmt.Errorf("getting secname failed: %w", err)
		}
		privPasswd := privPasswdSecret.String()
		privPasswdSecret.Destroy()

		authPasswdSecret, err := s.AuthPassword.Get()
		if err != nil {
			return fmt.Errorf("getting secname failed: %w", err)
		}
		authPasswd := authPasswdSecret.String()
		authPasswdSecret.Destroy()

		s.listener.Params.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 secname,
			PrivacyProtocol:          privacyProtocol,
			PrivacyPassphrase:        privPasswd,
			AuthenticationPassphrase: authPasswd,
			AuthenticationProtocol:   authenticationProtocol,
		}
	}

	// wrap the handler, used in unit tests
	if nil != s.makeHandlerWrapper {
		s.listener.OnNewTrap = s.makeHandlerWrapper(s.listener.OnNewTrap)
	}

	split := strings.SplitN(s.ServiceAddress, "://", 2)
	if len(split) != 2 {
		return fmt.Errorf("invalid service address: %s", s.ServiceAddress)
	}

	protocol := split[0]
	addr := split[1]

	// gosnmp.TrapListener currently supports udp only.  For forward
	// compatibility, require udp in the service address
	if protocol != "udp" {
		return fmt.Errorf("unknown protocol %q in %q", protocol, s.ServiceAddress)
	}

	// If (*TrapListener).Listen immediately returns an error we need
	// to return it from this function.  Use a channel to get it here
	// from the goroutine.  Buffer one in case Listen returns after
	// Listening but before our Close is called.
	s.errCh = make(chan error, 1)
	go func() {
		s.errCh <- s.listener.Listen(addr)
	}()

	select {
	case <-s.listener.Listening():
		log.Printf("Listening on %s", s.ServiceAddress)
	case err := <-s.errCh:
		return err
	}

	return nil
}

func (s *Instance) Drop() {
	s.listener.Close()
	err := <-s.errCh
	if nil != err {
		log.Printf("Error stopping trap listener %v", err)
	}
}

func setTrapOid(tags map[string]string, oid string, e snmp.MibEntry) {
	tags["oid"] = oid
	tags["name"] = e.OidText
	tags["mib"] = e.MibName
}

// hasOIDPrefix checks if the oid starts with the given prefix at a valid segment boundary.
// It guards against digit-boundary mismatches (e.g., prefix ".1.3" incorrectly matching ".1.30").
func hasOIDPrefix(oid, prefix string) bool {
	if prefix == "" {
		return false
	}
	prefix = strings.TrimSuffix(prefix, ".")
	return oid == prefix || strings.HasPrefix(oid, prefix+".")
}

func makeTrapHandler(s *Instance, slist *types.SampleList) gosnmp.TrapHandlerFunc {
	return func(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
		if s.DebugMod {
			log.Printf("Received Trap from: %s, packet content: %v", addr.IP.String(), packet.SafeString())
		}
		fields := map[string]interface{}{}
		tags := map[string]string{}

		tags["version"] = packet.Version.String()
		tags["source"] = addr.IP.String()

		var trapOid string
		var trapName string

		// 1. Identify trap OID
		if packet.Version == gosnmp.Version1 {
			// Follow the procedure described in RFC 2576 3.1 to translate a v1 trap to v2.
			if packet.GenericTrap >= 0 && packet.GenericTrap < 6 {
				trapOid = ".1.3.6.1.6.3.1.1.5." + strconv.Itoa(packet.GenericTrap+1)
			} else if packet.GenericTrap == 6 {
				trapOid = packet.Enterprise + ".0." + strconv.Itoa(packet.SpecificTrap)
			}

			if packet.AgentAddress != "" {
				tags["agent_address"] = packet.AgentAddress
			}
			// sysUpTime is implicit in v1 header, push to fields to act like a varbind
			fields["sysUpTimeInstance"] = packet.Timestamp
		} else {
			for _, v := range packet.Variables {
				if v.Name == ".1.3.6.1.6.3.1.1.4.1.0" {
					if val, ok := v.Value.(string); ok {
						trapOid = val
					}
					break
				}
			}
		}

		if trapOid != "" {
			e, err := s.transl.lookup(trapOid)
			if err == nil {
				trapName = e.OidText
				setTrapOid(tags, trapOid, e)
			} else {
				trapName = trapOid
				tags["oid"] = trapOid
				tags["name"] = trapName
			}
		}

		// 2. Identify active TrapMapping (longest prefix wins)
		var trapMatchedMapping *TrapMapping
		bestMappingLen := 0
		for i := range s.TrapMapping {
			normalizedOid := strings.TrimSuffix(s.TrapMapping[i].Oid, ".")
			if hasOIDPrefix(trapOid, normalizedOid) && len(normalizedOid) > bestMappingLen {
				bestMappingLen = len(normalizedOid)
				trapMatchedMapping = &s.TrapMapping[i]
			}
		}

		isAggregated := false
		if trapMatchedMapping != nil {
			if trapMatchedMapping.Name != "" {
				trapName = trapMatchedMapping.Name
				tags["name"] = trapName
			}
			isAggregated = true
		} else if len(s.FieldsToLabels) > 0 || len(s.VarbindMapping) > 0 {
			isAggregated = true
		}

		var coreValue interface{}
		// 3. Process Varbinds
		for _, v := range packet.Variables {
			// Skip snmpTrapOID.0 only when it was already successfully extracted.
			// If extraction failed (trapOid is empty), keep processing so the
			// varbind is not silently dropped.
			if v.Name == ".1.3.6.1.6.3.1.1.4.1.0" && trapOid != "" {
				continue
			}

			var value interface{}
			switch v.Type {
			case gosnmp.ObjectIdentifier:
				val, ok := v.Value.(string)
				if !ok {
					continue
				}
				e, err := s.transl.lookup(val)
				if err == nil {
					value = e.OidText
				} else {
					value = val
				}
			default:
				value = v.Value
			}

			varbindName := v.Name
			e, err := s.transl.lookup(v.Name)
			if err == nil {
				varbindName = e.OidText
			}

			isLabel := false
			labelName := ""
			usedAsValue := false

			// Step A: Check TrapMapping
			if trapMatchedMapping != nil {
				// Check if it's the core value
				if trapMatchedMapping.Value != "" && hasOIDPrefix(v.Name, trapMatchedMapping.Value) {
					coreValue = value
					usedAsValue = true
				} else {
					// Check varbind labels (longest prefix wins)
					bestVBLen := 0
					for _, vbMapping := range trapMatchedMapping.Varbind {
						normalizedOid := strings.TrimSuffix(vbMapping.Oid, ".")
						if hasOIDPrefix(v.Name, normalizedOid) && len(normalizedOid) > bestVBLen {
							bestVBLen = len(normalizedOid)
							isLabel = true
							labelName = vbMapping.Name
						}
					}
				}
			}

			// Step B: Check Global configurations if not resolved yet
			if !isLabel && !usedAsValue {
				// 1. Rename via VarbindMapping (longest prefix wins)
				bestPrefix := ""
				bestMappedName := ""
				for prefix, mappedName := range s.VarbindMapping {
					normalizedPrefix := strings.TrimSuffix(prefix, ".")
					if hasOIDPrefix(v.Name, normalizedPrefix) && len(normalizedPrefix) > len(bestPrefix) {
						bestPrefix = normalizedPrefix
						bestMappedName = mappedName
					}
				}
				if bestPrefix != "" {
					suffix := v.Name[len(bestPrefix):]
					varbindName = bestMappedName + suffix
				}

				// 2. Check FieldsToLabels whitelist (against renamed name)
				for _, allowedField := range s.FieldsToLabels {
					if varbindName == allowedField || strings.HasPrefix(varbindName, allowedField+".") {
						isLabel = true
						labelName = allowedField // use the base name (no suffix)
						break
					}
				}
			}

			if usedAsValue {
				continue
			} else if isLabel && labelName != "" {
				tags[labelName] = fmt.Sprintf("%v", value)
			} else {
				fields[varbindName] = value
			}
		}

		// Also check fields populated implicitly (like sysUpTimeInstance in v1)
		if v1SysUpTime, exists := fields["sysUpTimeInstance"]; exists {
			if trapMatchedMapping != nil && hasOIDPrefix(".1.3.6.1.2.1.1.3.0", trapMatchedMapping.Value) {
				coreValue = v1SysUpTime
				delete(fields, "sysUpTimeInstance")
			}
		}

		if packet.Version == gosnmp.Version3 {
			if packet.ContextName != "" {
				tags["context_name"] = packet.ContextName
			}
			if packet.ContextEngineID != "" {
				tags["engine_id"] = fmt.Sprintf("%x", packet.ContextEngineID)
			}
		}

		now := time.Now()
		if s.timeFunc != nil {
			now = s.timeFunc()
		}

		// 4. Generate Core Metric
		if isAggregated {
			if coreValue == nil {
				coreValue = 1
			}
			if trapName == "" {
				trapName = trapOid
			}
			if trapName != "" {
				sanitizedTrapName := strings.ReplaceAll(strings.TrimPrefix(trapName, "."), ".", "_")
				slist.PushFront(types.NewSample(inputName, sanitizedTrapName, coreValue, tags).SetTime(now))
			}
		}

		// 5. Generate Dispersed Metrics
		for k, v := range fields {
			metricKey := strings.TrimPrefix(k, ".")
			slist.PushFront(types.NewSample(inputName, metricKey, v, tags).SetTime(now))
		}
	}
}

func (s *SnmpTrap) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}