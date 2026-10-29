// Shared by the VodMax pages: settings in localStorage, the admin token
// (from a link or asked on screen), the player id and API calls.
"use strict";

window.VodMax = (() => {
  const store = {
    get(key, fallback) {
      try { const v = localStorage.getItem("vodmax." + key); return v === null ? fallback : JSON.parse(v); }
      catch { return fallback; }
    },
    set(key, value) { try { localStorage.setItem("vodmax." + key, JSON.stringify(value)); } catch { /* private mode */ } },
  };

  const viewer = store.get("viewer", "") || (() => {
    const id = "v" + Array.from(crypto.getRandomValues(new Uint8Array(12)), (b) => b.toString(16).padStart(2, "0")).join("");
    store.set("viewer", id);
    return id;
  })();

  // A link may carry the token once: "/#token=…". It is saved and removed
  // from the address bar.
  function tokenFromLink() {
    const t = new URLSearchParams(location.hash.slice(1)).get("token");
    if (t) {
      store.set("token", t);
      history.replaceState(null, "", location.pathname + location.search);
    }
    return Boolean(t);
  }
  tokenFromLink();
  // Pasting a new link in the same tab only changes the hash: no reload.
  addEventListener("hashchange", () => { if (tokenFromLink()) location.reload(); });

  function token() { return store.get("token", ""); }

  // Asks for the admin token when it is missing or refused, on any page.
  function askToken(message) {
    if (document.getElementById("token-setup")) return;
    const input = Object.assign(document.createElement("input"), { type: "password", autocomplete: "off", spellcheck: false });
    input.setAttribute("aria-label", "Token di amministrazione");
    const form = document.createElement("form");
    const button = Object.assign(document.createElement("button"), { type: "submit", textContent: "Continua" });
    form.append(input, button);
    const box = Object.assign(document.createElement("section"), { id: "token-setup", className: "panel setup-panel" });
    const title = Object.assign(document.createElement("h1"), { textContent: "VodMax" });
    const text = Object.assign(document.createElement("p"), { textContent: message || "Inserisci il token di amministrazione del server (file admin.token)." });
    box.append(title, text, form);
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      store.set("token", input.value.trim());
      location.reload();
    });
    document.body.append(box);
    input.focus();
  }

  // Runs start() when a token is available, otherwise asks for it.
  function withToken(start) {
    if (token()) start(); else askToken();
  }

  async function api(method, path, body) {
    const res = await fetch(path, {
      method,
      headers: { Authorization: "Bearer " + token(), "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (res.status === 401) {
      askToken("Il token salvato non è valido (forse è di un'altra installazione): inseriscilo di nuovo.");
      throw new Error("token non valido");
    }
    const data = res.status === 204 ? null : await res.json().catch(() => null);
    if (!res.ok) throw Object.assign(new Error(data && data.error ? data.error : "errore " + res.status), { status: res.status });
    return data;
  }

  // Remote control keys of browsers, LG webOS and Android TV.
  function keyOf(e) {
    const byCode = { 33: "ChannelUp", 34: "ChannelDown", 403: "Red", 404: "Green", 405: "Yellow", 406: "Blue",
      457: "Info", 461: "Back", 427: "ChannelUp", 428: "ChannelDown", 415: "Play", 19: "Pause", 413: "Stop" };
    if (byCode[e.keyCode]) return byCode[e.keyCode];
    const byKey = { PageUp: "ChannelUp", PageDown: "ChannelDown", ColorF0Red: "Red", ColorF1Green: "Green",
      ColorF2Yellow: "Yellow", ColorF3Blue: "Blue", Escape: "Back", BrowserBack: "Back", GoBack: "Back", Enter: "OK",
      MediaPlayPause: "PlayPause", MediaPlay: "Play", MediaPause: "Pause", MediaStop: "Stop" };
    return byKey[e.key] || e.key;
  }

  const pad = (n) => String(n).padStart(2, "0");
  function clock(seconds) {
    seconds = Math.max(0, Math.floor(seconds || 0));
    const h = Math.floor(seconds / 3600), m = Math.floor((seconds % 3600) / 60), s = seconds % 60;
    return (h ? h + ":" + pad(m) : m) + ":" + pad(s);
  }
  function hhmm(unix) { const d = new Date(unix * 1000); return pad(d.getHours()) + ":" + pad(d.getMinutes()); }

  return { store, viewer, token, withToken, askToken, api, keyOf, clock, hhmm };
})();
