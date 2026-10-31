package api

import (
	"net/http"

	"github.com/silirim/vodmax/internal/service"
)

// The assistant (a language model chosen by the admin).

func (a *api) assistantView(w http.ResponseWriter, r *http.Request) {
	s, key, err := a.svc.AssistantSettings(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": s.Enabled, "url": s.URL, "model": s.Model, "has_key": key})
}

func (a *api) putAssistant(w http.ResponseWriter, r *http.Request) {
	var in service.AssistantInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.svc.SetAssistant(r.Context(), in); err != nil {
		a.fail(w, r, err)
		return
	}
	a.assistantView(w, r)
}

func (a *api) testAssistant(w http.ResponseWriter, r *http.Request) {
	answer, took, err := a.svc.TestAssistant(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"answer": answer, "seconds": took.Seconds()})
}

func (a *api) assistantOn(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": a.svc.AssistantOn(r.Context())})
}

func (a *api) assistantRule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.svc.AssistantRule(r.Context(), in.Text)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *api) assistantTonight(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Wish string `json:"wish"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	out, err := a.svc.AssistantTonight(r.Context(), in.Wish)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
