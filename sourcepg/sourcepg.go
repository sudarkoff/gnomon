// Package sourcepg provides a Postgres-backed gnomon.DataSource + gnomon.Store
// over a gnomon_snapshots table. See schema.sql for the expected DDL.
package sourcepg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sudarkoff/gnomon"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Query runs arbitrary metric SQL and returns rows as column->value maps.
func (s *Store) Query(ctx context.Context, sql string) ([]gnomon.Row, error) {
	rows, err := s.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	maps, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, err
	}
	out := make([]gnomon.Row, len(maps))
	for i, m := range maps {
		out[i] = gnomon.Row(m)
	}
	return out, nil
}

// UpsertSnapshots writes (or overwrites) the day's samples for a metric.
// An empty samples slice is a no-op.
func (s *Store) UpsertSnapshots(ctx context.Context, on time.Time, metric string, samples []gnomon.Sample) error {
	if len(samples) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, sm := range samples {
		batch.Queue(
			`INSERT INTO gnomon_snapshots (captured_on, metric, dimension, value)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (captured_on, metric, dimension)
			 DO UPDATE SET value = EXCLUDED.value`,
			on, metric, sm.Dimension, sm.Value,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range samples {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ReadSeries returns stored points for a metric within [from, to], ordered by day.
// Note: from and to are compared against a DATE column; they are interpreted in
// their own time.Location. Callers should pass UTC-based times to avoid
// off-by-one-day boundary surprises.
func (s *Store) ReadSeries(ctx context.Context, metric string, from, to time.Time) ([]gnomon.Point, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT captured_on, dimension, value
		 FROM gnomon_snapshots
		 WHERE metric = $1 AND captured_on BETWEEN $2 AND $3
		 ORDER BY captured_on, dimension`,
		metric, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []gnomon.Point
	for rows.Next() {
		var p gnomon.Point
		if err := rows.Scan(&p.CapturedOn, &p.Dimension, &p.Value); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

var (
	_ gnomon.DataSource = (*Store)(nil)
	_ gnomon.Store      = (*Store)(nil)
)
