package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Meta reads a cached metadata entry; ok is false when missing.
func (s *Store) Meta(ctx context.Context, key string) (data string, fetched time.Time, ok bool, err error) {
	var at int64
	err = s.r.QueryRowContext(ctx, `SELECT data, fetched_at FROM meta_cache WHERE key = ?`, key).Scan(&data, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	return data, time.Unix(at, 0), err == nil, err
}

func (s *Store) PutMeta(ctx context.Context, key, data string) error {
	_, err := s.w.ExecContext(ctx, `INSERT INTO meta_cache (key, data, fetched_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET data = excluded.data, fetched_at = excluded.fetched_at`, key, data, time.Now().Unix())
	return err
}

// MetaItem is what metadata lookups need about a film or series.
type MetaItem struct {
	ID             int64
	SubscriptionID int64
	Kind           string
	SourceID       string
	TMDBID         string
	Image          string
}

func (s *Store) MetaItem(ctx context.Context, id int64) (MetaItem, error) {
	var m MetaItem
	err := s.r.QueryRowContext(ctx, `SELECT id, subscription_id, kind, source_id, coalesce(tmdb_id, ''), image FROM item WHERE id = ?`, id).
		Scan(&m.ID, &m.SubscriptionID, &m.Kind, &m.SourceID, &m.TMDBID, &m.Image)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}
