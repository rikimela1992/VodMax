package service

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Messages for people are written in Italian, as format strings; other
// languages translate those format strings in i18n/<lang>.json (it.json is
// the base that translators see on Weblate). A message without a
// translation stays in Italian.

//go:embed i18n/*.json
var messageFiles embed.FS

// Languages are the languages of the messages, Italian first.
var Languages = []string{"it"}

var catalogs = map[string]map[string]string{}

func init() {
	files, _ := fs.Glob(messageFiles, "i18n/*.json")
	for _, f := range files {
		code := strings.TrimSuffix(path.Base(f), ".json")
		if code == "it" {
			continue
		}
		raw, _ := messageFiles.ReadFile(f)
		m := map[string]string{}
		if err := json.Unmarshal(raw, &m); err != nil {
			panic(f + ": " + err.Error())
		}
		for k, v := range m {
			if v == "" { // not translated yet
				delete(m, k)
			}
		}
		catalogs[code] = m
		Languages = append(Languages, code)
	}
}

type langKey struct{}

// WithLang attaches the language of a request ("it", "en"...).
func WithLang(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, langKey{}, lang)
}

// LangOf is the request's language; Italian for internal work.
func LangOf(ctx context.Context) string {
	if l, ok := ctx.Value(langKey{}).(string); ok {
		return l
	}
	return "it"
}

// PickLang chooses a supported language from a preference such as
// "en", "en-GB" or an Accept-Language header; "" if none fits.
func PickLang(pref string) string {
	for _, part := range strings.Split(pref, ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		for _, l := range Languages {
			if base == l {
				return l
			}
		}
	}
	return ""
}

// tr formats an Italian message in lang.
func tr(lang, format string, args ...any) string {
	if f, ok := catalogs[lang][format]; ok {
		format = f
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// T formats an Italian message in the request's language.
func T(ctx context.Context, format string, args ...any) string {
	return tr(LangOf(ctx), format, args...)
}

// localizer is an error whose message is built from its fields.
type localizer interface{ localized(lang string) string }

// Localize returns the message of err for people speaking lang. A known
// whole text is translated; otherwise the outermost translatable error of
// the chain (input errors, errors with fields, known messages) has its
// text replaced, and the rest (e.g. a "guida: " prefix) stays in Italian.
func Localize(lang string, err error) string {
	msg := err.Error()
	if f, ok := catalogs[lang][msg]; ok {
		return f
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		var text string
		switch v := e.(type) {
		case ErrInvalid:
			if v.format == "" {
				continue
			}
			args := slices.Clone(v.args)
			for i, a := range args {
				if inner, ok := a.(error); ok {
					args[i] = Localize(lang, inner)
				}
			}
			text = tr(lang, v.format, args...)
		case localizer:
			text = v.localized(lang)
		default:
			f, ok := catalogs[lang][e.Error()]
			if !ok {
				continue
			}
			text = f
		}
		return strings.Replace(msg, e.Error(), text, 1)
	}
	return msg
}
