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

type Instance struct {
	config.InstanceConfig

	ServiceAddress string          `toml:"service_address"`
	Timeout        config.Duration `toml:"timeout" deprecated:"1.20.0;unused option"`
	Version        string          `toml:"version"`
	Translator     string          `toml:"translator"`
	Path           []string        `toml:"path"`

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

func makeTrapHandler(s *Instance, slist *types.SampleList) gosnmp.TrapHandlerFunc {
	return func(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
		if s.DebugMod {
			log.Printf("Received Trap from: %s, packet content: %v", addr.IP.String(), packet.SafeString())
		}
		fields := map[string]interface{}{}
		tags := map[string]string{}

		tags["version"] = packet.Version.String()
		tags["source"] = addr.IP.String()

		if packet.Version == gosnmp.Version1 {
			// Follow the procedure described in RFC 2576 3.1 to
			// translate a v1 trap to v2.
			var trapOid string

			if packet.GenericTrap >= 0 && packet.GenericTrap < 6 {
				trapOid = ".1.3.6.1.6.3.1.1.5." + strconv.Itoa(packet.GenericTrap+1)
			} else if packet.GenericTrap == 6 {
				trapOid = packet.Enterprise + ".0." + strconv.Itoa(packet.SpecificTrap)
			}

			if trapOid != "" {
				e, err := s.transl.lookup(trapOid)
				if err != nil {
					log.Printf("Error resolving V1 OID, oid=%s, source=%s: %v", trapOid, tags["source"], err)
					return
				}
				setTrapOid(tags, trapOid, e)
			}

			if packet.AgentAddress != "" {
				tags["agent_address"] = packet.AgentAddress
			}

			fields["sysUpTimeInstance"] = packet.Timestamp
		}

		for _, v := range packet.Variables {
			// Use system mibs to resolve oids.  Don't fall back to
			// numeric oid because it's not useful enough to the end
			// user and can be difficult to translate or remove from
			// the database later.

			var value interface{}

			// todo: format the pdu value based on its snmp type and
			// the mib's textual convention.  The snmp input plugin
			// only handles textual convention for ip and mac
			// addresses

			switch v.Type {
			case gosnmp.ObjectIdentifier:
				val, ok := v.Value.(string)
				if !ok {
					log.Println("E! Error getting value OID")
					return
				}

				var e snmp.MibEntry
				var err error
				e, err = s.transl.lookup(val)
				if nil != err {
					log.Printf("Error resolving value OID, oid=%s, source=%s: %v", val, tags["source"], err)
					return
				}

				value = e.OidText

				// 1.3.6.1.6.3.1.1.4.1.0 is SNMPv2-MIB::snmpTrapOID.0.
				// If v.Name is this oid, set a tag of the trap name.
				if v.Name == ".1.3.6.1.6.3.1.1.4.1.0" {
					setTrapOid(tags, val, e)
					continue
				}
			default:
				value = v.Value
			}

			e, err := s.transl.lookup(v.Name)
			if nil != err {
				log.Printf("Error resolving OID oid=%s, source=%s: %v", v.Name, tags["source"], err)
				return
			}

			name := e.OidText

			fields[name] = value
		}

		if packet.Version == gosnmp.Version3 {
			if packet.ContextName != "" {
				tags["context_name"] = packet.ContextName
			}
			if packet.ContextEngineID != "" {
				// SNMP RFCs like 3411 and 5343 show engine ID as a hex string
				tags["engine_id"] = fmt.Sprintf("%x", packet.ContextEngineID)
			}
		}
		for k, v := range fields {
			slist.PushFront(types.NewSample(inputName, k, v, tags).SetTime(time.Now()))
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
