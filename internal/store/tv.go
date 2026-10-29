package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// GuideChannel and GuideProgramme are what ReplaceGuide stores.
type GuideChannel struct {
	ID      string
	NameKey string
}

type GuideProgramme struct {
	Channel     string
	Start, Stop int64
	Title       string
	Subtitle    string
	Description string
	Category    string
}

// ReplaceGuide makes channels and programmes the guide of a subscription.
// Programmes of the given channels are replaced as a whole.
func (s *Store) ReplaceGuide(ctx context.Context, subID int64, channels []GuideChannel, programmes []GuideProgramme) error {
	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM epg_programme WHERE channel IN (SELECT id FROM epg_channel WHERE subscription_id = ?)`, subID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM epg_channel WHERE subscription_id = ?`, subID); err != nil {
		return err
	}
	for _, c := range channels {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO epg_channel VALUES (?, ?, ?)`, subID, c.ID, c.NameKey); err != nil {
			return err
		}
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO epg_programme VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range programmes {
		if _, err := stmt.ExecContext(ctx, p.Channel, p.Start, p.Stop, p.Title, p.Subtitle, p.Description, p.Category); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Programme is a guide entry as returned to clients.
type Programme struct {
	Start       int64  `json:"start"`
	Stop        int64  `json:"stop"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// TVChannel is one entry of the channel list: the primary variant of a
// channel, its number and what is on.
type TVChannel struct {
	ID           int64      `json:"id"`
	Number       int        `json:"number"`
	Official     bool       `json:"official"` // number from the national numbering
	Name         string     `json:"name"`
	Image        string     `json:"image,omitempty"` // provider URL: cleared before reaching clients
	Logo         string     `json:"logo,omitempty"`  // signed local URL
	Section      string     `json:"section"`
	Quality      int        `json:"quality"`
	VariantCount int        `json:"variant_count"`
	Favorite     bool       `json:"favorite"`
	CatchupDays  int        `json:"catchup_days,omitempty"`
	Now          *Programme `json:"now,omitempty"`
	Next         *Programme `json:"next,omitempty"`

	GroupKey string `json:"-"`
	GuideID  string `json:"-"`
}

// FirstFreeNumber is where channels without an official number start.
const FirstFreeNumber = 100

// TVChannels lists channels in remote-control order: officially numbered
// channels first, then the others (TV, sport, radio, each by name) numbered
// from FirstFreeNumber. Numbers are computed over all channels, so a
// channel keeps its number in every section and in the favourites.
func (s *Store) TVChannels(ctx context.Context, profileID int64, noAdult bool, section string, favoritesOnly bool, now time.Time) ([]TVChannel, error) {
	rows, err := s.r.QueryContext(ctx, `
		SELECT i.id, coalesce(i.lcn, 0), i.display_title, i.image, i.section, i.quality, i.variant_count, i.catchup_days,
			i.group_key, f.position IS NOT NULL, coalesce(f.position, 0),
			coalesce((SELECT c.id FROM epg_channel c WHERE c.id = lower(i.epg_id) LIMIT 1),
			         (SELECT c.id FROM epg_channel c WHERE c.name_key = i.name_key AND i.name_key <> '' LIMIT 1), '')
		FROM item i
		LEFT JOIN favorite f ON f.profile_id = ? AND f.kind = 'live' AND f.group_key = i.group_key
		WHERE i.kind = 'live' AND i.available = 1 AND i.visible = 1 AND i.primary_variant = 1 AND (? = 0 OR i.adult = 0)
		ORDER BY i.lcn IS NULL, i.lcn,
			CASE i.section WHEN 'tv' THEN 0 WHEN 'sport' THEN 1 ELSE 2 END,
			i.display_title COLLATE NOCASE, i.id`, profileID, noAdult)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []TVChannel
	positions := map[int64]int{}
	next := FirstFreeNumber
	used := map[int]bool{}
	for rows.Next() {
		var c TVChannel
		var pos int
		if err := rows.Scan(&c.ID, &c.Number, &c.Name, &c.Image, &c.Section, &c.Quality, &c.VariantCount, &c.CatchupDays,
			&c.GroupKey, &c.Favorite, &pos, &c.GuideID); err != nil {
			return nil, err
		}
		// Two variants groups can match the same official number (e.g.
		// "Rai Sport" and "Rai Sport +"): the first keeps it.
		if c.Number > 0 && !used[c.Number] {
			c.Official = true
			used[c.Number] = true
		} else {
			c.Number = 0
		}
		if c.Number == 0 {
			for used[next] {
				next++
			}
			c.Number = next
			used[next] = true
		}
		positions[c.ID] = pos
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []TVChannel
	for _, c := range all {
		if (section == "" || c.Section == section) && (!favoritesOnly || c.Favorite) {
			out = append(out, c)
		}
	}
	if favoritesOnly {
		sortByPosition(out, positions)
	}
	return out, s.attachNowNext(ctx, out, now)
}

func sortByPosition(list []TVChannel, pos map[int64]int) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && pos[list[j].ID] < pos[list[j-1].ID]; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func (s *Store) attachNowNext(ctx context.Context, list []TVChannel, now time.Time) error {
	byGuide := map[string][]int{}
	for i, c := range list {
		if c.GuideID != "" {
			byGuide[c.GuideID] = append(byGuide[c.GuideID], i)
		}
	}
	if len(byGuide) == 0 {
		return nil
	}
	keys := make([]any, 0, len(byGuide)+2)
	keys = append(keys, now.Unix(), now.Add(12*time.Hour).Unix())
	for k := range byGuide {
		keys = append(keys, k)
	}
	rows, err := s.r.QueryContext(ctx, `SELECT channel, start, stop, title, subtitle, category FROM epg_programme
		WHERE stop > ? AND start < ? AND channel IN (?`+strings.Repeat(",?", len(byGuide)-1)+`)
		ORDER BY channel, start`, keys...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ch string
		var p Programme
		if err := rows.Scan(&ch, &p.Start, &p.Stop, &p.Title, &p.Subtitle, &p.Category); err != nil {
			return err
		}
		for _, i := range byGuide[ch] {
			c := &list[i]
			switch {
			case c.Now == nil && p.Start <= now.Unix():
				pc := p
				c.Now = &pc
			case c.Next == nil && p.Start > now.Unix():
				pc := p
				c.Next = &pc
			}
		}
	}
	return rows.Err()
}

// ChannelGuide returns the programmes of a channel between from and to.
func (s *Store) ChannelGuide(ctx context.Context, itemID int64, from, to time.Time) ([]Programme, error) {
	var guide string
	err := s.r.QueryRowContext(ctx, `SELECT
			coalesce((SELECT c.id FROM epg_channel c WHERE c.id = lower(i.epg_id) LIMIT 1),
			         (SELECT c.id FROM epg_channel c WHERE c.name_key = i.name_key AND i.name_key <> '' LIMIT 1), '')
		FROM item i WHERE i.id = ? AND i.kind = 'live'`, itemID).Scan(&guide)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil || guide == "" {
		return []Programme{}, err
	}
	rows, err := s.r.QueryContext(ctx, `SELECT start, stop, title, subtitle, description, category FROM epg_programme
		WHERE channel = ? AND stop > ? AND start < ? ORDER BY start`, guide, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Programme{}
	for rows.Next() {
		var p Programme
		if err := rows.Scan(&p.Start, &p.Stop, &p.Title, &p.Subtitle, &p.Description, &p.Category); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GuideChannelKeys lists the guide channel ids and name keys of the
// visible channels, so the guide import keeps only what can be shown.
func (s *Store) GuideChannelKeys(ctx context.Context) (ids, names map[string]bool, err error) {
	rows, err := s.r.QueryContext(ctx, `SELECT lower(epg_id), name_key FROM item WHERE kind = 'live' AND available = 1 AND visible = 1`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	ids, names = map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		if id != "" {
			ids[id] = true
		}
		if name != "" {
			names[name] = true
		}
	}
	return ids, names, rows.Err()
}

// SetFavorite adds or removes a channel (by its group of variants) from a
// profile's favourites; new favourites go to the end.
func (s *Store) SetFavorite(ctx context.Context, profileID, itemID int64, favorite bool) error {
	var kind, group string
	err := s.r.QueryRowContext(ctx, `SELECT kind, group_key FROM item WHERE id = ?`, itemID).Scan(&kind, &group)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !favorite {
		_, err = s.w.ExecContext(ctx, `DELETE FROM favorite WHERE profile_id = ? AND kind = ? AND group_key = ?`, profileID, kind, group)
		return err
	}
	_, err = s.w.ExecContext(ctx, `INSERT OR IGNORE INTO favorite VALUES (?, ?, ?,
		(SELECT coalesce(max(position), 0) + 1 FROM favorite WHERE profile_id = ? AND kind = ?), ?)`,
		profileID, kind, group, profileID, kind, time.Now().Unix())
	return err
}

// CachedImage is an image fetched from a provider. An empty ContentType
// records a failed fetch, so it is not retried at every request.
type CachedImage struct {
	ContentType string
	Data        []byte
	FetchedAt   time.Time
}

func (s *Store) CachedImage(ctx context.Context, urlHash string) (CachedImage, bool, error) {
	var img CachedImage
	var at int64
	err := s.r.QueryRowContext(ctx, `SELECT content_type, data, fetched_at FROM image_cache WHERE url_hash = ?`, urlHash).
		Scan(&img.ContentType, &img.Data, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return img, false, nil
	}
	img.FetchedAt = time.Unix(at, 0)
	return img, err == nil, err
}

func (s *Store) PutCachedImage(ctx context.Context, urlHash string, img CachedImage) error {
	_, err := s.w.ExecContext(ctx, `INSERT OR REPLACE INTO image_cache VALUES (?, ?, ?, ?)`,
		urlHash, img.ContentType, img.Data, time.Now().Unix())
	return err
}

// ItemImage returns the provider image URL and subscription of an item.
func (s *Store) ItemImage(ctx context.Context, itemID int64) (string, int64, error) {
	var url string
	var sub int64
	err := s.r.QueryRowContext(ctx, `SELECT image, subscription_id FROM item WHERE id = ?`, itemID).Scan(&url, &sub)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	return url, sub, err
}

// GuideWindow returns the programmes of the given guide channels that
// overlap [from, to), by guide channel.
func (s *Store) GuideWindow(ctx context.Context, guides []string, from, to time.Time) (map[string][]Programme, error) {
	out := map[string][]Programme{}
	if len(guides) == 0 {
		return out, nil
	}
	// One query per batch: SQLite limits bound parameters.
	for len(guides) > 0 {
		n := min(len(guides), 400)
		batch := guides[:n]
		guides = guides[n:]
		args := []any{from.Unix(), to.Unix()}
		for _, g := range batch {
			args = append(args, g)
		}
		rows, err := s.r.QueryContext(ctx, `SELECT channel, start, stop, title, subtitle, description, category FROM epg_programme
			WHERE stop > ? AND start < ? AND channel IN (?`+strings.Repeat(", ?", n-1)+`) ORDER BY channel, start`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ch string
			var p Programme
			if err := rows.Scan(&ch, &p.Start, &p.Stop, &p.Title, &p.Subtitle, &p.Description, &p.Category); err != nil {
				rows.Close()
				return nil, err
			}
			out[ch] = append(out[ch], p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}
