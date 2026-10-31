package api

import (
	"net/http"

	"github.com/silirim/vodmax/internal/service"
)

// Video conversion with the graphics card.

func (a *api) getTranscode(w http.ResponseWriter, r *http.Request) {
	t, err := a.svc.TranscodeSettings(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	opts, devices := a.svc.TranscodeOptions(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"accel": t.Accel, "device": t.Device, "options": opts, "devices": devices})
}

func (a *api) putTranscode(w http.ResponseWriter, r *http.Request) {
	var in service.TranscodeSettings
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err := a.svc.SetTranscode(r.Context(), in); err != nil {
		a.fail(w, r, err)
		return
	}
	a.getTranscode(w, r)
}

func (a *api) testTranscode(w http.ResponseWriter, r *http.Request) {
	var in service.TranscodeSettings
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	took, err := a.svc.TestTranscode(r.Context(), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"seconds": took.Seconds()})
}
