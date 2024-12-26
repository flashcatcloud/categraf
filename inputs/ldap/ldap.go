package ldap

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/go-ldap/ldap/v3"

	commontls "flashcat.cloud/categraf/pkg/tls"
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "ldap"

type LDAP struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type Instance struct {
	config.InstanceConfig
	Server            string        `toml:"server"`
	Dialect           string        `toml:"dialect"`
	BindDn            string        `toml:"bind_dn"`
	BindPassword      config.Secret `toml:"bind_password"`
	ReverseFieldNames bool          `toml:"reverse_field_names"`
	commontls.ClientConfig

	tlsCfg   *tls.Config
	requests []request
	mode     string
	host     string
	port     string
}

type request struct {
	query   *ldap.SearchRequest
	convert func(*ldap.SearchResult, time.Time) []types.Metric
}

func (ins *Instance) Init() error {
	if ins.Server == "" {
		return types.ErrInstancesEmpty
		//ins.Server = "ldap://localhost:389"
	}

	u, err := url.Parse(ins.Server)
	if err != nil {
		return fmt.Errorf("parsing server failed: %w", err)
	}

	// Verify the server setting and set the defaults
	switch u.Scheme {
	case "ldap":
		if u.Port() == "" {
			u.Host = u.Host + ":389"
		}
		ins.UseTLS = false
	case "starttls":
		if u.Port() == "" {
			u.Host = u.Host + ":389"
		}
		ins.UseTLS = true
	case "ldaps":
		if u.Port() == "" {
			u.Host = u.Host + ":636"
		}
		ins.UseTLS = true
	default:
		return fmt.Errorf("invalid scheme: %q", u.Scheme)
	}
	ins.mode = u.Scheme
	ins.Server = u.Host
	ins.host, ins.port = u.Hostname(), u.Port()

	// Setup TLS configuration
	tlsCfg, err := ins.ClientConfig.TLSConfig()
	if err != nil {
		return fmt.Errorf("creating TLS config failed: %w", err)
	}

	ins.tlsCfg = tlsCfg

	// Initialize the search request(s)
	switch ins.Dialect {
	case "", "openldap":
		ins.requests = ins.newOpenLDAPConfig()
	case "389ds":
		ins.requests = ins.new389dsConfig()
	default:
		return fmt.Errorf("invalid dialect %q", ins.Dialect)
	}

	return nil
}

func (ins *Instance) Gather(slist *types.SampleList) {
	conn, err := ins.connect()
	if err != nil {
		log.Println("E! failed to connect the server:", ins.Server, "error:", err)
		return
	}
	defer conn.Close()

	for _, req := range ins.requests {
		result, err := conn.Search(req.query)
		if err != nil {
			log.Println("E! failed to search the server:", ins.Server, "error:", err)
			continue
		}
		s, err := ins.gather(req, result)
		if err != nil {
			log.Println("E! failed to gather metrics: ", err)
			return
		}
		slist.PushFrontN(s)
	}
}

func (ins *Instance) gather(req request, result *ldap.SearchResult) ([]*types.Sample, error) {
	if len(result.Entries) <= 0 {
		return nil, errors.New("E! ldap Entries is less than or equal to 0")
	}
	now := time.Now()
	samples := make([]*types.Sample, 0, len(req.convert(result, now)))
	// Collect metrics
	for _, m := range req.convert(result, now) {
		for name, value := range m.Fields() {
			sample := types.NewSample(m.Name(), name, value, m.Tags()).
				SetTime(m.Time().Local())

			samples = append(samples, sample)
		}
	}
	return samples, nil
}

func (ins *Instance) connect() (*ldap.Conn, error) {
	var conn *ldap.Conn
	switch ins.mode {
	case "ldap":
		var err error
		conn, err = ldap.Dial("tcp", ins.Server)
		if err != nil {
			return nil, err
		}
	case "ldaps":
		var err error
		conn, err = ldap.DialTLS("tcp", ins.Server, ins.tlsCfg)
		if err != nil {
			return nil, err
		}
	case "starttls":
		var err error
		conn, err = ldap.Dial("tcp", ins.Server)
		if err != nil {
			return nil, err
		}
		if err := conn.StartTLS(ins.tlsCfg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid tls_mode: %s", ins.mode)
	}

	if ins.BindDn == "" && ins.BindPassword.Empty() {
		return conn, nil
	}

	// Bind username and password
	passwd, err := ins.BindPassword.Get()
	if err != nil {
		return nil, fmt.Errorf("getting password failed: %w", err)
	}
	defer passwd.Destroy()

	if err := conn.Bind(ins.BindDn, passwd.String()); err != nil {
		return nil, fmt.Errorf("binding credentials failed: %w", err)
	}

	return conn, nil
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &LDAP{}
	})
}

func (l *LDAP) Clone() inputs.Input {
	return &LDAP{}
}

func (l *LDAP) Name() string {
	return inputName
}

func (l *LDAP) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(l.Instances))
	for i := 0; i < len(l.Instances); i++ {
		ret[i] = l.Instances[i]
	}
	return ret
}
