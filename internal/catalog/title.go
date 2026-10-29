// Package catalog turns provider playlists into catalog items.
package catalog

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	trailingTags = regexp.MustCompile(`(?i)(?:\s*[-–|]?\s*(?:\[[^\[\]]*\]|\((?:JP|US|UK|IT|KR|FR|DE|ES|CN)\)|` +
		`\b(?:4K|UHD|FHD|HD|SD|HDR|HEVC|H\.?26[45]|2160p|1080p|720p|ITA|SUB(?:\s*ITA)?)\b))+\s*$`)
	trailingYear = regexp.MustCompile(`(?:\s*\((\d{4})\)|\s+[-–]\s*(\d{4}))\s*$`)
	edgeJunk     = regexp.MustCompile(`[\s\-–|]+$`)
	leadingJunk  = regexp.MustCompile(`^[\s"',;]+`)
	separator    = regexp.MustCompile(`^\s*[-=#*_~+]{2,}.*[-=#*_~+]{2,}\s*$`)
)

// ChannelTitle trims stray punctuation that some providers leave at the
// start of channel names (e.g. "\",US: ESPN").
func ChannelTitle(raw string) string {
	if t := strings.TrimSpace(leadingJunk.ReplaceAllString(raw, "")); t != "" {
		return t
	}
	return strings.TrimSpace(raw)
}

// IsSeparator reports fake channels used by providers as list headings,
// such as "----Italia----" or "===== CINEMA =====".
func IsSeparator(name string) bool {
	return separator.MatchString(name)
}

// CleanTitle separates the display title from quality tags and a delimited
// release year: "1408 (2007) 4K" → ("1408", 2007). Leading brackets
// ("[REC]") and bare numbers ("Blade Runner 2049", "1917") are kept.
// Metadata services such as TMDB only match the clean title.
func CleanTitle(raw string) (string, int) {
	clean, year := strings.TrimSpace(raw), 0
	for range 3 {
		clean = strings.TrimSpace(trailingTags.ReplaceAllString(clean, ""))
		m := trailingYear.FindStringSubmatchIndex(clean)
		if m == nil || year != 0 || m[0] == 0 {
			break
		}
		var digits string
		if m[2] >= 0 {
			digits = clean[m[2]:m[3]]
		} else {
			digits = clean[m[4]:m[5]]
		}
		if y, _ := strconv.Atoi(digits); y >= 1900 && y <= 2100 {
			year, clean = y, strings.TrimSpace(clean[:m[0]])
			continue
		}
		break
	}
	clean = strings.TrimSpace(edgeJunk.ReplaceAllString(clean, ""))
	if clean == "" {
		return strings.TrimSpace(raw), year
	}
	return clean, year
}
