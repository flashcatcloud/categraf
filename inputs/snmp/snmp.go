package snmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = `snmp`
const description = `Retrieves SNMP values from remote agents`
const sampleConfig = `
  ## Agent addresses to retrieve values from.
  ##   format:  agents = ["<scheme://><hostname>:<port>"]
  ##   scheme:  optional, either udp, udp4, udp6, tcp, tcp4, tcp6.
  ##            default is udp
  ##   port:    optional
  ##   example: agents = ["udp://127.0.0.1:161"]
  ##            agents = ["tcp://127.0.0.1:161"]
  ##            agents = ["udp4://v4only-snmp-agent"]
  agents = ["udp://127.0.0.1:161"]

  ## Timeout for each request.
  # timeout = "5s"

  ## SNMP version; can be 1, 2, or 3.
  # version = 2

  ## Path to mib files
  ## Used by the gosmi translator.
  ## To add paths when translating with netsnmp, use the MIBDIRS environment variable
  # path = ["/usr/share/snmp/mibs"]

  ## Agent host tag; the tag used to reference the source host
  # agent_host_tag = "agent_host"

  ## SNMP community string.
  # community = "public"

  ## Number of retries to attempt.
  # retries = 3

  ## The GETBULK max-repetitions parameter.
  # max_repetitions = 10

  ## SNMPv3 authentication and encryption options.
  ##
  ## Security Name.
  # sec_name = "myuser"
  ## Authentication protocol; one of "MD5", "SHA", "SHA224", "SHA256", "SHA384", "SHA512" or "".
  # auth_protocol = "MD5"
  ## Authentication password.
  # auth_password = "pass"
  ## Security Level; one of "noAuthNoPriv", "authNoPriv", or "authPriv".
  # sec_level = "authNoPriv"
  ## Context Name.
  # context_name = ""
  ## Privacy protocol used for encrypted messages; one of "DES", "AES" or "".
  # priv_protocol = ""
  ## Privacy password used for encrypted messages.
  # priv_password = ""

  ## Add fields and tables defining the variables you wish to collect.  This
  ## example collects the system uptime and interface variables.  Reference the
  ## full plugin documentation for configuration details.
`

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
}

// Snmp holds the configuration for the plugin.
type Snmp struct {
	config.PluginConfig
	Instances []*Instance `toml:"instances"`
}

type Instance struct {
	config.InstanceConfig
	// The SNMP agent to query. Format is [SCHEME://]ADDR[:PORT] (e.g.
	// udp://1.2.3.4:161).  If the scheme is not specified then "udp" is used.
	Agents []string `toml:"agents"`

	// The tag used to name the agent host
	AgentHostTag string `toml:"agent_host_tag"`

	ClientConfig

	Tables []Table `toml:"table"`

	// Name & Fields are the elements of a Table.
	// Telegraf chokes if we try to embed a Table. So instead we have to embed the
	// fields of a Table, and construct a Table during runtime.
	Name   string  `toml:"name"`
	Fields []Field `toml:"field"`

	connectionCache []snmpConnection

	translator Translator
}

func (ins *Instance) Init() error {

	if len(ins.Agents) == 0 {
		return types.ErrInstancesEmpty
	}

	switch ins.Translator {
	case "", "netsnmp":
		ins.translator = NewNetsnmpTranslator()
	default:
		return fmt.Errorf("invalid translator value")
	}

	ins.connectionCache = make([]snmpConnection, len(ins.Agents))

	for i := range ins.Tables {
		if err := ins.Tables[i].Init(ins.translator); err != nil {
			return fmt.Errorf("initializing table %s ins: %s", ins.Tables[i].Name, err)
		}
	}

	for i := range ins.Fields {
		if err := ins.Fields[i].init(ins.translator); err != nil {
			return fmt.Errorf("initializing field %s ins: %w", ins.Fields[i].Name, err)
		}
	}

	if len(ins.AgentHostTag) == 0 {
		ins.AgentHostTag = "agent_host"
	}

	return nil
}

// Table holds the configuration for an SNMP table.
type Table struct {
	// Name will be the name of the measurement.
	Name string `toml:"name"`

	// Which tags to inherit from the top-level config.
	InheritTags []string `toml:"inherit_tags"`

	// Adds each row's table index as a tag.
	IndexAsTag bool `toml:"index_as_tag"`

	// Fields is the tags and values to look up.
	Fields []Field `toml:"field"`

	// OID for automatic field population.
	// If provided, init() will populate Fields with all the table columns of the
	// given OID.
	Oid string `toml:"oid"`

	initialized bool `toml:"initialized"`
}

// Init builds & initializes the nested fields.
func (t *Table) Init(tr Translator) error {
	// makes sure oid or name is set in config file
	// otherwise snmp will produce metrics with an empty name
	if t.Oid == "" && t.Name == "" {
		return fmt.Errorf("SNMP table in config file is not named. One or both of the oid and name settings must be set")
	}

	if t.initialized {
		return nil
	}

	if err := t.initBuild(tr); err != nil {
		return err
	}

	secondaryIndexTablePresent := false
	// initialize all the nested fields
	for i := range t.Fields {
		if err := t.Fields[i].init(tr); err != nil {
			return fmt.Errorf("initializing field %s: %w", t.Fields[i].Name, err)
		}
		if t.Fields[i].SecondaryIndexTable {
			if secondaryIndexTablePresent {
				return fmt.Errorf("only one field can be SecondaryIndexTable")
			}
			secondaryIndexTablePresent = true
		}
	}

	t.initialized = true
	return nil
}

// initBuild initializes the table if it has an OID configured. If so, the
// net-snmp tools will be used to look up the OID and autopopulate the table's
// fields.
func (t *Table) initBuild(tr Translator) error {
	if t.Oid == "" {
		return nil
	}

	_, _, oidText, fields, err := tr.SnmpTable(t.Oid)
	if err != nil {
		return err
	}

	if t.Name == "" {
		t.Name = oidText
	}

	knownOIDs := map[string]bool{}
	for _, f := range t.Fields {
		knownOIDs[f.Oid] = true
	}
	for _, f := range fields {
		if !knownOIDs[f.Oid] {
			t.Fields = append(t.Fields, f)
		}
	}

	return nil
}

// Field holds the configuration for a Field to look up.
type Field struct {
	// Name will be the name of the field.
	Name string `toml:"name"`
	// OID is prefix for this field. The plugin will perform a walk through all
	// OIDs with this as their parent. For each value found, the plugin will strip
	// off the OID prefix, and use the remainder as the index. For multiple fields
	// to show up in the same row, they must share the same index.
	Oid string `toml:"oid"`
	// OidIndexSuffix is the trailing sub-identifier on a table record OID that will be stripped off to get the record's index.
	OidIndexSuffix string `toml:"oid_index_suffix"`
	// OidIndexLength specifies the length of the index in OID path segments. It can be used to remove sub-identifiers that vary in content or length.
	OidIndexLength int `toml:"oid_index_length"`
	// IsTag controls whether this OID is output as a tag or a value.
	IsTag bool `toml:"is_tag"`
	// Conversion controls any type conversion that is done on the value.
	//  "float"/"float(0)" will convert the value into a float.
	//  "float(X)" will convert the value into a float, and then move the decimal before Xth right-most digit.
	//  "int" will conver the value into an integer.
	//  "hwaddr" will convert a 6-byte string to a MAC address.
	//  "ipaddr" will convert the value to an IPv4 or IPv6 address.
	Conversion string `toml:"conversion"`
	// Translate tells if the value of the field should be snmptranslated
	Translate bool `toml:"translate"`
	// Secondary index table allows to merge data from two tables with different index
	//  that this filed will be used to join them. There can be only one secondary index table.
	SecondaryIndexTable bool `toml:"secondary_index_table"`
	// This field is using secondary index, and will be later merged with primary index
	//  using SecondaryIndexTable. SecondaryIndexTable and SecondaryIndexUse are exclusive.
	SecondaryIndexUse bool `toml:"secondary_index_use"`
	// Controls if entries from secondary table should be added or not if joining
	//  index is present or not. I set to true, means that join is outer, and
	//  index is prepended with "Secondary." for missing values to avoid overlaping
	//  indexes from both tables.
	// Can be set per field or globally with SecondaryIndexTable, global true overrides
	//  per field false.
	SecondaryOuterJoin bool `toml:"secondary_outer_join"`

	initialized bool `toml:"initialized"`
}

// init() converts OID names to numbers, and sets the .Name attribute if unset.
func (f *Field) init(tr Translator) error {
	if f.initialized {
		return nil
	}

	// check if oid needs translation or name is not set
	if strings.ContainsAny(f.Oid, ":abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") || f.Name == "" {
		_, oidNum, oidText, conversion, err := tr.SnmpTranslate(f.Oid)
		if err != nil {
			return fmt.Errorf("translating: %w", err)
		}
		f.Oid = oidNum
		if f.Name == "" {
			f.Name = oidText
		}
		if f.Conversion == "" {
			f.Conversion = conversion
		}
		// TODO use textual convention conversion from the MIB
	}

	if f.SecondaryIndexTable && f.SecondaryIndexUse {
		return fmt.Errorf("SecondaryIndexTable and UseSecondaryIndex are exclusive")
	}

	if !f.SecondaryIndexTable && !f.SecondaryIndexUse && f.SecondaryOuterJoin {
		return fmt.Errorf("SecondaryOuterJoin set to true, but field is not being used in join")
	}

	f.initialized = true
	return nil
}

// RTable is the resulting table built from a Table.
type RTable struct {
	// Name is the name of the field, copied from Table.Name.
	Name string `toml:"name"`
	// Time is the time the table was built.
	Time time.Time `toml:"time"`
	// Rows are the rows that were found, one row for each table OID index found.
	Rows []RTableRow `toml:"rows"`
}

// RTableRow is the resulting row containing all the OID values which shared
// the same index.
type RTableRow struct {
	// Tags are all the Field values which had IsTag=true.
	Tags map[string]string `toml:"tags"`
	// Fields are all the Field values which had IsTag=false.
	Fields map[string]interface{} `toml:"fields"`
}

type walkError struct {
	msg string
	err error
}

func (e *walkError) Error() string {
	return e.msg
}

func (e *walkError) Unwrap() error {
	return e.err
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Snmp{}
	})
}

func (s *Snmp) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}

// SampleConfig returns the default configuration of the input.
func (s *Snmp) SampleConfig() string {
	return sampleConfig
}

// Description returns a one-sentence description on the input.
func (s *Snmp) Description() string {
	return description
}

// Gather retrieves all the configured fields and tables.
// Any error encountered does not halt the process. The errors are accumulated
// and returned at the end.
// func (s *Instance) Gather(acc telegraf.Accumulator) error {
func (ins *Instance) Gather(slist *types.SampleList) {
	var wg sync.WaitGroup
	for i, agent := range ins.Agents {
		wg.Add(1)
		go func(i int, agent string) {
			defer wg.Done()
			gs, err := ins.getConnection(i)
			if err != nil {
				log.Printf("agent %s ins: %s", agent, err)
				return
			}

			// First is the top-level fields. We treat the fields as table prefixes with an empty index.
			t := Table{
				Name:   ins.Name,
				Fields: ins.Fields,
			}
			topTags := map[string]string{}
			if err := ins.gatherTable(slist, gs, t, topTags, false); err != nil {
				log.Printf("agent %s ins: %s", agent, err)
			}

			// Now is the real tables.
			for _, t := range ins.Tables {
				if err := ins.gatherTable(slist, gs, t, topTags, true); err != nil {
					log.Printf("agent %s ins: gathering table %s error: %s", agent, t.Name, err)
				}
			}
		}(i, agent)
	}
	wg.Wait()
}

func (ins *Instance) gatherTable(slist *types.SampleList, gs snmpConnection, t Table, topTags map[string]string, walk bool) error {
	rt, err := t.Build(gs, walk, ins.translator)
	if err != nil {
		return err
	}

	prefix := inputName
	if len(rt.Name) != 0 {
		prefix = inputName + "_" + rt.Name
	}
	for _, tr := range rt.Rows {
		if !walk {
			// top-level table. Add tags to topTags.
			for k, v := range tr.Tags {
				topTags[k] = v
			}
		} else {
			// real table. Inherit any specified tags.
			for _, k := range t.InheritTags {
				if v, ok := topTags[k]; ok {
					tr.Tags[k] = v
				}
			}
		}
		if _, ok := tr.Tags[ins.AgentHostTag]; !ok {
			tr.Tags[ins.AgentHostTag] = gs.Host()
		}
		slist.PushSamples(prefix, tr.Fields, tr.Tags)
	}

	return nil
}

// Build retrieves all the fields specified in the table and constructs the RTable.
func (t Table) Build(gs snmpConnection, walk bool, tr Translator) (*RTable, error) {
	rows := map[string]RTableRow{}

	// translation table for secondary index (when preforming join on two tables)
	secIdxTab := make(map[string]string)
	secGlobalOuterJoin := false
	for i, f := range t.Fields {
		if f.SecondaryIndexTable {
			secGlobalOuterJoin = f.SecondaryOuterJoin
			if i != 0 {
				t.Fields[0], t.Fields[i] = t.Fields[i], t.Fields[0]
			}
			break
		}
	}

	tagCount := 0
	for _, f := range t.Fields {
		if f.IsTag {
			tagCount++
		}

		if len(f.Oid) == 0 {
			return nil, fmt.Errorf("cannot have empty OID on field %s", f.Name)
		}
		var oid string
		if f.Oid[0] == '.' {
			oid = f.Oid
		} else {
			// make sure OID has "." because the BulkWalkAll results do, and the prefix needs to match
			oid = "." + f.Oid
		}

		// ifv contains a mapping of table OID index to field value
		ifv := map[string]interface{}{}

		if !walk {
			// This is used when fetching non-table fields. Fields configured the top
			// scope of the plugin.
			// We fetch the fields directly, and add them to ifv as if the index were an
			// empty string. This results in all the non-table fields sharing the same
			// index, and being added on the same row.
			if pkt, err := gs.Get([]string{oid}); err != nil {
				if errors.Is(err, gosnmp.ErrUnknownSecurityLevel) {
					return nil, fmt.Errorf("unknown security level (sec_level)")
				} else if errors.Is(err, gosnmp.ErrUnknownUsername) {
					return nil, fmt.Errorf("unknown username (sec_name)")
				} else if errors.Is(err, gosnmp.ErrWrongDigest) {
					return nil, fmt.Errorf("wrong digest (auth_protocol, auth_password)")
				} else if errors.Is(err, gosnmp.ErrDecryption) {
					return nil, fmt.Errorf("decryption error (priv_protocol, priv_password)")
				} else {
					return nil, fmt.Errorf("performing get on field %s: %w", f.Name, err)
				}
			} else if pkt != nil && len(pkt.Variables) > 0 && pkt.Variables[0].Type != gosnmp.NoSuchObject && pkt.Variables[0].Type != gosnmp.NoSuchInstance {
				ent := pkt.Variables[0]
				fv, err := fieldConvert(f.Conversion, ent.Value)
				if err != nil {
					return nil, fmt.Errorf("converting %q (OID %s) for field %s: %w", ent.Value, ent.Name, f.Name, err)
				}
				ifv[""] = fv
			}
		} else {
			err := gs.Walk(oid, func(ent gosnmp.SnmpPDU) error {
				if len(ent.Name) <= len(oid) || ent.Name[:len(oid)+1] != oid+"." {
					return &walkError{} // break the walk
				}

				idx := ent.Name[len(oid):]
				if f.OidIndexSuffix != "" {
					if !strings.HasSuffix(idx, f.OidIndexSuffix) {
						// this entry doesn't match our OidIndexSuffix. skip it
						return nil
					}
					idx = idx[:len(idx)-len(f.OidIndexSuffix)]
				}
				if f.OidIndexLength != 0 {
					i := f.OidIndexLength + 1 // leading separator
					idx = strings.Map(func(r rune) rune {
						if r == '.' {
							i--
						}
						if i < 1 {
							return -1
						}
						return r
					}, idx)
				}

				// snmptranslate table field value here
				if f.Translate {
					if entOid, ok := ent.Value.(string); ok {
						_, _, oidText, _, err := tr.SnmpTranslate(entOid)
						if err == nil {
							// If no error translating, the original value for ent.Value should be replaced
							ent.Value = oidText
						}
					}
				}

				fv, err := fieldConvert(f.Conversion, ent.Value)
				if err != nil {
					return &walkError{
						msg: fmt.Sprintf("converting %q (OID %s) for field %s", ent.Value, ent.Name, f.Name),
						err: err,
					}
				}
				ifv[idx] = fv
				return nil
			})
			if err != nil {
				// Our callback always wraps errors in a walkError.
				// If this error isn't a walkError, we know it's not
				// from the callback
				if _, ok := err.(*walkError); !ok {
					return nil, fmt.Errorf("performing bulk walk for field %s: %w", f.Name, err)
				}
			}
		}

		for idx, v := range ifv {
			if f.SecondaryIndexUse {
				if newidx, ok := secIdxTab[idx]; ok {
					idx = newidx
				} else {
					if !secGlobalOuterJoin && !f.SecondaryOuterJoin {
						continue
					}
					idx = ".Secondary" + idx
				}
			}
			rtr, ok := rows[idx]
			if !ok {
				rtr = RTableRow{}
				rtr.Tags = map[string]string{}
				rtr.Fields = map[string]interface{}{}
				rows[idx] = rtr
			}
			if t.IndexAsTag && idx != "" {
				if idx[0] == '.' {
					idx = idx[1:]
				}
				rtr.Tags["index"] = idx
			}
			// don't add an empty string
			if vs, ok := v.(string); !ok || vs != "" {
				if f.IsTag {
					if ok {
						rtr.Tags[f.Name] = vs
					} else {
						rtr.Tags[f.Name] = fmt.Sprintf("%v", v)
					}
				} else {
					rtr.Fields[f.Name] = v
				}
				if f.SecondaryIndexTable {
					// indexes are stored here with prepending "." so we need to add them if needed
					var vss string
					if ok {
						vss = "." + vs
					} else {
						vss = fmt.Sprintf(".%v", v)
					}
					if idx[0] == '.' {
						secIdxTab[vss] = idx
					} else {
						secIdxTab[vss] = "." + idx
					}
				}
			}
		}
	}

	rt := RTable{
		Name: t.Name,
		Time: time.Now(), // TODO record time at start
		Rows: make([]RTableRow, 0, len(rows)),
	}
	for _, r := range rows {
		rt.Rows = append(rt.Rows, r)
	}
	return &rt, nil
}

// snmpConnection is an interface which wraps a *gosnmp.GoSNMP object.
// We interact through an interface, so we can mock it out in tests.
type snmpConnection interface {
	Host() string

	// BulkWalkAll(string) ([]gosnmp.SnmpPDU, error)

	Walk(string, gosnmp.WalkFunc) error
	Get(oids []string) (*gosnmp.SnmpPacket, error)
}

// getConnection creates a snmpConnection (*gosnmp.GoSNMP) object and caches the
// result using `agentIndex` as the cache key.  This is done to allow multiple
// connections to a single address.  It is an error to use a connection in
// more than one goroutine.
func (ins *Instance) getConnection(idx int) (snmpConnection, error) {
	if gs := ins.connectionCache[idx]; gs != nil {
		return gs, nil
	}

	agent := ins.Agents[idx]

	var err error
	var gs GosnmpWrapper
	gs, err = NewWrapper(ins.ClientConfig)
	if err != nil {
		return nil, err
	}

	err = gs.SetAgent(agent)
	if err != nil {
		return nil, err
	}

	ins.connectionCache[idx] = gs

	if err := gs.Connect(); err != nil {
		return nil, fmt.Errorf("setting up connection: %w", err)
	}

	return gs, nil
}

func fieldNonStandardConvertInt64(v string) int64 {
	lowerV := strings.ToLower(v)

	if strings.HasSuffix(lowerV, "g") || strings.HasSuffix(lowerV, "gb") {
		v64, _ := strconv.ParseInt(lowerV[0:strings.LastIndex(lowerV, "g")], 10, 64)
		v64 = v64 * 1024 * 1024 * 1024
		return v64
	} else if strings.HasSuffix(lowerV, "t") || strings.HasSuffix(lowerV, "tb") {
		v64, _ := strconv.ParseInt(lowerV[0:strings.LastIndex(lowerV, "t")], 10, 64)
		v64 = v64 * 1024 * 1024 * 1024 * 1024
		return v64
	} else if strings.HasSuffix(lowerV, "m") || strings.HasSuffix(lowerV, "mb") {
		v64, _ := strconv.ParseInt(lowerV[0:strings.LastIndex(lowerV, "m")], 10, 64)
		v64 = v64 * 1024 * 1024
		return v64
	} else if strings.HasSuffix(lowerV, "k") || strings.HasSuffix(lowerV, "kb") {
		v64, _ := strconv.ParseInt(lowerV[0:strings.LastIndex(lowerV, "m")], 10, 64)
		v64 = v64 * 1024
		return v64
	} else {
		v64, _ := strconv.ParseInt(lowerV, 10, 64)
		return v64
	}
}

// fieldConvert converts from any type according to the conv specification
func fieldConvert(conv string, v interface{}) (interface{}, error) {
	if conv == "" {
		if bs, ok := v.([]byte); ok {
			return string(bs), nil
		}
		return v, nil
	}

	var d int
	if _, err := fmt.Sscanf(conv, "float(%d)", &d); err == nil || conv == "float" {
		switch vt := v.(type) {
		case float32:
			v = float64(vt) / math.Pow10(d)
		case float64:
			v = vt / math.Pow10(d)
		case int:
			v = float64(vt) / math.Pow10(d)
		case int8:
			v = float64(vt) / math.Pow10(d)
		case int16:
			v = float64(vt) / math.Pow10(d)
		case int32:
			v = float64(vt) / math.Pow10(d)
		case int64:
			v = float64(vt) / math.Pow10(d)
		case uint:
			v = float64(vt) / math.Pow10(d)
		case uint8:
			v = float64(vt) / math.Pow10(d)
		case uint16:
			v = float64(vt) / math.Pow10(d)
		case uint32:
			v = float64(vt) / math.Pow10(d)
		case uint64:
			v = float64(vt) / math.Pow10(d)
		case []byte:
			vf, _ := strconv.ParseFloat(string(vt), 64)
			v = vf / math.Pow10(d)
		case string:
			vf, _ := strconv.ParseFloat(vt, 64)
			v = vf / math.Pow10(d)
		}
		return v, nil
	}

	if conv == "int" {
		switch vt := v.(type) {
		case float32:
			v = int64(vt)
		case float64:
			v = int64(vt)
		case int:
			v = int64(vt)
		case int8:
			v = int64(vt)
		case int16:
			v = int64(vt)
		case int32:
			v = int64(vt)
		case int64:
			v = vt
		case uint:
			v = int64(vt)
		case uint8:
			v = int64(vt)
		case uint16:
			v = int64(vt)
		case uint32:
			v = int64(vt)
		case uint64:
			v = int64(vt)
		case []byte:
			v = fieldNonStandardConvertInt64(string(vt))
		case string:
			v = fieldNonStandardConvertInt64(vt)
		}
		return v, nil
	}

	if conv == "hwaddr" {
		switch vt := v.(type) {
		case string:
			v = net.HardwareAddr(vt).String()
		case []byte:
			v = net.HardwareAddr(vt).String()
		default:
			return nil, fmt.Errorf("invalid type (%T) for hwaddr conversion", v)
		}
		return v, nil
	}

	split := strings.Split(conv, ":")
	if split[0] == "hextoint" && len(split) == 3 {
		endian := split[1]
		bit := split[2]

		bv, ok := v.([]byte)
		if !ok {
			return v, nil
		}

		switch endian {
		case "LittleEndian":
			switch bit {
			case "uint64":
				v = binary.LittleEndian.Uint64(bv)
			case "uint32":
				v = binary.LittleEndian.Uint32(bv)
			case "uint16":
				v = binary.LittleEndian.Uint16(bv)
			default:
				return nil, fmt.Errorf("invalid bit value (%s) for hex to int conversion", bit)
			}
		case "BigEndian":
			switch bit {
			case "uint64":
				v = binary.BigEndian.Uint64(bv)
			case "uint32":
				v = binary.BigEndian.Uint32(bv)
			case "uint16":
				v = binary.BigEndian.Uint16(bv)
			default:
				return nil, fmt.Errorf("invalid bit value (%s) for hex to int conversion", bit)
			}
		default:
			return nil, fmt.Errorf("invalid Endian value (%s) for hex to int conversion", endian)
		}

		return v, nil
	}

	if conv == "ipaddr" {
		var ipbs []byte

		switch vt := v.(type) {
		case string:
			ipbs = []byte(vt)
		case []byte:
			ipbs = vt
		default:
			return nil, fmt.Errorf("invalid type (%T) for ipaddr conversion", v)
		}

		switch len(ipbs) {
		case 4, 16:
			v = net.IP(ipbs).String()
		default:
			return nil, fmt.Errorf("invalid length (%d) for ipaddr conversion", len(ipbs))
		}

		return v, nil
	}

	return nil, fmt.Errorf("invalid conversion type '%s'", conv)
}
