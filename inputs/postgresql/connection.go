package postgresql

import (
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// parseConnectionConfig preserves the v4 connection-string options that users
// used to select the simple protocol or avoid server-side prepared statements.
func parseConnectionConfig(address string) (*pgx.ConnConfig, error) {
	// pgx consumes default_query_exec_mode while parsing. Use the lower-level
	// parser first to detect explicit options, including ones from service files,
	// without trying to split or rewrite quoted keyword strings and URLs.
	raw, err := pgconn.ParseConfig(address)
	if err != nil {
		return nil, err
	}
	if value, ok := raw.RuntimeParams["min_read_buffer_size"]; ok {
		// v4 parsed this client option as int32. v5 manages its read buffer
		// internally, so retain valid legacy DSNs without forwarding the key.
		size, err := strconv.ParseInt(value, 10, 32)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid min_read_buffer_size: expected a non-negative 32-bit integer")
		}
	}
	simple, hasSimple := raw.RuntimeParams["prefer_simple_protocol"]
	cacheMode, hasCacheMode := raw.RuntimeParams["statement_cache_mode"]
	_, hasQueryMode := raw.RuntimeParams["default_query_exec_mode"]
	if hasQueryMode && (hasSimple || hasCacheMode) {
		return nil, fmt.Errorf("default_query_exec_mode cannot be combined with prefer_simple_protocol or statement_cache_mode")
	}

	preferSimple := false
	if hasSimple {
		preferSimple, err = strconv.ParseBool(simple)
		if err != nil {
			return nil, fmt.Errorf("invalid prefer_simple_protocol: %w", err)
		}
	}
	if hasCacheMode && cacheMode != "prepare" && cacheMode != "describe" {
		return nil, fmt.Errorf("invalid statement_cache_mode: expected prepare or describe")
	}

	connConfig, err := pgx.ParseConfig(address)
	if err != nil {
		return nil, err
	}
	// These are client options, never PostgreSQL startup parameters.
	delete(connConfig.RuntimeParams, "prefer_simple_protocol")
	delete(connConfig.RuntimeParams, "statement_cache_mode")
	delete(connConfig.RuntimeParams, "min_read_buffer_size")

	if !hasQueryMode {
		switch {
		case preferSimple:
			connConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		case connConfig.StatementCacheCapacity <= 0:
			// v4 used an unnamed, uncached statement when caching was disabled.
			connConfig.DefaultQueryExecMode = pgx.QueryExecModeDescribeExec
		case cacheMode == "describe":
			connConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
			connConfig.DescriptionCacheCapacity = connConfig.StatementCacheCapacity
		}
	}

	// Accept the socket filename form supported by the existing plugin.
	connConfig.Host = socketRegexp.ReplaceAllLiteralString(connConfig.Host, "")
	return connConfig, nil
}
