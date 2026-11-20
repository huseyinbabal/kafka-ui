// Package analytics samples clusters periodically and serves time series.
package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Store persists samples in SQLite and answers range queries.
type Store struct {
	db *sql.DB
	// RawFor is how long full-resolution samples are kept before rollup.
	RawFor time.Duration
	// Retention is how long minute rollups are kept.
	Retention time.Duration
	now       func() time.Time
}

func NewStore(db *sql.DB, retention time.Duration) *Store {
	return &Store{db: db, RawFor: 6 * time.Hour, Retention: retention, now: time.Now}
}

type TopicSample struct {
	Topic     string
	EndSum    int64
	StartSum  int64
	SizeBytes int64
}

type GroupSample struct {
	Group        string
	Topic        string
	CommittedSum int64
	Lag          int64
}

type ClusterSample struct {
	Partitions, URP, Offline int
}

// Snapshot is everything one collector tick writes.
type Snapshot struct {
	Cluster string
	At      time.Time
	Topics  []TopicSample
	Groups  []GroupSample
	Overall ClusterSample
}

// Write stores one snapshot in a single transaction.
func (s *Store) Write(ctx context.Context, snap Snapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ts := snap.At.UnixMilli()
	tst, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO topic_samples_raw VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, t := range snap.Topics {
		if _, err := tst.ExecContext(ctx, snap.Cluster, t.Topic, ts, t.EndSum, t.StartSum, t.SizeBytes); err != nil {
			return err
		}
	}
	gst, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO group_samples_raw VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, g := range snap.Groups {
		if _, err := gst.ExecContext(ctx, snap.Cluster, g.Group, g.Topic, ts, g.CommittedSum, g.Lag); err != nil {
			return err
		}
	}
	o := snap.Overall
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO cluster_samples_raw VALUES (?,?,?,?,?)`,
		snap.Cluster, ts, o.Partitions, o.URP, o.Offline); err != nil {
		return err
	}
	return tx.Commit()
}

// Compact rolls raw samples older than RawFor into minute buckets and drops
// rollups older than Retention. Counters keep the last (max) value per minute;
// lag and URP keep the max, so spikes stay visible.
func (s *Store) Compact(ctx context.Context) error {
	now := s.now()
	// Align to a minute so a bucket is never split between raw and rollup.
	cutoff := now.Add(-s.RawFor).Truncate(time.Minute).UnixMilli()
	expire := now.Add(-s.Retention).UnixMilli()
	stmts := []string{
		`INSERT OR REPLACE INTO topic_samples_1m
		 SELECT cluster, topic, (ts/60000)*60000, MAX(end_sum), MAX(start_sum), MAX(size_bytes)
		 FROM topic_samples_raw WHERE ts < ?1 GROUP BY cluster, topic, ts/60000`,
		`DELETE FROM topic_samples_raw WHERE ts < ?1`,
		`INSERT OR REPLACE INTO group_samples_1m
		 SELECT cluster, grp, topic, (ts/60000)*60000, MAX(committed_sum), MAX(lag)
		 FROM group_samples_raw WHERE ts < ?1 GROUP BY cluster, grp, topic, ts/60000`,
		`DELETE FROM group_samples_raw WHERE ts < ?1`,
		`INSERT OR REPLACE INTO cluster_samples_1m
		 SELECT cluster, (ts/60000)*60000, MAX(partitions), MAX(urp), MAX(offline)
		 FROM cluster_samples_raw WHERE ts < ?1 GROUP BY cluster, ts/60000`,
		`DELETE FROM cluster_samples_raw WHERE ts < ?1`,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q, cutoff); err != nil {
			return fmt.Errorf("compact: %w", err)
		}
	}
	for _, t := range []string{"topic_samples_1m", "group_samples_1m", "cluster_samples_1m"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE ts < ?`, expire); err != nil { //nolint:gosec // fixed table names
			return fmt.Errorf("expire %s: %w", t, err)
		}
	}
	return tx.Commit()
}
