package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// RuleSpec says which guide programmes a rule records. Words are matched
// on title and subtitle, ignoring case and accents; a word matches a word
// of the programme that starts with it ("juve" matches "Juventus"), a
// phrase with spaces matches as written.
type RuleSpec struct {
	All     []string `json:"all"`               // every one
	Any     []string `json:"any,omitempty"`     // at least one, if given
	None    []string `json:"none,omitempty"`    // none
	Channel string   `json:"channel,omitempty"` // channel group key; "" any
	Section string   `json:"section,omitempty"` // tv, sport, radio; "" any
}

// RecordingRule records matching programmes automatically.
type RecordingRule struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Spec        RuleSpec `json:"spec"`
	Keep        int      `json:"keep"` // completed recordings to keep, 0 = all
	Enabled     bool     `json:"enabled"`
	CreatedAt   int64    `json:"created_at"`
	LastRun     *int64   `json:"last_run,omitempty"`
	LastNote    string   `json:"last_note,omitempty"`
	ChannelName string   `json:"channel_name,omitempty"`
}

const ruleColumns = `id, name, spec, keep, enabled, created_at, last_run, last_note`

func scanRule(row interface{ Scan(...any) error }) (RecordingRule, error) {
	var r RecordingRule
	var spec string
	var last sql.NullInt64
	if err := row.Scan(&r.ID, &r.Name, &spec, &r.Keep, &r.Enabled, &r.CreatedAt, &last, &r.LastNote); err != nil {
		return r, err
	}
	json.Unmarshal([]byte(spec), &r.Spec)
	if last.Valid {
		r.LastRun = &last.Int64
	}
	return r, nil
}

func (s *Store) RecordingRules(ctx context.Context) ([]RecordingRule, error) {
	rows, err := s.r.QueryContext(ctx, `SELECT `+ruleColumns+` FROM recording_rule ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecordingRule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RecordingRule(ctx context.Context, id int64) (RecordingRule, error) {
	r, err := scanRule(s.r.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM recording_rule WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) AddRecordingRule(ctx context.Context, r RecordingRule) (int64, error) {
	spec, _ := json.Marshal(r.Spec)
	res, err := s.w.ExecContext(ctx, `INSERT INTO recording_rule (name, spec, keep, enabled, created_at) VALUES (?, ?, ?, ?, ?)`,
		r.Name, string(spec), r.Keep, r.Enabled, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateRecordingRule(ctx context.Context, r RecordingRule) error {
	spec, _ := json.Marshal(r.Spec)
	res, err := s.w.ExecContext(ctx, `UPDATE recording_rule SET name = ?, spec = ?, keep = ?, enabled = ? WHERE id = ?`,
		r.Name, string(spec), r.Keep, r.Enabled, r.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRule removes a rule and its scheduled recordings; what it already
// recorded stays.
func (s *Store) DeleteRecordingRule(ctx context.Context, id int64) error {
	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM recording WHERE rule_id = ? AND status IN ('scheduled', 'cancelled')`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE recording SET rule_id = NULL WHERE rule_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM recording_rule WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) SetRecordingRuleResult(ctx context.Context, id int64, note string) error {
	_, err := s.w.ExecContext(ctx, `UPDATE recording_rule SET last_run = ?, last_note = ? WHERE id = ?`, time.Now().Unix(), note, id)
	return err
}
