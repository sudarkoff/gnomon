package sourcepg

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sudarkoff/gnomon"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("GNOMON_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GNOMON_TEST_DATABASE_URL to run sourcepg integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE gnomon_snapshots"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return New(pool)
}

func TestUpsertAndReadSeries(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	day := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)

	if err := s.UpsertSnapshots(ctx, day, "mrr", []gnomon.Sample{{Dimension: "", Value: 100}}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// re-running the same day overwrites (idempotent)
	if err := s.UpsertSnapshots(ctx, day, "mrr", []gnomon.Sample{{Dimension: "", Value: 150}}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	pts, err := s.ReadSeries(ctx, "mrr", day.AddDate(0, 0, -1), day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("read series: %v", err)
	}
	if len(pts) != 1 || pts[0].Value != 150 {
		t.Fatalf("expected single idempotent point of 150, got %+v", pts)
	}
}

func TestQueryReturnsRows(t *testing.T) {
	s := testStore(t)
	rows, err := s.Query(context.Background(), "SELECT 7 AS value, 'x' AS dimension")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 || rows[0]["dimension"] != "x" {
		t.Fatalf("bad rows: %+v", rows)
	}
}

var (
	_ gnomon.DataSource = (*Store)(nil)
	_ gnomon.Store      = (*Store)(nil)
)
