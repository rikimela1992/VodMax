package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// The people of films and series (cast and director), read from their
// detail pages, so the assistant can find "a film with…" in the catalog.

// visibleTitles: the films and series people see in the catalog.
const visibleTitles = `i.kind IN ('movie', 'series') AND i.available = 1 AND i.visible = 1 AND i.primary_variant = 1`

// PutItemPeople stores the people of an item, already folded; "" records
// that its page had none.
func (s *Store) PutItemPeople(ctx context.Context, itemID int64, people string) error {
	_, err := s.w.ExecContext(ctx, `INSERT INTO item_people (item_id, people, checked_at) VALUES (?, ?, ?)
		ON CONFLICT (item_id) DO UPDATE SET people = excluded.people, checked_at = excluded.checked_at`, itemID, people, time.Now().Unix())
	return err
}

// ItemsWithPerson returns the visible films and series whose people have
// name (folded words, as a whole name or its end), best rated first.
func (s *Store) ItemsWithPerson(ctx context.Context, name string, noAdult bool, limit int) ([]CatalogItem, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(name)
	where := visibleTitles
	if noAdult {
		where += " AND i.adult = 0"
	}
	rows, err := s.r.QueryContext(ctx, `SELECT `+itemColumns+` FROM item_people p JOIN item i ON i.id = p.item_id
		WHERE (p.people LIKE ? ESCAPE '\' OR p.people LIKE ? ESCAPE '\') AND `+where+`
		ORDER BY coalesce(i.rating, 0) DESC, i.id LIMIT ?`, `% `+esc+` ,%`, `% `+esc+` `, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatalogItem
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ItemsWithoutPeople returns visible films and series of playlists that
// have detail pages (Xtream) and of local collections, whose people were
// never read or were missing before retryBefore.
func (s *Store) ItemsWithoutPeople(ctx context.Context, retryBefore time.Time, limit int) ([]MetaItem, error) {
	rows, err := s.r.QueryContext(ctx, `SELECT i.id, i.subscription_id, i.kind, i.source_id, coalesce(i.tmdb_id, ''), i.image
		FROM item i JOIN subscription s ON s.id = i.subscription_id LEFT JOIN item_people p ON p.item_id = i.id
		WHERE `+visibleTitles+` AND s.kind IN ('xtream', 'local') AND (p.item_id IS NULL OR (p.people = '' AND p.checked_at < ?))
		ORDER BY coalesce(i.rating, 0) DESC, i.id LIMIT ?`, retryBefore.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetaItem
	for rows.Next() {
		var m MetaItem
		if err := rows.Scan(&m.ID, &m.SubscriptionID, &m.Kind, &m.SourceID, &m.TMDBID, &m.Image); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PeopleCoverage says for how many visible films and series the people
// are known.
func (s *Store) PeopleCoverage(ctx context.Context) (known, total int, err error) {
	var k sql.NullInt64
	err = s.r.QueryRowContext(ctx, `SELECT count(*), sum(p.people <> '') FROM item i LEFT JOIN item_people p ON p.item_id = i.id
		WHERE `+visibleTitles).Scan(&total, &k)
	return int(k.Int64), total, err
}
