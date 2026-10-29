package postgresql

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flashcat.cloud/categraf/pkg/cfg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Start at the same TOML loader as a user's configuration, including Init.
func configuredInstance(t *testing.T, address, extra string) (*Instance, error) {
	t.Helper()
	var plugin Postgresql
	err := cfg.LoadSingleConfig(cfg.ConfigWithFormat{
		Format: cfg.TomlFormat,
		Config: fmt.Sprintf("[[instances]]\naddress = %q\n%s", address, extra),
	}, &plugin)
	if err != nil {
		return nil, err
	}
	if len(plugin.Instances) != 1 {
		t.Fatal("expected one configured instance")
	}
	ins := plugin.Instances[0]
	if err := ins.Init(); err != nil {
		return nil, err
	}
	t.Cleanup(func() { stdlib.UnregisterConnConfig(ins.connConfig) })
	if ins.IsPgBouncer {
		t.Fatal("TOML must not set the internal IsPgBouncer flag")
	}
	return ins, nil
}

func TestConnectionConfiguration(t *testing.T) {
	const base = "host=localhost user=test dbname=test sslmode=disable "
	for _, tc := range []struct {
		name, address string
		mode          pgx.QueryExecMode
		errorContains string
	}{
		{"default", base, pgx.QueryExecModeCacheStatement, ""},
		{"legacy true", base + "prefer_simple_protocol=true", pgx.QueryExecModeSimpleProtocol, ""},
		{"legacy false", base + "prefer_simple_protocol=false", pgx.QueryExecModeCacheStatement, ""},
		{"legacy boolean one", base + "prefer_simple_protocol=1", pgx.QueryExecModeSimpleProtocol, ""},
		{"legacy boolean uppercase", base + "prefer_simple_protocol=TRUE", pgx.QueryExecModeSimpleProtocol, ""},
		{"URI legacy", "postgres://test@localhost/test?sslmode=disable&prefer_simple_protocol=true", pgx.QueryExecModeSimpleProtocol, ""},
		{"URI native", "postgresql://test@localhost/test?sslmode=disable&default_query_exec_mode=simple_protocol", pgx.QueryExecModeSimpleProtocol, ""},
		{"native cache statement", base + "default_query_exec_mode=cache_statement", pgx.QueryExecModeCacheStatement, ""},
		{"native cache describe", base + "default_query_exec_mode=cache_describe", pgx.QueryExecModeCacheDescribe, ""},
		{"native describe exec", base + "default_query_exec_mode=describe_exec", pgx.QueryExecModeDescribeExec, ""},
		{"native exec", base + "default_query_exec_mode=exec", pgx.QueryExecModeExec, ""},
		{"legacy prepare", base + "statement_cache_mode=prepare", pgx.QueryExecModeCacheStatement, ""},
		{"legacy describe", base + "statement_cache_mode=describe statement_cache_capacity=7", pgx.QueryExecModeCacheDescribe, ""},
		{"disabled statement cache", base + "statement_cache_capacity=0", pgx.QueryExecModeDescribeExec, ""},
		{"disabled describe cache", base + "statement_cache_mode=describe statement_cache_capacity=0", pgx.QueryExecModeDescribeExec, ""},
		{"simple overrides caching", base + "prefer_simple_protocol=true statement_cache_mode=describe statement_cache_capacity=0", pgx.QueryExecModeSimpleProtocol, ""},
		{"quoted credentials", base + "password='prefer_simple_protocol=false with spaces' prefer_simple_protocol=true", pgx.QueryExecModeSimpleProtocol, ""},
		{"invalid boolean", base + "prefer_simple_protocol=maybe", 0, "invalid prefer_simple_protocol"},
		{"invalid native mode", base + "default_query_exec_mode=invalid", 0, "invalid default_query_exec_mode"},
		{"invalid cache mode", base + "statement_cache_mode=invalid", 0, "invalid statement_cache_mode"},
		{"conflicting simple modes", base + "prefer_simple_protocol=true default_query_exec_mode=simple_protocol", 0, "cannot be combined"},
		{"conflicting false mode", base + "prefer_simple_protocol=false default_query_exec_mode=exec", 0, "cannot be combined"},
		{"conflicting cache mode", base + "statement_cache_mode=prepare default_query_exec_mode=exec", 0, "cannot be combined"},
		{"URI conflicting modes", "postgres://test@localhost/test?prefer_simple_protocol=true&default_query_exec_mode=exec", 0, "cannot be combined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ins, err := configuredInstance(t, tc.address, "prepared_statements = false\n")
			if tc.errorContains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorContains) {
					t.Fatalf("expected error containing %q, got %v", tc.errorContains, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			conn, err := parseConnectionConfig(ins.Address)
			if err != nil {
				t.Fatal(err)
			}
			if conn.DefaultQueryExecMode != tc.mode {
				t.Fatalf("mode = %v, want %v", conn.DefaultQueryExecMode, tc.mode)
			}
			for _, key := range []string{"prefer_simple_protocol", "statement_cache_mode", "default_query_exec_mode", "statement_cache_capacity"} {
				if _, ok := conn.RuntimeParams[key]; ok {
					t.Errorf("client option %q leaked into startup parameters", key)
				}
			}
			if tc.name == "legacy describe" && conn.DescriptionCacheCapacity != 7 {
				t.Errorf("description cache capacity = %d, want 7", conn.DescriptionCacheCapacity)
			}
		})
	}
}

func TestConnectionServiceFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pg_service.conf")
	if err := os.WriteFile(path, []byte("[legacy]\nhost=localhost\nprefer_simple_protocol=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PGSERVICEFILE", path)
	ins, err := configuredInstance(t, "service=legacy", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := parseConnectionConfig(ins.Address)
	if err != nil || conn.DefaultQueryExecMode != pgx.QueryExecModeSimpleProtocol {
		t.Fatalf("service file simple protocol: config=%v, err=%v", conn, err)
	}
	if _, err := configuredInstance(t, "service=legacy default_query_exec_mode=exec", ""); err == nil {
		t.Fatal("expected conflict with legacy mode inherited from service file")
	}
}

func TestConnectionUnixSocketFilename(t *testing.T) {
	ins, err := configuredInstance(t, "host=/tmp/postgresql/.s.PGSQL.5432 sslmode=disable", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := parseConnectionConfig(ins.Address)
	if err != nil || conn.Host != "/tmp/postgresql" {
		t.Fatalf("unexpected socket config: %v, %v", conn, err)
	}
}
