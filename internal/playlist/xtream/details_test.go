package xtream

import (
	"encoding/json"
	"testing"
)

func TestParseDetailsLoose(t *testing.T) {
	d := ParseDetails(json.RawMessage(`{"plot":"Un uomo...","cast":"A, B","director":"C","genre":"Dramma","releasedate":"2007-06-22",
		"rating":"7.2","duration_secs":6240,"youtube_trailer":"https://www.youtube.com/watch?v=abcDEF12345","tmdb_id":3021,
		"backdrop_path":["http://img/b1.jpg",""],"movie_image":"http://img/p.jpg","o_name":"1408"}`))
	if d.Plot != "Un uomo..." || d.Rating != 7.2 || d.DurationSecs != 6240 || d.Trailer != "abcDEF12345" || d.TMDB != "3021" ||
		len(d.Backdrops) != 1 || d.Poster != "http://img/p.jpg" || d.OriginalTitle != "1408" || d.Released != "2007-06-22" {
		t.Fatalf("%+v", d)
	}
	// Series panels: minutes, a single backdrop string, "info": [] when missing.
	s := ParseDetails(json.RawMessage(`{"plot":"","cast":"N/A","episode_run_time":"45","backdrop_path":"http://img/b.jpg","youtube_trailer":"<script>"}`))
	if s.Plot != "" || s.Cast != "" || s.DurationSecs != 2700 || len(s.Backdrops) != 1 || s.Trailer != "" {
		t.Fatalf("%+v", s)
	}
	if e := ParseDetails(json.RawMessage(`[]`)); e.Plot != "" || e.Backdrops != nil {
		t.Fatalf("%+v", e)
	}
}
