// Package api exposes the administration and catalog HTTP API (/api/v1).
// Every endpoint except /api/v1/health requires the admin token.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rikimela1992/vodmax/internal/catalog"
	"github.com/rikimela1992/vodmax/internal/service"
	"github.com/rikimela1992/vodmax/internal/store"
	"github.com/rikimela1992/vodmax/internal/web"
)

// Handler builds the router. token is the admin bearer token.
func Handler(svc *service.Service, token string, log *slog.Logger) http.Handler {
	a := &api{svc: svc, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/vpn", a.listVPN)
	protected.HandleFunc("POST /api/v1/vpn", a.addVPN)
	protected.HandleFunc("POST /api/v1/vpn/{id}/connect", a.connectVPN)
	protected.HandleFunc("POST /api/v1/vpn/{id}/disconnect", a.disconnectVPN)
	protected.HandleFunc("DELETE /api/v1/vpn/{id}", a.deleteVPN)
	protected.HandleFunc("GET /api/v1/subscriptions", a.listSubscriptions)
	protected.HandleFunc("POST /api/v1/subscriptions", a.addSubscription)
	protected.HandleFunc("POST /api/v1/subscriptions/{id}/sync", a.syncSubscription)
	protected.HandleFunc("DELETE /api/v1/subscriptions/{id}", a.deleteSubscription)
	protected.HandleFunc("GET /api/v1/subscriptions/{id}/rules", a.getRules)
	protected.HandleFunc("PUT /api/v1/subscriptions/{id}/rules", a.putRules)
	protected.HandleFunc("POST /api/v1/subscriptions/{id}/rules/preview", a.previewRules)
	protected.HandleFunc("GET /api/v1/subscriptions/{id}/selection", a.selectionStats)
	protected.HandleFunc("GET /api/v1/subscriptions/{id}/categories", a.selectionCategories)
	protected.HandleFunc("PUT /api/v1/subscriptions/{id}/categories/{kind}/{category}", a.putCategory)
	protected.HandleFunc("PUT /api/v1/items/{id}/hidden", a.putHidden)
	protected.HandleFunc("GET /api/v1/items/{id}/variants", a.variants)
	protected.HandleFunc("GET /api/v1/catalog/{kind}", a.catalog)
	protected.HandleFunc("GET /api/v1/categories/{kind}", a.categories)
	protected.HandleFunc("GET /api/v1/tv/channels", a.tvChannels)
	protected.HandleFunc("GET /api/v1/tv/channels/{id}/guide", a.channelGuide)
	protected.HandleFunc("PUT /api/v1/tv/favorites/{id}", a.putFavorite)
	protected.HandleFunc("POST /api/v1/subscriptions/{id}/guide/sync", a.syncGuide)
	protected.HandleFunc("POST /api/v1/live/{id}/ticket", a.liveTicket)
	mux.Handle("/api/", requireToken(token, protected))
	// The stream is authorised by its single-use ticket: video players
	// cannot send an Authorization header.
	mux.HandleFunc("GET /stream/live/{ticket}", a.liveStream)
	mux.Handle("/", web.Handler())
	return mux
}

type api struct {
	svc *service.Service
	log *slog.Logger
}

func requireToken(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token di amministrazione mancante o errato"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	var bad service.ErrInvalid
	status := http.StatusInternalServerError
	switch {
	case errors.As(err, &bad):
		status = http.StatusBadRequest
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, store.ErrInUse), errors.Is(err, service.ErrBusy), errors.Is(err, store.ErrSuspiciousDrop),
		errors.As(err, new(service.ConnectionBusy)):
		status = http.StatusConflict
	case errors.Is(err, service.ErrTicket):
		status = http.StatusForbidden
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}
	if status == http.StatusInternalServerError {
		a.log.Error("richiesta fallita", "path", r.URL.Path, "err", err)
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return service.ErrInvalid{Msg: "corpo della richiesta non valido: " + err.Error()}
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, service.ErrInvalid{Msg: "id non valido"}
	}
	return id, nil
}

func (a *api) listVPN(w http.ResponseWriter, r *http.Request) {
	list, err := a.svc.VPNs(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) addVPN(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name   string `json:"name"`
		Config string `json:"config"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	id, err := a.svc.AddVPN(r.Context(), in.Name, in.Config)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (a *api) connectVPN(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	st, err := a.svc.ConnectVPN(ctx, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (a *api) disconnectVPN(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.svc.DisconnectVPN(id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) deleteVPN(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = a.svc.DeleteVPN(r.Context(), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	list, err := a.svc.Subscriptions(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) addSubscription(w http.ResponseWriter, r *http.Request) {
	var in service.SubscriptionInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	id, err := a.svc.AddSubscription(r.Context(), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (a *api) syncSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// The sync outlives a dropped client: the catalog must not be left
	// half-fetched because a browser tab closed.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Minute)
	defer cancel()
	res, err := a.svc.Sync(ctx, id, r.URL.Query().Get("allow_drop") == "true")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *api) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = a.svc.DeleteSubscription(r.Context(), id)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) catalog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := store.Query{
		Kind: r.PathValue("kind"), Text: q.Get("q"), CategoryID: q.Get("category"),
		Sort: q.Get("sort"), Section: q.Get("section"), All: q.Get("all") == "true", Cursor: q.Get("cursor"),
	}
	query.Limit, _ = strconv.Atoi(q.Get("limit"))
	query.SubscriptionID, _ = strconv.ParseInt(q.Get("subscription"), 10, 64)
	if len(query.Text) > 200 {
		query.Text = query.Text[:200]
	}
	page, err := a.svc.Catalog(r.Context(), query)
	if err != nil {
		if strings.Contains(err.Error(), "non valido") {
			err = service.ErrInvalid{Msg: err.Error()}
		}
		a.fail(w, r, err)
		return
	}
	if page.Items == nil {
		page.Items = []store.CatalogItem{}
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *api) categories(w http.ResponseWriter, r *http.Request) {
	sub, _ := strconv.ParseInt(r.URL.Query().Get("subscription"), 10, 64)
	list, err := a.svc.Categories(r.Context(), r.PathValue("kind"), sub)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if list == nil {
		list = []store.Category{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) getRules(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	rules, err := a.svc.Rules(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if rules.Countries == nil {
		rules.Countries = []string{}
	}
	writeJSON(w, http.StatusOK, rules)
}

func (a *api) putRules(w http.ResponseWriter, r *http.Request) {
	a.rules(w, r, a.svc.SetRules)
}

func (a *api) previewRules(w http.ResponseWriter, r *http.Request) {
	a.rules(w, r, a.svc.PreviewRules)
}

func (a *api) rules(w http.ResponseWriter, r *http.Request, apply func(context.Context, int64, catalog.Rules) (catalog.Stats, error)) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var rules catalog.Rules
	if err := decode(r, &rules); err != nil {
		a.fail(w, r, err)
		return
	}
	stats, err := apply(r.Context(), id, rules)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *api) selectionStats(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	stats, err := a.svc.SubscriptionStats(r.Context(), id, nil)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *api) selectionCategories(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	list, err := a.svc.SelectionCategories(r.Context(), id, r.URL.Query().Get("kind"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) putCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var o store.CategoryOverride
	if err := decode(r, &o); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.svc.SetCategoryOverride(r.Context(), id, r.PathValue("kind"), r.PathValue("category"), o); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) putHidden(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		Hidden bool `json:"hidden"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.svc.SetItemHidden(r.Context(), id, in.Hidden); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) variants(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	list, err := a.svc.Variants(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) tvChannels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := a.svc.TVChannels(r.Context(), q.Get("section"), q.Get("list") == "favorites")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) channelGuide(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	list, err := a.svc.ChannelGuide(r.Context(), id, hours)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) putFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		Favorite bool `json:"favorite"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.svc.SetFavorite(r.Context(), id, in.Favorite); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) syncGuide(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Minute)
	defer cancel()
	res, err := a.svc.SyncGuide(ctx, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *api) liveTicket(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		Viewer string `json:"viewer"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	t, err := a.svc.LiveTicket(r.Context(), id, in.Viewer)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": "/stream/live/" + t})
}

func (a *api) liveStream(w http.ResponseWriter, r *http.Request) {
	stream, err := a.svc.OpenLive(r.Context(), r.PathValue("ticket"), r.URL.Query().Get("force") == "1")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", stream.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 64<<10)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // viewer gone
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
