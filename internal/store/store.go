// Package store persists VPN connections, subscriptions and the catalog in
// SQLite. Reads use their own connection pool (WAL), so a catalog import
// never blocks browsing.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"runtime"

	_ "modernc.org/sqlite"
)

// Store holds one writer connection and a pool of read-only connections.
type Store struct {
	w *sql.DB
	r *sql.DB
}

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("non trovato")

// Open creates or migrates the database at path.
func Open(path string) (*Store, error) {
	dsn := func(extra string) string {
		q := url.Values{}
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "busy_timeout(10000)")
		q.Add("_pragma", "foreign_keys(1)")
		q.Add("_pragma", "synchronous(NORMAL)")
		if extra != "" {
			q.Add("_pragma", extra)
		}
		return "file:" + path + "?" + q.Encode()
	}
	w, err := sql.Open("sqlite", dsn(""))
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	r, err := sql.Open("sqlite", dsn("query_only(1)"))
	if err != nil {
		w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(max(4, runtime.NumCPU()))
	s := &Store{w: w, r: r}
	if err := s.migrate(context.Background()); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close releases both pools.
func (s *Store) Close() error {
	return errors.Join(s.r.Close(), s.w.Close())
}

var migrations = []string{
	// 1: VPN, subscriptions, catalog.
	`CREATE TABLE vpn (
		id         INTEGER PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		config     BLOB NOT NULL,           -- sealed wg-quick text
		created_at INTEGER NOT NULL
	);
	CREATE TABLE subscription (
		id          INTEGER PRIMARY KEY,
		name        TEXT NOT NULL UNIQUE,
		kind        TEXT NOT NULL CHECK (kind IN ('xtream', 'm3u')),
		vpn_id      INTEGER NOT NULL REFERENCES vpn(id),
		settings    TEXT NOT NULL,           -- JSON, no secrets
		secret      BLOB NOT NULL,           -- sealed password or playlist URL
		created_at  INTEGER NOT NULL,
		synced_at   INTEGER,
		sync_status TEXT NOT NULL DEFAULT 'never',
		sync_error  TEXT NOT NULL DEFAULT '',
		account     TEXT NOT NULL DEFAULT '{}' -- JSON: provider account state
	);
	CREATE TABLE category (
		subscription_id INTEGER NOT NULL REFERENCES subscription(id) ON DELETE CASCADE,
		kind            TEXT NOT NULL,
		source_id       TEXT NOT NULL,
		name            TEXT NOT NULL,
		PRIMARY KEY (subscription_id, kind, source_id)
	);
	CREATE TABLE item (
		id              INTEGER PRIMARY KEY,
		subscription_id INTEGER NOT NULL REFERENCES subscription(id) ON DELETE CASCADE,
		kind            TEXT NOT NULL CHECK (kind IN ('live', 'movie', 'series')),
		source_id       TEXT NOT NULL,
		title           TEXT NOT NULL,       -- as published by the provider
		display_title   TEXT NOT NULL,       -- without quality tags / year
		year            INTEGER,
		category_id     TEXT NOT NULL DEFAULT '',
		image           TEXT NOT NULL DEFAULT '',
		epg_id          TEXT NOT NULL DEFAULT '',
		number          INTEGER,
		rating          REAL,
		tmdb_id         TEXT NOT NULL DEFAULT '',
		extension       TEXT NOT NULL DEFAULT '',
		catchup_days    INTEGER NOT NULL DEFAULT 0,
		added_at        INTEGER,
		locator         BLOB,                -- sealed stream URL (M3U only)
		first_seen_at   INTEGER NOT NULL,
		last_seen_at    INTEGER NOT NULL,
		available       INTEGER NOT NULL DEFAULT 1,
		UNIQUE (subscription_id, kind, source_id)
	);
	CREATE INDEX item_by_title ON item (kind, available, display_title COLLATE NOCASE, id);
	CREATE INDEX item_by_arrival ON item (kind, available, first_seen_at DESC, id);
	CREATE INDEX item_by_category ON item (subscription_id, kind, category_id, available);
	CREATE VIRTUAL TABLE item_search USING fts5 (
		display_title, content = 'item', content_rowid = 'id',
		tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3'
	);`,
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.w.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	err := s.w.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.w.ExecContext(ctx, `INSERT INTO schema_version VALUES (0)`); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database creato da una versione più recente (schema %d)", version)
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.w.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrazione %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schema_version SET version = ?`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
