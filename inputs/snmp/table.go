package snmp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Knetic/govaluate"
	"github.com/gosnmp/gosnmp"

	snmppkg "flashcat.cloud/categraf/pkg/snmp"
)

const (
	commonFormat = 2
	fullFormat   = 3

	defaultExprPrefix = "expr"
)

const (
	StrictMode = "strict"
)

const (
	ErrorPolicyLegacy  = "legacy"
	ErrorPolicyPartial = "partial"
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
	ErrorPolicy  string             `toml:"error_policy"`

	dependencyPlan tableDependencyPlan `toml:"-"`

	DebugMode bool
}

type Filter struct {
	key       string
	fieldName string
	re        *regexp.Regexp
}

type tableDependencyPlan struct {
	Planned                bool
	TagFields              []string
	FilterFields           map[string]bool
	SecondaryProviderIndex int
	SecondaryConsumers     map[string]bool
	InheritTags            []string
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
			const escapeMarker = "##COLON##"
			processedFilter := strings.Replace(filter, "\\:", escapeMarker, -1)

			fields := strings.Split(processedFilter, ":")
			if t.filterFormat == 0 {
				t.filterFormat = len(fields)
			}
			if t.filterFormat != len(fields) {
				return fmt.Errorf("invalid filter format: %s, format must be {A}:{oid}:{match} or {oid}:{matrch}", filter)
			}
			switch t.filterFormat {
			case commonFormat:
				fields[1] = strings.Replace(fields[1], escapeMarker, ":", -1)
				re, err := regexp.Compile(fields[1])
				if err != nil {
					return fmt.Errorf("filters %q regexp compile error: %w", filter, err)
				}
				exprKey := fmt.Sprintf("%s%d", defaultExprPrefix, idx)
				t.filtersMap[exprKey] = &Filter{
					key: fields[0],
					re:  re,
				}
				if t.FilterExpression == "" {
					if filterExpression == "" {
						filterExpression = exprKey
					} else {
						filterExpression = fmt.Sprintf("%s||%s", filterExpression, exprKey)
					}
				}

			case fullFormat:
				fields[2] = strings.Replace(fields[2], escapeMarker, ":", -1)
				re, err := regexp.Compile(fields[2])
				if err != nil {
					return fmt.Errorf("filters %q regexp compile error: %w", filter, err)
				}
				t.filtersMap[fields[0]] = &Filter{
					key: fields[1],
					re:  re,
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
	if err := t.bindFilters(); err != nil {
		return err
	}
	t.dependencyPlan = t.buildDependencyPlan()

	t.initialized = true
	return nil
}

func (t Table) buildDependencyPlan() tableDependencyPlan {
	plan := tableDependencyPlan{
		Planned:                true,
		FilterFields:           map[string]bool{},
		SecondaryProviderIndex: -1,
		SecondaryConsumers:     map[string]bool{},
		InheritTags:            append([]string(nil), t.InheritTags...),
	}
	for _, f := range t.Fields {
		if f.IsTag {
			plan.TagFields = append(plan.TagFields, f.Name)
		}
		if t.isFilterDependency(f) {
			plan.FilterFields[f.Name] = true
		}
		if f.SecondaryIndexTable {
			for i := range t.Fields {
				if t.Fields[i].Name == f.Name {
					plan.SecondaryProviderIndex = i
					break
				}
			}
		}
		if f.SecondaryIndexUse {
			plan.SecondaryConsumers[f.Name] = true
		}
	}
	return plan
}

func (t Table) dependencyPlanForBuild() tableDependencyPlan {
	if t.dependencyPlan.Planned {
		return t.dependencyPlan
	}
	return t.buildDependencyPlan()
}

func (t *Table) bindFilters() error {
	if len(t.filtersMap) == 0 {
		return nil
	}

	for name, filter := range t.filtersMap {
		if filter == nil {
			return fmt.Errorf("filter %s is nil", name)
		}
		fieldName, err := t.bindFilterField(filter.key)
		if err != nil {
			return fmt.Errorf("filter %s key %q: %w", name, filter.key, err)
		}
		filter.fieldName = fieldName
	}

	if t.FilterExpression != "" {
		expr, err := govaluate.NewEvaluableExpression(t.FilterExpression)
		if err != nil {
			return fmt.Errorf("filters_expression error: %w", err)
		}
		for _, v := range expr.Vars() {
			if _, ok := t.filtersMap[v]; !ok {
				return fmt.Errorf("filters_expression references undefined variable %q", v)
			}
		}
	}
	return nil
}

func (t *Table) bindFilterField(key string) (string, error) {
	for _, f := range t.Fields {
		if f.Name == key {
			return f.Name, nil
		}
	}

	matches := make([]string, 0, 1)
	for _, f := range t.Fields {
		if strings.HasPrefix(f.Name, key) {
			matches = append(matches, f.Name)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no matching field")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous prefix match %v", matches)
	}
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
	// ConvertRules provides ordered mapping/extraction rules prior to type conversion.
	ConvertRules []ConvertRule `toml:"convert_rule"`
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
	if err := snmppkg.InitConvertRules(f.ConvertRules); err != nil {
		return err
	}

	f.initialized = true
	return nil
}

type ConvertRule = snmppkg.ConvertRule

type convertRuleError struct {
	err error
}

func (e *convertRuleError) Error() string {
	return e.err.Error()
}

func (e *convertRuleError) Unwrap() error {
	return e.err
}

func (f *Field) convertValue(tr Translator, ent gosnmp.SnmpPDU) (interface{}, error) {
	if len(f.ConvertRules) == 0 {
		return fieldConvert(tr, f.Conversion, ent)
	}

	result := snmppkg.MatchConvertRule(f.ConvertRules, ent.Value, f.Conversion)
	if !result.Matched {
		return fieldConvert(tr, f.Conversion, ent)
	}
	if result.FixedValue {
		return result.Value, nil
	}
	if result.Conversion == "" {
		return result.Value, nil
	}

	ent.Value = result.Value
	converted, err := fieldConvertStrict(tr, result.Conversion, ent)
	if err != nil {
		return nil, &convertRuleError{err: fmt.Errorf("convert_rule failed to convert value %q to %s: %w", result.Value, result.Conversion, err)}
	}
	return converted, nil
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

type BuildResult struct {
	Table        *RTable
	Dependencies BuildDependencies
	Stats        BuildStats
}

type BuildDependencies struct {
	TopLevelTags map[string]string
}

type DependencyState string

const (
	DependencyStateCurrent     DependencyState = "current"
	DependencyStateCached      DependencyState = "cached"
	DependencyStateKnownAbsent DependencyState = "known_absent"
	DependencyStateUnknown     DependencyState = "unknown"
)

type FieldError struct {
	FieldName  string
	OID        string
	Operation  string
	Reason     string
	Err        error
	Dependency bool
	Fatal      bool
}

type DependencyStatus struct {
	Type      string
	FieldName string
	State     DependencyState
}

type DependencyCacheEvent struct {
	Type   string
	Result string
}

type BuildStats struct {
	TableName        string
	SuccessfulFields int
	FailedFields     int
	FieldErrors      []FieldError
	DependencyStates []DependencyStatus
	CacheEvents      []DependencyCacheEvent
	SkippedRows      map[string]int
	Partial          bool
	FatalClass       string
}

func (s *BuildStats) recordFieldSuccess() {
	s.SuccessfulFields++
}

func (s *BuildStats) recordFieldError(field Field, oid, operation string, err error, dependency bool) {
	s.FailedFields++
	s.FieldErrors = append(s.FieldErrors, FieldError{
		FieldName:  field.Name,
		OID:        oid,
		Operation:  operation,
		Reason:     classifyFieldError(err),
		Err:        err,
		Dependency: dependency,
		Fatal:      isFatalSNMPError(err),
	})
}

func (s *BuildStats) recordFatalFieldError(field Field, oid, operation string, err error, dependency bool) {
	s.recordFieldError(field, oid, operation, err, dependency)
	if len(s.FieldErrors) > 0 {
		s.FieldErrors[len(s.FieldErrors)-1].Fatal = true
	}
	s.FatalClass = classifyFatalError(err)
}

func (s *BuildStats) recordDependency(field Field, depType string, state DependencyState) {
	s.DependencyStates = append(s.DependencyStates, DependencyStatus{
		Type:      depType,
		FieldName: field.Name,
		State:     state,
	})
}

func (s *BuildStats) recordCache(depType, result string) {
	if result == "" {
		return
	}
	s.CacheEvents = append(s.CacheEvents, DependencyCacheEvent{Type: depType, Result: result})
}

func (s *BuildStats) skip(reason string) {
	if s.SkippedRows == nil {
		s.SkippedRows = map[string]int{}
	}
	s.SkippedRows[reason]++
}

func (s BuildStats) isPartialResult() bool {
	if !s.Partial {
		return false
	}
	if s.FatalClass != "" {
		return false
	}
	if s.FailedFields > 0 || len(s.CacheEvents) > 0 {
		return true
	}
	for _, n := range s.SkippedRows {
		if n > 0 {
			return true
		}
	}
	return false
}

func classifyFieldError(err error) string {
	if err == nil {
		return ""
	}
	if isFatalSNMPError(err) {
		return "fatal"
	}
	var crErr *convertRuleError
	if errors.As(err, &crErr) {
		return "conversion"
	}
	if isPermanentSocketError(err) {
		return "permanent_socket"
	}
	if isTransportFailure(err) {
		return "transport"
	}
	return "error"
}

func classifyFatalError(err error) string {
	switch {
	case isFatalSNMPError(err):
		return "snmp_fatal"
	case isPermanentSocketError(err):
		return "permanent_socket"
	case isTransportFailure(err):
		return "transport"
	default:
		return "fatal"
	}
}

type walkError struct {
	msg string
	err error
}

func (e *walkError) Error() string {
	if e.err != nil {
		return e.msg + ": " + e.err.Error()
	}
	return e.msg
}

func (e *walkError) Unwrap() error {
	return e.err
}

func (t Table) isFilterDependency(f Field) bool {
	if len(t.filtersMap) == 0 {
		return false
	}
	for _, filter := range t.filtersMap {
		if filter == nil {
			continue
		}
		if f.Name == filter.fieldName {
			return true
		}
	}
	return false
}

func (t Table) hasAmbiguousFilterDependency() bool {
	if len(t.filtersMap) == 0 {
		return false
	}
	for _, filter := range t.filtersMap {
		if filter == nil || filter.fieldName == "" {
			return true
		}
	}
	return false
}

type rowDependencyState struct {
	hasCurrentOrdinaryValue bool
	identityState           DependencyState
	filterDecision          filterDecision
}

func rowIdentityState(row RTableRow, tagFieldNames []string, tagStates map[string]DependencyState) DependencyState {
	state := DependencyStateCurrent
	for _, name := range tagFieldNames {
		if _, ok := row.Tags[name]; ok {
			continue
		}
		switch tagStates[name] {
		case DependencyStateCurrent, DependencyStateCached, DependencyStateKnownAbsent:
			if state != DependencyStateUnknown {
				state = DependencyStateKnownAbsent
			}
		default:
			return DependencyStateUnknown
		}
	}
	return state
}

type filterDecision int

const (
	filterDecisionNone filterDecision = iota
	filterDecisionAllow
	filterDecisionDeny
	filterDecisionUnknown
)

func rowGate(partial bool, walk bool, state rowDependencyState) (bool, string) {
	if partial {
		switch state.identityState {
		case DependencyStateUnknown:
			return false, "identity_unknown"
		case DependencyStateKnownAbsent:
			return false, "identity_known_absent"
		}
	}
	if partial && walk && !state.hasCurrentOrdinaryValue {
		return false, "no_current_value"
	}
	switch state.filterDecision {
	case filterDecisionDeny:
		return false, "filter_deny"
	case filterDecisionUnknown:
		return false, "filter_unknown"
	default:
		return true, ""
	}
}

type fatalSNMPError struct {
	err error
}

func (e *fatalSNMPError) Error() string {
	return e.err.Error()
}

func (e *fatalSNMPError) Unwrap() error {
	return e.err
}

func newFatalSNMPError(err error) error {
	return &fatalSNMPError{err: err}
}

func isFatalSNMPError(err error) bool {
	if err == nil {
		return false
	}
	var fatalErr *fatalSNMPError
	if errors.As(err, &fatalErr) {
		return true
	}
	return errors.Is(err, gosnmp.ErrUnknownSecurityLevel) ||
		errors.Is(err, gosnmp.ErrUnknownUsername) ||
		errors.Is(err, gosnmp.ErrWrongDigest) ||
		errors.Is(err, gosnmp.ErrDecryption) ||
		errors.Is(err, gosnmp.ErrInvalidMsgs) ||
		errors.Is(err, gosnmp.ErrUnknownSecurityModels) ||
		errors.Is(err, gosnmp.ErrUnknownPDUHandlers) ||
		errors.Is(err, gosnmp.ErrUnknownReportPDU)
}

type tableBuildOptions struct {
	dependencyCache *dependencyCache
	tableID         string
	tableIndex      int
	topLevel        bool
	now             time.Time
}

func (t Table) cacheID(walk bool) string {
	return t.cacheIDWithIndex(walk, -1)
}

func (t Table) cacheIDWithIndex(walk bool, tableIndex int) string {
	hash := fnv.New64a()
	_, _ = fmt.Fprintf(hash, "name=%s|oid=%s|walk=%t", t.Name, t.Oid, walk)
	for _, f := range t.Fields {
		_, _ = fmt.Fprintf(hash, "|field=%s,%s,%s,%d,%t,%t,%t,%t,%t",
			f.Name, f.Oid, f.OidIndexSuffix, f.OidIndexLength,
			f.IsTag, f.SecondaryIndexTable, f.SecondaryIndexUse,
			f.SecondaryOuterJoin, t.isFilterDependency(f))
	}
	if !walk {
		return fmt.Sprintf("top:%016x", hash.Sum64())
	}
	if tableIndex >= 0 {
		return fmt.Sprintf("table:%d:%s:%016x", tableIndex, t.Name, hash.Sum64())
	}
	return fmt.Sprintf("table:%s:%016x", t.Name, hash.Sum64())
}

func dependencyTypeForField(opts tableBuildOptions, f Field, filterDependency bool) string {
	switch {
	case opts.topLevel && f.IsTag:
		return "inherit_tag"
	case f.IsTag:
		return "tag"
	case f.SecondaryIndexTable:
		return "secondary"
	case filterDependency:
		return "filter"
	default:
		return "value"
	}
}

func (t Table) applyCachedDependency(opts tableBuildOptions, f Field, filterDependency bool, ifv map[string]interface{}, cachedFilterValues map[string]map[string]interface{}, secIdxTab map[string]string, stats *BuildStats) bool {
	if opts.dependencyCache == nil {
		if stats != nil {
			stats.recordCache(dependencyTypeForField(opts, f, filterDependency), dependencyCacheResultDisabled)
		}
		return false
	}

	cacheHit := false
	depType := dependencyTypeForField(opts, f, filterDependency)
	if opts.topLevel && f.IsTag {
		v, ok, result := opts.dependencyCache.topTag(f.Name, opts.now)
		if stats != nil {
			stats.recordCache(depType, result)
		}
		if ok {
			ifv[""] = v
			cacheHit = true
		}
		return cacheHit
	}

	if f.IsTag {
		values, ok, result := opts.dependencyCache.tableField(opts.tableID, f.Name, opts.now)
		if stats != nil {
			stats.recordCache("tag", result)
		}
		if ok {
			for idx, v := range values {
				ifv[idx] = v
			}
			cacheHit = true
		}
	}
	if filterDependency && !f.IsTag {
		values, ok, result := opts.dependencyCache.tableField(opts.tableID, f.Name, opts.now)
		if stats != nil {
			stats.recordCache("filter", result)
		}
		if ok {
			addCachedFilterValues(cachedFilterValues, f.Name, values)
			cacheHit = true
		}
	}
	if f.SecondaryIndexTable {
		mapping, ok, result := opts.dependencyCache.secondaryMapping(opts.tableID, opts.now)
		if stats != nil {
			stats.recordCache("secondary", result)
		}
		if ok {
			replaceStringMap(secIdxTab, mapping)
			cacheHit = true
		}
	}
	return cacheHit
}

func (t Table) storeSuccessfulDependency(opts tableBuildOptions, f Field, filterDependency bool, ifv map[string]interface{}, secIdxTab map[string]string, secGlobalOuterJoin bool) {
	if opts.dependencyCache == nil {
		return
	}

	if opts.topLevel && f.IsTag {
		if v, ok := ifv[""]; ok {
			opts.dependencyCache.storeTopTag(f.Name, v, opts.now)
		} else {
			opts.dependencyCache.deleteTopTag(f.Name)
		}
		return
	}

	if f.IsTag || filterDependency {
		opts.dependencyCache.replaceTableField(opts.tableID, f.Name, cacheSnapshotValues(f, ifv, secIdxTab, secGlobalOuterJoin), opts.now)
	}
	if f.SecondaryIndexTable {
		opts.dependencyCache.replaceSecondary(opts.tableID, secIdxTab, opts.now)
	}
}

func cacheSnapshotValues(f Field, values map[string]interface{}, secIdxTab map[string]string, secGlobalOuterJoin bool) map[string]interface{} {
	if !f.SecondaryIndexUse || f.IsTag {
		return values
	}

	out := make(map[string]interface{}, len(values))
	for idx, value := range values {
		if newidx, ok := secIdxTab[idx]; ok {
			out[newidx] = value
			continue
		}
		if secGlobalOuterJoin || f.SecondaryOuterJoin {
			out[".Secondary"+idx] = value
		}
	}
	return out
}

func addCachedFilterValues(dst map[string]map[string]interface{}, fieldName string, values map[string]interface{}) {
	for idx, value := range values {
		if _, ok := dst[idx]; !ok {
			dst[idx] = map[string]interface{}{}
		}
		dst[idx][fieldName] = value
	}
}

func replaceStringMap(dst map[string]string, src map[string]string) {
	for k := range dst {
		delete(dst, k)
	}
	for k, v := range src {
		dst[k] = v
	}
}

func dependencyTagString(v interface{}) (string, bool) {
	if s, ok := v.(string); ok {
		return s, s != ""
	}
	return fmt.Sprintf("%v", v), true
}

// Build retrieves all the fields specified in the table and constructs the RTable.
func (t Table) Build(gs snmpConnection, walk bool, tr Translator) (*RTable, error) {
	return t.BuildWithCache(gs, walk, tr, tableBuildOptions{
		tableID:    t.cacheID(walk),
		tableIndex: -1,
		now:        time.Now(),
	})
}

func (t Table) BuildWithCache(gs snmpConnection, walk bool, tr Translator, opts tableBuildOptions) (*RTable, error) {
	result, err := t.BuildResultWithCache(gs, walk, tr, opts)
	if result == nil {
		return nil, err
	}
	return result.Table, err
}

func (t Table) BuildResultWithCache(gs snmpConnection, walk bool, tr Translator, opts tableBuildOptions) (*BuildResult, error) {
	if opts.now.IsZero() {
		opts.now = time.Now()
	}
	if opts.tableID == "" {
		opts.tableID = t.cacheIDWithIndex(walk, opts.tableIndex)
	}
	rows := map[string]RTableRow{}
	rowStates := map[string]*rowDependencyState{}
	cachedFilterValues := map[string]map[string]interface{}{}
	dependencies := BuildDependencies{
		TopLevelTags: map[string]string{},
	}
	partial := t.ErrorPolicy == ErrorPolicyPartial
	stats := BuildStats{
		TableName:   t.Name,
		Partial:     partial,
		SkippedRows: map[string]int{},
	}
	emptyResult := func() *BuildResult {
		return &BuildResult{
			Table: &RTable{
				Name: t.Name,
				Time: time.Now(),
				Rows: nil,
			},
			Dependencies: dependencies,
			Stats:        stats,
		}
	}
	secondaryMappingUnknown := false
	plan := t.dependencyPlanForBuild()
	tagFieldStates := map[string]DependencyState{}
	filterFieldStates := map[string]DependencyState{}
	rowStateFor := func(idx string) *rowDependencyState {
		state, ok := rowStates[idx]
		if !ok {
			state = &rowDependencyState{}
			rowStates[idx] = state
		}
		return state
	}

	fields := append([]Field(nil), t.Fields...)
	// translation table for secondary index (when preforming join on two tables)
	secIdxTab := make(map[string]string)
	secGlobalOuterJoin := false
	if plan.SecondaryProviderIndex >= 0 && plan.SecondaryProviderIndex < len(fields) {
		f := fields[plan.SecondaryProviderIndex]
		secGlobalOuterJoin = f.SecondaryOuterJoin
		if plan.SecondaryProviderIndex != 0 {
			fields[0], fields[plan.SecondaryProviderIndex] = fields[plan.SecondaryProviderIndex], fields[0]
		}
	}

	tagFieldNames := append([]string(nil), plan.TagFields...)
	for _, f := range fields {
		filterDependency := plan.FilterFields[f.Name]
		dependencyField := f.IsTag || f.SecondaryIndexTable || filterDependency
		if len(f.Oid) == 0 {
			err := fmt.Errorf("cannot have empty OID on field %s", f.Name)
			stats.recordFatalFieldError(f, "", "config", err, dependencyField)
			return emptyResult(), err
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
		fieldFetchSucceeded := false
		dependencyState := DependencyState("")

		if !walk {
			// This is used when fetching non-table fields. Fields configured the top
			// scope of the plugin.
			// We fetch the fields directly, and add them to ifv as if the index were an
			// empty string. This results in all the non-table fields sharing the same
			// index, and being added on the same row.
			if pkt, err := gs.Get([]string{oid}); err != nil {
				if isFatalSNMPError(err) {
					err = newFatalSNMPError(fmt.Errorf("performing get for field %s(%s): %w", f.Name, oid, err))
					stats.recordFatalFieldError(f, oid, "get", err, dependencyField)
					return emptyResult(), err
				} else if isPermanentSocketError(err) {
					err = fmt.Errorf("performing get on field %s(%s): %w", f.Name, oid, err)
					stats.recordFatalFieldError(f, oid, "get", err, dependencyField)
					return emptyResult(), err
				} else {
					if partial {
						stats.recordFieldError(f, oid, "get", err, dependencyField)
						if dependencyField {
							cacheHit := t.applyCachedDependency(opts, f, filterDependency, ifv, cachedFilterValues, secIdxTab, &stats)
							if cacheHit {
								dependencyState = DependencyStateCached
							} else {
								dependencyState = DependencyStateUnknown
							}
							if f.SecondaryIndexTable && !cacheHit {
								secondaryMappingUnknown = true
							}
						}
						log.Printf("W! snmp get field error:%s, oid:%s", err, oid)
						if len(ifv) == 0 {
							if dependencyField {
								stats.recordDependency(f, dependencyTypeForField(opts, f, filterDependency), dependencyState)
							}
							continue
						}
					} else {
						return emptyResult(), fmt.Errorf("performing get on field %s(%s): %w", f.Name, oid, err)
					}
				}
			} else if pkt != nil && len(pkt.Variables) > 0 &&
				pkt.Variables[0].Type != gosnmp.NoSuchObject &&
				pkt.Variables[0].Type != gosnmp.NoSuchInstance {
				ent := pkt.Variables[0]
				fv, err := f.convertValue(tr, ent)
				if err != nil {
					if partial {
						stats.recordFieldError(f, oid, "get", err, dependencyField)
						if dependencyField {
							cacheHit := t.applyCachedDependency(opts, f, filterDependency, ifv, cachedFilterValues, secIdxTab, &stats)
							if cacheHit {
								dependencyState = DependencyStateCached
							} else {
								dependencyState = DependencyStateUnknown
							}
							if f.SecondaryIndexTable && !cacheHit {
								secondaryMappingUnknown = true
							}
						}
						log.Printf("W! converting %q (OID %s) for field %s: %s", ent.Value, ent.Name, f.Name, err)
						if len(ifv) == 0 {
							if dependencyField {
								stats.recordDependency(f, dependencyTypeForField(opts, f, filterDependency), dependencyState)
							}
							continue
						}
					} else {
						return emptyResult(), fmt.Errorf("converting %q (OID %s) for field %s: %w", ent.Value, ent.Name, f.Name, err)
					}
				} else {
					ifv[""] = fv
					fieldFetchSucceeded = true
				}
			} else {
				log.Println("W! no info for oid:", oid, "target:", gs.Host())
				fieldFetchSucceeded = true
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

				fv, err := f.convertValue(tr, ent)
				if err != nil {
					return &walkError{
						msg: fmt.Sprintf("converting %q (OID %s) for field %s", ent.Value, ent.Name, f.Name),
						err: err,
					}
				}
				ifv[idx] = fv
				return nil
			})
			fieldFetchSucceeded = err == nil
			if err != nil {
				// Our callback always wraps errors in a walkError.
				var walkErr *walkError
				if !errors.As(err, &walkErr) {
					// Underlying SNMP walk error.
					log.Printf("E! snmp walk error:%s, oid:%s ", err, oid)
					if isFatalSNMPError(err) {
						err = newFatalSNMPError(fmt.Errorf("performing bulk walk for field %s(%s): %w", f.Name, oid, err))
						stats.recordFatalFieldError(f, oid, "walk", err, dependencyField)
						return emptyResult(), err
					}
					if isPermanentSocketError(err) {
						err = fmt.Errorf("performing bulk walk for field %s(%s): %w", f.Name, oid, err)
						stats.recordFatalFieldError(f, oid, "walk", err, dependencyField)
						return emptyResult(), err
					}
					if partial {
						stats.recordFieldError(f, oid, "walk", err, dependencyField)
						ifv = map[string]interface{}{}
						if dependencyField {
							cacheHit := t.applyCachedDependency(opts, f, filterDependency, ifv, cachedFilterValues, secIdxTab, &stats)
							if cacheHit {
								dependencyState = DependencyStateCached
							} else {
								dependencyState = DependencyStateUnknown
							}
							if f.SecondaryIndexTable && !cacheHit {
								secondaryMappingUnknown = true
							}
						}
						if len(ifv) == 0 {
							if dependencyField {
								stats.recordDependency(f, dependencyTypeForField(opts, f, filterDependency), dependencyState)
							}
							continue
						}
					} else {
						return emptyResult(), fmt.Errorf("performing bulk walk for field %s(%s): %w", f.Name, oid, err)
					}
				} else if walkErr.err == nil {
					// Empty sentinel used to break the walk normally.
					fieldFetchSucceeded = true
				} else {
					var ruleErr *convertRuleError
					if errors.As(walkErr.err, &ruleErr) {
						log.Printf("E! snmp walk error:%s, oid:%s ", err, oid)
						if !partial {
							return emptyResult(), fmt.Errorf("performing bulk walk for field %s(%s): %w", f.Name, oid, err)
						}
					} else {
						log.Printf("W! snmp walk error:%s, oid:%s", err, oid)
					}
					if partial {
						stats.recordFieldError(f, oid, "walk", err, dependencyField)
						ifv = map[string]interface{}{}
						if dependencyField {
							cacheHit := t.applyCachedDependency(opts, f, filterDependency, ifv, cachedFilterValues, secIdxTab, &stats)
							if cacheHit {
								dependencyState = DependencyStateCached
							} else {
								dependencyState = DependencyStateUnknown
							}
							if f.SecondaryIndexTable && !cacheHit {
								secondaryMappingUnknown = true
							}
						}
						if len(ifv) == 0 {
							if dependencyField {
								stats.recordDependency(f, dependencyTypeForField(opts, f, filterDependency), dependencyState)
							}
							continue
						}
					}
				}
			}
		}

		if opts.topLevel && f.IsTag {
			for _, v := range ifv {
				if tagValue, ok := dependencyTagString(v); ok {
					dependencies.TopLevelTags[f.Name] = tagValue
				}
				break
			}
		}

		fieldRows := map[string]RTableRow{}
		fieldCurrentRows := map[string]bool{}
		fieldSecIdxTab := map[string]string{}
		for idx, v := range ifv {
			if f.SecondaryIndexUse {
				if partial && secondaryMappingUnknown {
					continue
				}
				if newidx, ok := secIdxTab[idx]; ok {
					idx = newidx
				} else {
					if !secGlobalOuterJoin && !f.SecondaryOuterJoin {
						continue
					}
					idx = ".Secondary" + idx
				}
			}
			rtr := fieldRows[idx]
			if rtr.Tags == nil {
				rtr.Tags = map[string]string{}
			}
			if rtr.Fields == nil {
				rtr.Fields = map[string]interface{}{}
			}
			outputIdx := idx
			if t.IndexAsTag && idx != "" {
				tagIdx := idx
				if tagIdx[0] == '.' {
					tagIdx = tagIdx[1:]
				}
				rtr.Tags["index"] = tagIdx
			}

			// don't add an empty string
			vs, ok := v.(string)
			if ok && vs == "" {
				continue
			}
			if f.IsTag {
				if ok {
					rtr.Tags[f.Name] = vs
				} else {
					rtr.Tags[f.Name] = fmt.Sprintf("%v", v)
				}
			} else {
				rtr.Fields[f.Name] = v
			}
			if fieldFetchSucceeded && !f.IsTag && !f.SecondaryIndexTable {
				fieldCurrentRows[outputIdx] = true
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
					fieldSecIdxTab[vss] = idx
				} else {
					fieldSecIdxTab[vss] = "." + idx
				}
			}
			fieldRows[outputIdx] = rtr
		}
		for idx, fieldRow := range fieldRows {
			rtr, ok := rows[idx]
			if !ok {
				rtr = RTableRow{
					Tags:   map[string]string{},
					Fields: map[string]interface{}{},
				}
			}
			for k, v := range fieldRow.Tags {
				rtr.Tags[k] = v
			}
			for k, v := range fieldRow.Fields {
				rtr.Fields[k] = v
			}
			rows[idx] = rtr
		}
		for k, v := range fieldSecIdxTab {
			secIdxTab[k] = v
		}
		for idx := range fieldCurrentRows {
			rowStateFor(idx).hasCurrentOrdinaryValue = true
		}
		if fieldFetchSucceeded {
			stats.recordFieldSuccess()
			if dependencyField {
				if len(ifv) == 0 {
					dependencyState = DependencyStateKnownAbsent
				} else {
					dependencyState = DependencyStateCurrent
				}
				stats.recordDependency(f, dependencyTypeForField(opts, f, filterDependency), dependencyState)
			}
			t.storeSuccessfulDependency(opts, f, filterDependency, ifv, secIdxTab, secGlobalOuterJoin)
		} else if dependencyField && dependencyState != "" {
			stats.recordDependency(f, dependencyTypeForField(opts, f, filterDependency), dependencyState)
		}
		if f.IsTag && dependencyState != "" {
			tagFieldStates[f.Name] = dependencyState
		}
		if filterDependency && dependencyState != "" {
			filterFieldStates[f.Name] = dependencyState
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
			if partial {
				return &BuildResult{Table: &rt, Dependencies: dependencies, Stats: stats}, nil
			}
		}
	}
	strictMode := t.FilterMode == StrictMode
	for idx, r := range rows {
		decision := filterDecisionNone
		params := make(map[string]interface{})
		filterUnknown := false
		if expr != nil {
			for rk, rv := range t.filtersMap {
				if strictMode && !partial {
					params[rk] = false
				}
				if v, ok := r.Tags[rv.fieldName]; ok {
					params[rk] = rv.re.MatchString(v)
					if t.DebugMode {
						log.Printf("D! snmp filter tags, k:%s, v:%v, rv.key:%s, express:%s, match:%t", rv.fieldName, v, rv.key, rv.re.String(), params[rk])
					}
				}

				if v, ok := r.Fields[rv.fieldName]; ok {
					params[rk] = rv.re.MatchString(fmt.Sprintf("%v", v))
					if t.DebugMode {
						log.Printf("D! snmp filter fields, metric:%s, value:%v, rv.key:%s, express:%+v, match:%t", rv.fieldName, v, rv.key, rv.re.String(), params[rk])
					}
				}
				if _, ok := params[rk]; !ok {
					if cachedFields, ok := cachedFilterValues[idx]; ok {
						if v, ok := cachedFields[rv.fieldName]; ok {
							params[rk] = rv.re.MatchString(fmt.Sprintf("%v", v))
							if t.DebugMode {
								log.Printf("D! snmp filter cache, metric:%s, value:%v, rv.key:%s, express:%+v, match:%t", rv.fieldName, v, rv.key, rv.re.String(), params[rk])
							}
						}
					}
				}
				if _, ok := params[rk]; !ok {
					switch filterFieldStates[rv.fieldName] {
					case DependencyStateCurrent, DependencyStateCached, DependencyStateKnownAbsent:
						params[rk] = false
					default:
						filterUnknown = true
					}
				}
			}
			if filterUnknown && partial {
				decision = filterDecisionUnknown
			} else {
				result, err := expr.Evaluate(params)
				if err != nil {
					log.Println("filter expression err:", err)
					if partial {
						decision = filterDecisionUnknown
					}
				} else if match, ok := result.(bool); ok && match {
					decision = filterDecisionAllow
				} else {
					if t.DebugMode {
						log.Printf("D! snmp filter tables, params:%v, table:%+v", params, r)
					}
					decision = filterDecisionDeny
				}
			}
		}
		state := rowDependencyState{
			identityState:  rowIdentityState(r, tagFieldNames, tagFieldStates),
			filterDecision: decision,
		}
		if rowState, ok := rowStates[idx]; ok {
			state.hasCurrentOrdinaryValue = rowState.hasCurrentOrdinaryValue
		}
		allow, reason := rowGate(partial, walk, state)
		if !allow {
			stats.skip(reason)
			continue
		}
		rt.Rows = append(rt.Rows, r)
	}
	return &BuildResult{Table: &rt, Dependencies: dependencies, Stats: stats}, nil
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

func fieldNonStandardConvertInt64Strict(v string) (int64, error) {
	lowerV := strings.ToLower(v)
	multiplier := int64(1)
	numeric := lowerV

	for _, unit := range []struct {
		suffix     string
		multiplier int64
	}{
		{"gb", 1024 * 1024 * 1024}, {"g", 1024 * 1024 * 1024},
		{"tb", 1024 * 1024 * 1024 * 1024}, {"t", 1024 * 1024 * 1024 * 1024},
		{"mb", 1024 * 1024}, {"m", 1024 * 1024},
		{"kb", 1024}, {"k", 1024},
	} {
		if strings.HasSuffix(lowerV, unit.suffix) {
			numeric = strings.TrimSuffix(lowerV, unit.suffix)
			multiplier = unit.multiplier
			break
		}
	}

	parsed, err := strconv.ParseInt(numeric, 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed * multiplier, nil
}

// fieldConvert converts from any type according to the conv specification
func fieldConvert(tr Translator, conv string, ent gosnmp.SnmpPDU) (v interface{}, err error) {
	return fieldConvertMode(tr, conv, ent, false)
}

func fieldConvertStrict(tr Translator, conv string, ent gosnmp.SnmpPDU) (v interface{}, err error) {
	return fieldConvertMode(tr, conv, ent, true)
}

func fieldConvertMode(tr Translator, conv string, ent gosnmp.SnmpPDU, strict bool) (v interface{}, err error) {
	if conv == "" {
		if bs, ok := ent.Value.([]byte); ok {
			return string(bs), nil
		}
		return ent.Value, nil
	}

	var d int
	if _, err := fmt.Sscanf(conv, "float(%d)", &d); err == nil || conv == "float" {
		floatConv := func(vt string) (float64, error) {
			floatVal, err := heuristicDataExtract(vt)
			if err != nil {
				if strict {
					return 0, err
				}
				log.Printf("E! failed to extract float from string: %s, error: %v", vt, err)
				vf, _ := strconv.ParseFloat(vt, 64)
				return vf / math.Pow10(d), nil
			}
			return floatVal / math.Pow10(d), nil
		}

		v = ent.Value
		var floatErr error
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
			v, floatErr = floatConv(string(vt))
		case string:
			v, floatErr = floatConv(vt)
		}
		if floatErr != nil {
			return nil, floatErr
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
			if strict && vt > math.MaxInt64 {
				return nil, fmt.Errorf("uint64 value %d overflows int64 conversion", vt)
			}
			v = int64(vt)
		case []byte:
			if strict {
				v, err = fieldNonStandardConvertInt64Strict(string(vt))
			} else {
				v = fieldNonStandardConvertInt64(string(vt))
			}
		case string:
			if strict {
				v, err = fieldNonStandardConvertInt64Strict(vt)
			} else {
				v = fieldNonStandardConvertInt64(vt)
			}
		}
		if err != nil {
			return nil, err
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
		if strict {
			width := map[string]int{"uint16": 2, "uint32": 4, "uint64": 8}[bit]
			if width > 0 && len(bv) < width {
				return nil, fmt.Errorf("invalid length (%d) for %s hex to int conversion; need at least %d bytes", len(bv), bit, width)
			}
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
