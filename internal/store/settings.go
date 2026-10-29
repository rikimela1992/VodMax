package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Setting reads a JSON value into v; a missing key leaves v unchanged.
func (s *Store) Setting(ctx context.Context, key string, v any) error {
	var raw string
	err := s.r.QueryRowContext(ctx, `SELECT value FROM setting WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

// SetSetting stores v as JSON.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.w.ExecContext(ctx, `INSERT INTO setting (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, string(raw))
	return err
}
