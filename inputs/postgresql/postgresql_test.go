package postgresql

import (
	"testing"

	"flashcat.cloud/categraf/types"
	"github.com/DATA-DOG/go-sqlmock"
)

func newTestInstance() *Instance {
	return &Instance{
		Address:       "host=localhost dbname=test sslmode=disable",
		OutputAddress: "localhost:5432",
		Version:       160000, // PG16 to avoid version query
		MaxIdle:       1,
		MaxOpen:       1,
	}
}

func sampleNames(slist *types.SampleList) []string {
	samples := slist.PopBackAll()
	names := make([]string, 0, len(samples))
	for _, s := range samples {
		names = append(names, s.Metric)
	}
	return names
}

func hasPrefix(names []string, prefix string) bool {
	for _, n := range names {
		if len(n) >= len(prefix) && n[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func requireMetricPrefixes(t *testing.T, names []string, prefixes ...string) {
	t.Helper()

	for _, prefix := range prefixes {
		if !hasPrefix(names, prefix) {
			t.Fatalf("expected metric prefix %q, got: %v", prefix, names)
		}
	}
}

func TestGatherMetrics_DefaultCollectsAll(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ins := newTestInstance()
	ins.db = db

	// pg_stat_database
	mock.ExpectQuery(`^SELECT \* FROM pg_stat_database$`).WillReturnRows(
		sqlmock.NewRows([]string{"datname", "numbackends", "xact_commit"}).
			AddRow("testdb", 5, 100),
	)

	// pg_stat_bgwriter (PG < 17)
	mock.ExpectQuery(`^SELECT \* FROM pg_stat_bgwriter$`).WillReturnRows(
		sqlmock.NewRows([]string{"checkpoints_timed", "buffers_alloc"}).
			AddRow(10, 200),
	)

	slist := types.NewSampleList()
	ins.gatherMetrics(slist)

	names := sampleNames(slist)
	requireMetricPrefixes(t, names,
		"postgresql_numbackends",
		"postgresql_xact_commit",
		"postgresql_checkpoints_timed",
		"postgresql_buffers_alloc",
	)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGatherMetrics_DisablePgStatDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ins := newTestInstance()
	ins.db = db
	ins.DisablePgStatDatabase = true

	// Only pg_stat_bgwriter should be queried
	mock.ExpectQuery(`^SELECT \* FROM pg_stat_bgwriter$`).WillReturnRows(
		sqlmock.NewRows([]string{"checkpoints_timed", "buffers_alloc"}).
			AddRow(10, 200),
	)

	slist := types.NewSampleList()
	ins.gatherMetrics(slist)

	names := sampleNames(slist)
	if hasPrefix(names, "postgresql_numbackends") || hasPrefix(names, "postgresql_xact_commit") {
		t.Errorf("pg_stat_database metrics should be skipped when disabled, got: %v", names)
	}
	requireMetricPrefixes(t, names, "postgresql_checkpoints_timed", "postgresql_buffers_alloc")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGatherMetrics_DisablePgStatBgwriter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ins := newTestInstance()
	ins.db = db
	ins.DisablePgStatBgwriter = true

	// Only pg_stat_database should be queried
	mock.ExpectQuery(`^SELECT \* FROM pg_stat_database$`).WillReturnRows(
		sqlmock.NewRows([]string{"datname", "numbackends", "xact_commit"}).
			AddRow("testdb", 5, 100),
	)

	slist := types.NewSampleList()
	ins.gatherMetrics(slist)

	names := sampleNames(slist)
	requireMetricPrefixes(t, names, "postgresql_numbackends", "postgresql_xact_commit")
	if hasPrefix(names, "postgresql_checkpoints_timed") || hasPrefix(names, "postgresql_buffers_alloc") {
		t.Errorf("pg_stat_bgwriter metrics should be skipped when disabled, got: %v", names)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGatherMetrics_Pg17CollectsBgwriterAndCheckpointer(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ins := newTestInstance()
	ins.db = db
	ins.Version = 170000

	mock.ExpectQuery(`^SELECT \* FROM pg_stat_database$`).WillReturnRows(
		sqlmock.NewRows([]string{"datname", "numbackends", "xact_commit"}).
			AddRow("testdb", 5, 100),
	)

	mock.ExpectQuery(`^SELECT \* FROM pg_stat_bgwriter$`).WillReturnRows(
		sqlmock.NewRows([]string{"buffers_clean", "maxwritten_clean"}).
			AddRow(20, 2),
	)

	mock.ExpectQuery(`(?s)^SELECT\s+num_timed AS checkpoints_timed,.*FROM pg_stat_checkpointer$`).WillReturnRows(
		sqlmock.NewRows([]string{
			"checkpoints_timed",
			"checkpoints_req",
			"checkpoint_write_time",
			"checkpoint_sync_time",
			"buffers_checkpoint",
			"restartpoints_timed",
			"restartpoints_req",
			"restartpoints_done",
		}).AddRow(10, 1, 11.5, 3.2, 200, 4, 5, 6),
	)

	slist := types.NewSampleList()
	ins.gatherMetrics(slist)

	names := sampleNames(slist)
	requireMetricPrefixes(t, names,
		"postgresql_numbackends",
		"postgresql_xact_commit",
		"postgresql_buffers_clean",
		"postgresql_maxwritten_clean",
		"postgresql_checkpoints_timed",
		"postgresql_checkpoints_req",
		"postgresql_checkpoint_write_time",
		"postgresql_checkpoint_sync_time",
		"postgresql_buffers_checkpoint",
		"postgresql_restartpoints_timed",
		"postgresql_restartpoints_req",
		"postgresql_restartpoints_done",
	)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGatherMetrics_DisableBoth(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ins := newTestInstance()
	ins.db = db
	ins.DisablePgStatDatabase = true
	ins.DisablePgStatBgwriter = true

	// No queries expected
	slist := types.NewSampleList()
	ins.gatherMetrics(slist)

	names := sampleNames(slist)
	if len(names) != 0 {
		t.Errorf("expected no metrics when both disabled, got: %v", names)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
