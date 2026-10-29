package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Run only against disposable test databases: the security regression creates a
// transaction-local canary table. No tables or data survive the transaction.
func TestPostgreSQLIntegration(t *testing.T) {
	for _, endpoint := range []struct{ name, env string }{
		{"postgres", "CATEGRAF_POSTGRES_TEST_DSN"},
		{"postgres18", "CATEGRAF_POSTGRES18_TEST_DSN"},
		{"pgbouncer", "CATEGRAF_PGBOUNCER_TEST_DSN"},
		{"TLS", "CATEGRAF_POSTGRES_TLS_TEST_DSN"},
		{"socket", "CATEGRAF_POSTGRES_SOCKET_TEST_DSN"},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			address := os.Getenv(endpoint.env)
			if address == "" {
				t.Skipf("set %s to a disposable test database", endpoint.env)
			}
			for _, tc := range []struct {
				name, options string
				mode          pgx.QueryExecMode
			}{
				{"default", "", pgx.QueryExecModeCacheStatement},
				{"legacy simple", "prefer_simple_protocol=true", pgx.QueryExecModeSimpleProtocol},
				{"native simple", "default_query_exec_mode=simple_protocol", pgx.QueryExecModeSimpleProtocol},
				{"legacy describe", "statement_cache_mode=describe statement_cache_capacity=7", pgx.QueryExecModeCacheDescribe},
				{"uncached", "statement_cache_capacity=0", pgx.QueryExecModeDescribeExec},
				{"legacy read buffer", "min_read_buffer_size=16384", pgx.QueryExecModeCacheStatement},
				{"legacy read buffer simple", "prefer_simple_protocol=true min_read_buffer_size=16384", pgx.QueryExecModeSimpleProtocol},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if endpoint.name == "pgbouncer" && tc.mode == pgx.QueryExecModeCacheStatement {
						t.Skip("transaction pooling with max_prepared_statements=0 does not support named statement caching; use simple protocol or an unprepared mode")
					}
					ins, err := configuredInstance(t, withConnectionOptions(t, address, tc.options), "")
					if err != nil {
						t.Fatal(err)
					}
					db, err := sql.Open("pgx/v5", ins.connConfig)
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					conn, err := db.Conn(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					if err := conn.Raw(func(driverConn interface{}) error {
						config := driverConn.(*stdlib.Conn).Conn().Config()
						if config.DefaultQueryExecMode != tc.mode {
							return fmt.Errorf("actual driver mode = %v, want %v", config.DefaultQueryExecMode, tc.mode)
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					assertSQLTypes(t, ctx, conn)
					var bound int
					if err := conn.QueryRowContext(ctx, "SELECT $1::int + 1", 41).Scan(&bound); err != nil || bound != 42 {
						t.Fatalf("parameterized query: value=%d, err=%v", bound, err)
					}
					if tc.mode == pgx.QueryExecModeSimpleProtocol {
						assertDollarQuotedParameters(t, ctx, conn)
					}
					var ignored int
					if err := conn.QueryRowContext(ctx, "SELECT 1 / 0").Scan(&ignored); err == nil {
						t.Fatal("expected SQL error")
					}
					shortCtx, shortCancel := context.WithTimeout(ctx, 100*time.Millisecond)
					defer shortCancel()
					if _, err := conn.ExecContext(shortCtx, "SELECT pg_sleep(5)"); err == nil || shortCtx.Err() == nil {
						t.Fatalf("expected query timeout, got %v", err)
					}
					// Gather uses its own registered sql connection and exercises
					// version detection, default metrics and custom SQL conversion.
					ins.Metrics = []MetricConfig{{
						Mesurement: "migration", LabelFields: []string{"label"},
						MetricFields: []string{"integer", "decimal", "floating"},
						Timeout:      10_000_000_000,
						Request:      "SELECT 'driver test'::text AS label, 42::bigint AS integer, 12.5::numeric AS decimal, 3.25::float8 AS floating",
					}}
					assertGatheredMetrics(t, ins)
				})
			}
		})
	}
}

func withConnectionOptions(t *testing.T, address, options string) string {
	t.Helper()
	if strings.HasPrefix(address, "postgres://") || strings.HasPrefix(address, "postgresql://") {
		u, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}
		query := u.Query()
		for _, option := range strings.Fields(options) {
			key, value, _ := strings.Cut(option, "=")
			query.Set(key, value)
		}
		u.RawQuery = query.Encode()
		return u.String()
	}
	return address + " " + options
}

func assertSQLTypes(t *testing.T, ctx context.Context, conn *sql.Conn) {
	t.Helper()
	var integer int64
	var decimal, floating float64
	var text string
	var timestamp time.Time
	var nullable sql.NullString
	err := conn.QueryRowContext(ctx, `SELECT 42::bigint, 12.5::numeric, 3.25::float8,
		'text'::text, '2026-01-02 03:04:05+00'::timestamptz, NULL::text`).
		Scan(&integer, &decimal, &floating, &text, &timestamp, &nullable)
	if err != nil {
		t.Fatal(err)
	}
	if integer != 42 || decimal != 12.5 || floating != 3.25 || text != "text" ||
		!timestamp.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) || nullable.Valid {
		t.Fatalf("unexpected SQL scan values: %v %v %v %v %v %v", integer, decimal, floating, text, timestamp, nullable)
	}
}

func assertDollarQuotedParameters(t *testing.T, ctx context.Context, conn *sql.Conn) {
	t.Helper()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE categraf_security_canary (value int) ON COMMIT DROP"); err != nil {
		t.Fatal(err)
	}
	attack := "$tag$; DROP TABLE categraf_security_canary; --"
	var literal, bound string
	if err := tx.QueryRowContext(ctx, `SELECT $tag$ $1 $tag$, $1`, attack).Scan(&literal, &bound); err != nil {
		t.Fatalf("dollar-quoted parameter regression: %v", err)
	}
	if literal != " $1 " || bound != attack {
		t.Fatalf("query literal or bound data changed: %q, %q", literal, bound)
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM categraf_security_canary").Scan(&count); err != nil {
		t.Fatalf("canary table affected by parameter: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func assertGatheredMetrics(t *testing.T, ins *Instance) {
	t.Helper()
	slist := types.NewSampleList()
	ins.Gather(slist)
	samples := slist.PopBackAll()
	expected := map[string]float64{
		"postgresql_up":                 1,
		"postgresql_migration_integer":  42,
		"postgresql_migration_decimal":  12.5,
		"postgresql_migration_floating": 3.25,
	}
	names := make([]string, 0, len(samples))
	for _, sample := range samples {
		names = append(names, sample.Metric)
		if want, ok := expected[sample.Metric]; ok {
			if fmt.Sprint(sample.Value) != fmt.Sprint(want) {
				t.Errorf("%s = %v, want %v", sample.Metric, sample.Value, want)
			}
			if strings.Contains(sample.Metric, "migration") && sample.Labels["label"] != "driver_test" {
				t.Errorf("custom metric label changed: %v", sample.Labels)
			}
			delete(expected, sample.Metric)
		}
	}
	if len(expected) > 0 {
		t.Errorf("missing metrics: %v", expected)
	}
	requireMetricPrefixes(t, names, "postgresql_numbackends", "postgresql_buffers_alloc", "postgresql_checkpoints_timed")
}
