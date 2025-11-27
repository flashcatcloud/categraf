package snmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Knetic/govaluate"
	"github.com/gosnmp/gosnmp"
)

const (
	commonFormat = 2
	fullFormat   = 3

	defaultExprPrefix = "expr"
)

const (
	StrictMode = "strict"
)

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

	IncludeFilter []string `toml:"include_filter"`

	Filters          []string `toml:"filters"`
	FilterExpression string   `toml:"filters_expression"`
	FilterMode       string   `toml:"filters_mode"`

	filterFormat int                `toml:"-"`
	filtersMap   map[string]*Filter `toml:"-"`

	DebugMode bool
}

type Filter struct {
	key string
	re  *regexp.Regexp
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
	if len(t.IncludeFilter) != 0 {
		log.Println("W! include_filter is deprecated, please use filters instead")
		t.Filters = append(t.Filters, t.IncludeFilter...)
	}

	if len(t.Filters) != 0 {
		t.filtersMap = make(map[string]*Filter)
		filterExpression := ""
		for idx, filter := range t.Filters {
			fields := strings.Split(filter, ":")
			if t.filterFormat == 0 {
				t.filterFormat = len(fields)
			}
			if t.filterFormat != len(fields) {
				return fmt.Errorf("invalid filter format: %s, format must be {A}:{oid}:{match} or {oid}:{matrch}", filter)
			}
			switch t.filterFormat {
			case commonFormat:
				exprKey := fmt.Sprintf("%s%d", defaultExprPrefix, idx)
				t.filtersMap[exprKey] = &Filter{
					key: fields[0],
					re:  regexp.MustCompile(fields[1]),
				}
				if t.FilterExpression == "" {
					if filterExpression == "" {
						filterExpression = exprKey
					} else {
						filterExpression = fmt.Sprintf("%s||%s", filterExpression, exprKey)
					}
				}

			case fullFormat:
				t.filtersMap[fields[0]] = &Filter{
					key: fields[1],
					re:  regexp.MustCompile(fields[2]),
				}

				if t.FilterExpression == "" {
					return fmt.Errorf("filters_expression cannot be empty when filters are defined as {A}:{oid}:{match}")
				}
			default:
				return fmt.Errorf("invalid filter format: %s, format must be {A}:{oid}:{match} or {oid}:{matrch}", filter)
			}
		}
		if t.FilterExpression == "" {
			t.FilterExpression = filterExpression
		}
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
					return nil, fmt.Errorf("performing get on field %s(%s): %w", f.Name, oid, err)
				}
			} else if pkt != nil && len(pkt.Variables) > 0 &&
				pkt.Variables[0].Type != gosnmp.NoSuchObject &&
				pkt.Variables[0].Type != gosnmp.NoSuchInstance {
				ent := pkt.Variables[0]
				fv, err := fieldConvert(tr, f.Conversion, ent)
				if err != nil {
					return nil, fmt.Errorf("converting %q (OID %s) for field %s: %w", ent.Value, ent.Name, f.Name, err)
				}
				ifv[""] = fv
			} else {
				log.Println("W! no info for oid:", oid, "target:", gs.Host())
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
						} else {
							log.Printf("E! translate error:%s, entOid:%s, oid:%s", err, entOid, oid)
						}
					}
				}

				fv, err := fieldConvert(tr, f.Conversion, ent)
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
				var walkErr *walkError
				if !errors.As(err, &walkErr) {
					log.Printf("E! snmp walk error:%s, oid:%s ", err, oid)
					return nil, fmt.Errorf("performing bulk walk for field %s(%s): %w", f.Name, oid, err)
				} else {
					log.Printf("W! snmp walk error:%s(%s), oid:%s", err, walkErr.Unwrap(), oid)
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

	var (
		err  error
		expr *govaluate.EvaluableExpression
	)
	if len(t.FilterExpression) != 0 {
		expr, err = govaluate.NewEvaluableExpression(t.FilterExpression)
		if err != nil {
			log.Println("filters_expression err:", err)
		}
	}
	strictMode := t.FilterMode == StrictMode
	for _, r := range rows {
		if expr == nil {
			rt.Rows = append(rt.Rows, r)
			continue
		}
		params := make(map[string]interface{})
		for rk, rv := range t.filtersMap {
			if strictMode {
				params[rk] = false
			}
			for k, v := range r.Tags {
				if strings.HasPrefix(k, rv.key) {
					if rv.re.MatchString(v) {
						params[rk] = true
					} else {
						params[rk] = false
					}
				}
			}

			for k, v := range r.Fields {
				if strings.HasPrefix(k, rv.key) {
					if rv.re.MatchString(fmt.Sprintf("%v", v)) {
						params[rk] = true
					} else {
						params[rk] = false
					}
				}
			}
		}
		if len(params) != 0 {
			result, err := expr.Evaluate(params)
			if err != nil {
				log.Println("filter expression err:", err)
			}
			if match, ok := result.(bool); ok && !match {
				continue
			}
		}
		rt.Rows = append(rt.Rows, r)
	}
	return &rt, nil
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
func fieldConvert(tr Translator, conv string, ent gosnmp.SnmpPDU) (v interface{}, err error) {
	if conv == "" {
		if bs, ok := ent.Value.([]byte); ok {
			return string(bs), nil
		}
		return ent.Value, nil
	}

	var d int
	if _, err := fmt.Sscanf(conv, "float(%d)", &d); err == nil || conv == "float" {
		floatConv := func(vt string) float64 {
			var ret float64
			floatVal, err := heuristicDataExtract(vt)
			if err != nil {
				log.Printf("E! failed to extract float from string: %s, error: %v", vt, err)
				vf, _ := strconv.ParseFloat(vt, 64)
				ret = vf / math.Pow10(d)
			} else {
				ret = floatVal / math.Pow10(d)
			}
			return ret
		}

		v = ent.Value
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
			v = floatConv(string(vt))
		case string:
			v = floatConv(vt)
		}
		return v, nil
	}
	if conv == "byte" {
		if val, ok := ent.Value.([]byte); ok {
			return byteConvert(string(val))
		}
		if val, ok := ent.Value.(string); ok {
			return byteConvert(val)
		}
		return nil, fmt.Errorf("invalid type (%T) for byte conversion", ent.Value)
	}

	if conv == "int" {
		v = ent.Value
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
		switch vt := ent.Value.(type) {
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

		bv, ok := ent.Value.([]byte)
		if !ok {
			return ent.Value, nil
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

		switch vt := ent.Value.(type) {
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

	// 55 57 57 51 46 56 32 77 66   may be the ascii arr for string ->  7993.8 MB
	if conv == "asciitobytes" {
		v = ent.Value
		input, ok := v.([]uint8)
		if !ok {
			return nil, fmt.Errorf("invalid type of %v (not rune arr)", v)
		}

		// 将ascii字符切片转换为字符串
		asciiStr := string(input)
		return byteConvert(asciiStr)
	}
	if conv == "enum" {
		return tr.SnmpFormatEnum(ent.Name, ent.Value, false)
	}

	if conv == "enum(1)" {
		return tr.SnmpFormatEnum(ent.Name, ent.Value, true)
	}

	if conv == "percent" {
		v = ent.Value
		switch vt := v.(type) {
		case float32:
			return float64(vt), nil
		case float64:
			return vt, nil
		case int:
			return float64(vt), nil
		case int8:
			return float64(vt), nil
		case int16:
			return float64(vt), nil
		case int32:
			return float64(vt), nil
		case int64:
			return float64(vt), nil
		case uint:
			return float64(vt), nil
		case uint8:
			return float64(vt), nil
		case uint16:
			return float64(vt), nil
		case uint32:
			return float64(vt), nil
		case uint64:
			return float64(vt), nil
		case []byte:
			return parsePercentString(string(vt))
		case string:
			return parsePercentString(vt)
		default:
			return nil, fmt.Errorf("invalid type (%T) for percent conversion", v)
		}
	}

	return nil, fmt.Errorf("invalid conversion type '%s'", conv)
}

func parsePercentString(str string) (interface{}, error) {
	// 处理空字符串或N/A
	if na := strings.TrimSpace(str); na == "N/A" || na == "" {
		return 0, nil
	}

	// 移除两端空格
	str = strings.TrimSpace(str)

	// 只匹配百分比格式（如"36%"、"36.2%"、"36 %"等）
	// 匹配数字和小数点，后面跟着可选的空格和百分号
	percentRe := regexp.MustCompile(`([0-9]+\.?[0-9]*)\s*%`)
	percentMatches := percentRe.FindStringSubmatch(str)

	if len(percentMatches) >= 2 {
		// 找到了百分比格式，直接提取数字部分
		value, err := strconv.ParseFloat(percentMatches[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid percent string: %s", str)
		}
		return value, nil
	}

	// 如果没有找到百分比格式，返回错误
	return nil, fmt.Errorf("not a percent string: %s", str)
}

func byteConvert(str string) (interface{}, error) {
	if na := strings.TrimSpace(str); na == "N/A" || na == "" {
		return 0, nil
	}
	var numericStr string
	for _, char := range str {
		if char >= '0' && char <= '9' || char == '.' {
			numericStr += string(char)
		}
	}

	// 将字符串转换为浮点数
	value, err := strconv.ParseFloat(numericStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number part of %s", str)
	}

	// 解析单位并转换为字节数
	unit := strings.ToUpper(strings.TrimSpace(strings.Trim(str, numericStr)))
	var result float64
	switch unit {
	case "", "B":
		result = value
	case "KB":
		result = value * 1000
	case "MB":
		result = value * 1000 * 1000
	case "GB":
		result = value * 1000 * 1000 * 1000
	case "TB":
		result = value * 1000 * 1000 * 1000 * 1000
	case "PB":
		result = value * 1000 * 1000 * 1000 * 1000 * 1000
	case "KIB":
		result = value * 1024
	case "MIB":
		result = value * 1024 * 1024
	case "GIB":
		result = value * 1024 * 1024 * 1024
	case "TIB":
		result = value * 1024 * 1024 * 1024 * 1024
	case "PIB":
		result = value * 1024 * 1024 * 1024 * 1024 * 1024
	default:
		return nil, fmt.Errorf("invalid unit of %s", unit)
	}
	return result, nil
}

// heuristicsDataExtract attempts to extract the first floating-point number from the input string.
// It scans the string for a valid float (optionally with a decimal point and exponent) and returns its value.
// Returns an error if no valid number is found or if parsing fails.
//
// Example input strings this function can handle:
//
//	"Temperature: 23.5C"      -> 23.5
//	"42.3 units"              -> 42.3
//	"Value is -12.7e3 volts"  -> -12700
//	"N/A"                     -> error
func heuristicDataExtract(s string) (float64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty string, cannot extract float value")
	}

	var (
		start    = -1
		end      = -1
		hasDot   = false
		hasExp   = false
		hasDigit = false
		i        = 0
	)

	for i < len(s) {
		c := s[i]

		if c > 127 {
			if start != -1 {
				end = i
				break
			}
			i++
			continue
		}

		if start == -1 {
			if c >= '0' && c <= '9' {
				start = i
				hasDigit = true
			} else if c == '-' || c == '+' {
				if i+1 < len(s) && ((s[i+1] >= '0' && s[i+1] <= '9') || s[i+1] == '.') {
					if c == '-' && i > 0 {
						prev := s[i-1]
						if prev != ' ' && prev != '\t' && prev != ':' && prev != '=' && prev != '(' && prev != ',' {
							i++
							continue
						}
					}
					start = i
				}
			} else if c == '.' {
				if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
					start = i
					hasDot = true
				}
			}
		} else {
			if c >= '0' && c <= '9' {
				hasDigit = true
			} else if c == '.' && !hasDot && !hasExp {
				hasDot = true
			} else if (c == 'e' || c == 'E') && !hasExp && hasDigit {
				hasExp = true
				if i+1 < len(s) && (s[i+1] == '+' || s[i+1] == '-') {
					i++
				}
			} else if c == '+' || c == '-' {
				if i > 0 && (s[i-1] == 'e' || s[i-1] == 'E') {
					// Allow '+' or '-' immediately after 'e' or 'E' in exponent part of float
				} else {
					end = i
					break
				}
			} else {
				end = i
				break
			}
		}
		i++
	}

	if start != -1 && end == -1 {
		end = len(s)
	}

	if start == -1 || !hasDigit {
		return 0, fmt.Errorf("no valid number found in string: %s", s)
	}

	numStr := s[start:end]
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse number '%s' from string '%s': %w", numStr, s, err)
	}

	return val, nil
}
