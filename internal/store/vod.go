package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Episode is one episode of a series.
type Episode struct {
	ID           int64     `json:"id"`
	SeriesItemID int64     `json:"series_id"`
	SourceID     string    `json:"-"`
	Season       int       `json:"season"`
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Extension    string    `json:"-"`
	Duration     int       `json:"duration,omitempty"`
	Plot         string    `json:"plot,omitempty"`
	Progress     *Progress `json:"progress,omitempty"`
}

// SeriesFetchedAt tells when the episodes of a series were last loaded.
func (s *Store) SeriesFetchedAt(ctx context.Context, seriesItemID int64) (time.Time, error) {
	var at int64
	err := s.r.QueryRowContext(ctx, `SELECT fetched_at FROM series_fetch WHERE series_item_id = ?`, seriesItemID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	return time.Unix(at, 0), err
}

// ReplaceEpisodes stores the episodes of a series, keeping the ids (and so
// the progress) of episodes that still exist.
func (s *Store) ReplaceEpisodes(ctx context.Context, seriesItemID int64, eps []Episode) error {
	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := make([]any, 0, len(eps)+1)
	keep = append(keep, seriesItemID)
	for _, e := range eps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO episode (series_item_id, source_id, season, number, title, extension, duration, plot)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (series_item_id, source_id) DO UPDATE SET season = excluded.season, number = excluded.number,
				title = excluded.title, extension = excluded.extension, duration = excluded.duration, plot = excluded.plot`,
			seriesItemID, e.SourceID, e.Season, e.Number, e.Title, e.Extension, e.Duration, e.Plot); err != nil {
			return err
		}
		keep = append(keep, e.SourceID)
	}
	if len(eps) > 0 {
		q := `DELETE FROM episode WHERE series_item_id = ? AND source_id NOT IN (?` + repeat(",?", len(eps)-1) + `)`
		if _, err := tx.ExecContext(ctx, q, keep...); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO series_fetch VALUES (?, ?)`, seriesItemID, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

// Episodes lists a series' episodes in order, with the profile's progress.
func (s *Store) Episodes(ctx context.Context, profileID, seriesItemID int64) ([]Episode, error) {
	rows, err := s.r.QueryContext(ctx, `SELECT e.id, e.series_item_id, e.source_id, e.season, e.number, e.title, e.extension,
			e.duration, e.plot, p.position, p.duration, p.completed, p.updated_at
		FROM episode e LEFT JOIN progress p ON p.profile_id = ? AND p.media = 'episode' AND p.media_id = e.id
		WHERE e.series_item_id = ? ORDER BY e.season, e.number, e.id`, profileID, seriesItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Episode{}
	for rows.Next() {
		var e Episode
		var pos, dur, completed, updated sql.NullInt64
		if err := rows.Scan(&e.ID, &e.SeriesItemID, &e.SourceID, &e.Season, &e.Number, &e.Title, &e.Extension,
			&e.Duration, &e.Plot, &pos, &dur, &completed, &updated); err != nil {
			return nil, err
		}
		if pos.Valid {
			e.Progress = &Progress{Position: int(pos.Int64), Duration: int(dur.Int64), Completed: completed.Int64 == 1, UpdatedAt: updated.Int64}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EpisodePlayback returns what is needed to open an episode.
func (s *Store) EpisodePlayback(ctx context.Context, episodeID int64) (PlaybackItem, Episode, error) {
	var it PlaybackItem
	var e Episode
	err := s.r.QueryRowContext(ctx, `SELECT e.id, e.series_item_id, e.source_id, e.season, e.number, e.title, e.extension,
			e.duration, i.subscription_id, i.display_title
		FROM episode e JOIN item i ON i.id = e.series_item_id WHERE e.id = ? AND i.available = 1`, episodeID).
		Scan(&e.ID, &e.SeriesItemID, &e.SourceID, &e.Season, &e.Number, &e.Title, &e.Extension, &e.Duration,
			&it.SubscriptionID, &it.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return it, e, ErrNotFound
	}
	it.ID, it.Kind, it.SourceID, it.Extension = e.ID, "episode", e.SourceID, e.Extension
	return it, e, err
}

// Progress is how far a profile got in a film or episode.
type Progress struct {
	Position  int   `json:"position"`
	Duration  int   `json:"duration"`
	Completed bool  `json:"completed"`
	UpdatedAt int64 `json:"updated_at"`
}

// SaveProgress records a position. A title counts as watched from 95% (or
// the last 3 minutes) on: it then leaves "continue watching".
func (s *Store) SaveProgress(ctx context.Context, profileID int64, media string, mediaID int64, position, duration int) (Progress, error) {
	if position < 0 || duration < 0 || (duration > 0 && position > duration+60) {
		return Progress{}, errors.New("posizione non valida")
	}
	completed := duration > 0 && (position >= duration*95/100 || duration-position <= 180)
	now := time.Now().Unix()
	_, err := s.w.ExecContext(ctx, `INSERT INTO progress VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (profile_id, media, media_id) DO UPDATE SET position = excluded.position,
			duration = excluded.duration, completed = excluded.completed, updated_at = excluded.updated_at`,
		profileID, media, mediaID, position, duration, completed, now)
	return Progress{Position: position, Duration: duration, Completed: completed, UpdatedAt: now}, err
}

// GetProgress returns the progress of one film or episode, nil if none.
func (s *Store) GetProgress(ctx context.Context, profileID int64, media string, mediaID int64) (*Progress, error) {
	var p Progress
	err := s.r.QueryRowContext(ctx, `SELECT position, duration, completed, updated_at FROM progress
		WHERE profile_id = ? AND media = ? AND media_id = ?`, profileID, media, mediaID).
		Scan(&p.Position, &p.Duration, &p.Completed, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

// ContinueEntry is one row of "continue watching".
type ContinueEntry struct {
	Media        string   `json:"media"` // movie or episode
	ID           int64    `json:"id"`    // item id (movie) or episode id
	Title        string   `json:"title"`
	SeriesID     int64    `json:"series_id,omitempty"`
	SeriesTitle  string   `json:"series_title,omitempty"`
	Season       int      `json:"season,omitempty"`
	Number       int      `json:"number,omitempty"`
	Year         *int     `json:"year,omitempty"`
	Progress     Progress `json:"progress"`
	ImageItemID  int64    `json:"-"`
	ImageAllowed bool     `json:"-"`
	Logo         string   `json:"image,omitempty"`
}

// Continue lists unfinished films and episodes, most recent first. For a
// series only the latest episode appears.
func (s *Store) Continue(ctx context.Context, profileID int64, noAdult bool, limit int) ([]ContinueEntry, error) {
	rows, err := s.r.QueryContext(ctx, `
		SELECT 'movie', i.id, i.display_title, 0, '', 0, 0, i.year, p.position, p.duration, p.updated_at, i.id, i.image <> ''
		FROM progress p JOIN item i ON i.id = p.media_id
		WHERE p.profile_id = ? AND p.media = 'movie' AND p.completed = 0 AND i.available = 1 AND (? = 0 OR i.adult = 0)
		UNION ALL
		SELECT 'episode', e.id, e.title, i.id, i.display_title, e.season, e.number, i.year, p.position, p.duration, p.updated_at,
			i.id, i.image <> ''
		FROM progress p JOIN episode e ON e.id = p.media_id JOIN item i ON i.id = e.series_item_id
		WHERE p.profile_id = ? AND p.media = 'episode' AND p.completed = 0 AND i.available = 1 AND (? = 0 OR i.adult = 0)
			AND p.updated_at = (SELECT max(p2.updated_at) FROM progress p2 JOIN episode e2 ON e2.id = p2.media_id
				WHERE p2.profile_id = p.profile_id AND p2.media = 'episode' AND e2.series_item_id = e.series_item_id)
		ORDER BY 11 DESC LIMIT ?`, profileID, noAdult, profileID, noAdult, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContinueEntry{}
	for rows.Next() {
		var c ContinueEntry
		var year sql.NullInt64
		if err := rows.Scan(&c.Media, &c.ID, &c.Title, &c.SeriesID, &c.SeriesTitle, &c.Season, &c.Number, &year,
			&c.Progress.Position, &c.Progress.Duration, &c.Progress.UpdatedAt, &c.ImageItemID, &c.ImageAllowed); err != nil {
			return nil, err
		}
		if year.Valid {
			y := int(year.Int64)
			c.Year = &y
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Recording is a scheduled or made recording of a live channel.
type Recording struct {
	ID        int64  `json:"id"`
	ItemID    int64  `json:"channel_id"`
	GroupKey  string `json:"-"`
	Channel   string `json:"channel"`
	Title     string `json:"title"`
	Start     int64  `json:"start"`
	Stop      int64  `json:"stop"`
	Status    string `json:"status"`
	File      string `json:"-"`
	Bytes     int64  `json:"bytes"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
	RuleID    int64  `json:"rule_id,omitempty"` // created by a rule
	Protected bool   `json:"protected"`         // never deleted automatically
}

const recordingColumns = `id, item_id, group_key, channel, title, start, stop, status, file, bytes, error, created_at, coalesce(rule_id, 0), protected`

func (r *Recording) scanTargets() []any {
	return []any{&r.ID, &r.ItemID, &r.GroupKey, &r.Channel, &r.Title, &r.Start, &r.Stop, &r.Status, &r.File, &r.Bytes, &r.Error,
		&r.CreatedAt, &r.RuleID, &r.Protected}
}

func scanRecording(row interface{ Scan(...any) error }) (Recording, error) {
	var r Recording
	err := row.Scan(r.scanTargets()...)
	return r, err
}

func (s *Store) AddRecording(ctx context.Context, r Recording) (int64, error) {
	var rule any
	if r.RuleID != 0 {
		rule = r.RuleID
	}
	res, err := s.w.ExecContext(ctx, `INSERT INTO recording (item_id, group_key, channel, title, start, stop, status, created_at, rule_id)
		VALUES (?, ?, ?, ?, ?, ?, 'scheduled', ?, ?)`, r.ItemID, r.GroupKey, r.Channel, r.Title, r.Start, r.Stop, time.Now().Unix(), rule)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Recording(ctx context.Context, id int64) (Recording, error) {
	r, err := scanRecording(s.r.QueryRowContext(ctx, `SELECT `+recordingColumns+` FROM recording WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// Recordings lists recordings, newest first.
func (s *Store) Recordings(ctx context.Context) ([]Recording, error) {
	return s.queryRecordings(ctx, `SELECT `+recordingColumns+` FROM recording WHERE status <> 'cancelled' ORDER BY start DESC`)
}

// DueRecordings lists scheduled recordings that should be running at t.
func (s *Store) DueRecordings(ctx context.Context, t time.Time) ([]Recording, error) {
	return s.queryRecordings(ctx, `SELECT `+recordingColumns+` FROM recording
		WHERE status = 'scheduled' AND start <= ? ORDER BY start`, t.Unix())
}

// RecordingsInState lists recordings with the given status.
func (s *Store) RecordingsInState(ctx context.Context, status string) ([]Recording, error) {
	return s.queryRecordings(ctx, `SELECT `+recordingColumns+` FROM recording WHERE status = ?`, status)
}

func (s *Store) queryRecordings(ctx context.Context, q string, args ...any) ([]Recording, error) {
	rows, err := s.r.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recording{}
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateRecording stores status, file, size and error of a recording.
func (s *Store) UpdateRecording(ctx context.Context, r Recording) error {
	_, err := s.w.ExecContext(ctx, `UPDATE recording SET status = ?, file = ?, bytes = ?, error = ? WHERE id = ?`,
		r.Status, r.File, r.Bytes, r.Error, r.ID)
	return err
}

func (s *Store) DeleteRecording(ctx context.Context, id int64) error {
	_, err := s.w.ExecContext(ctx, `DELETE FROM recording WHERE id = ?`, id)
	return err
}

// OverlappingRecording returns a non-final recording of the same channel
// overlapping [start, stop), if any.
func (s *Store) OverlappingRecording(ctx context.Context, groupKey string, start, stop int64) (*Recording, error) {
	r, err := scanRecording(s.r.QueryRowContext(ctx, `SELECT `+recordingColumns+` FROM recording
		WHERE group_key = ? AND status IN ('scheduled', 'recording') AND start < ? AND stop > ? LIMIT 1`, groupKey, stop, start))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &r, err
}

// ChannelForGroup returns the item that currently represents a channel
// (its primary visible variant), falling back to any available variant.
func (s *Store) ChannelForGroup(ctx context.Context, groupKey string) (PlaybackItem, error) {
	var id int64
	err := s.r.QueryRowContext(ctx, `SELECT id FROM item WHERE kind = 'live' AND group_key = ? AND available = 1
		ORDER BY primary_variant DESC, visible DESC, quality DESC, id LIMIT 1`, groupKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return PlaybackItem{}, ErrNotFound
	}
	if err != nil {
		return PlaybackItem{}, err
	}
	return s.PlaybackItem(ctx, id)
}

// GroupKeyOf returns the variant group of an item.
func (s *Store) GroupKeyOf(ctx context.Context, itemID int64) (string, error) {
	var g string
	err := s.r.QueryRowContext(ctx, `SELECT group_key FROM item WHERE id = ?`, itemID).Scan(&g)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return g, err
}

// ActiveRecordingsBetween lists scheduled or running recordings that
// overlap [start, stop), with the subscription of their channel.
func (s *Store) ActiveRecordingsBetween(ctx context.Context, start, stop int64) ([]Recording, []int64, error) {
	rows, err := s.r.QueryContext(ctx, `SELECT `+recordingColumns+`, coalesce((SELECT i.subscription_id FROM item i WHERE i.id = recording.item_id), 0)
		FROM recording WHERE status IN ('scheduled', 'recording') AND start < ? AND stop > ? ORDER BY start`, stop, start)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var list []Recording
	var subs []int64
	for rows.Next() {
		var r Recording
		var sub int64
		if err := rows.Scan(append(r.scanTargets(), &sub)...); err != nil {
			return nil, nil, err
		}
		list = append(list, r)
		subs = append(subs, sub)
	}
	return list, subs, rows.Err()
}

// RecordingCovers returns the state of a recording of the channel that
// overlaps [start, stop), cancelled ones too ("" if none): rules do not
// record a programme twice, nor again after the user cancelled it.
func (s *Store) RecordingCovers(ctx context.Context, groupKey string, start, stop int64) (string, error) {
	var status string
	err := s.r.QueryRowContext(ctx, `SELECT status FROM recording WHERE group_key = ? AND start < ? AND stop > ?
		ORDER BY status = 'cancelled' LIMIT 1`, groupKey, stop, start).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return status, err
}

// CancelRecording keeps a cancelled rule recording as a marker.
func (s *Store) CancelRecording(ctx context.Context, id int64) error {
	_, err := s.w.ExecContext(ctx, `UPDATE recording SET status = 'cancelled', file = '', bytes = 0 WHERE id = ?`, id)
	return err
}

// PurgeCancelled forgets cancelled markers of programmes long past.
func (s *Store) PurgeCancelled(ctx context.Context, before int64) error {
	_, err := s.w.ExecContext(ctx, `DELETE FROM recording WHERE status = 'cancelled' AND stop < ?`, before)
	return err
}

func (s *Store) SetRecordingProtected(ctx context.Context, id int64, protected bool) error {
	res, err := s.w.ExecContext(ctx, `UPDATE recording SET protected = ? WHERE id = ?`, protected, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CompletedRecordings lists finished recordings, oldest first.
func (s *Store) CompletedRecordings(ctx context.Context) ([]Recording, error) {
	return s.queryRecordings(ctx, `SELECT `+recordingColumns+` FROM recording WHERE status IN ('completed', 'failed') ORDER BY stop`)
}
