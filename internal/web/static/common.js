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
    if (!token()) { askToken(); return; }
    start();
    watchStatus();
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

  // ---- MPEG-TS playback (live channels, recordings in progress) ----
  //
  // mpegts.js feeds the browser's video element through Media Source
  // Extensions. Live streams can leave small gaps in the buffer (audio and
  // video starting at different times, a reconnect) where the element
  // stalls without any error; aggressive latency chasing makes it worse by
  // seeking into such gaps. So: a larger start-up buffer, no chasing, and
  // a watchdog that steps over gaps and resumes a stall the user did not
  // ask for.
  function tsPlayer(video, url, { live, onError, onStart } = {}) {
    const player = mpegts.createPlayer({ type: "mpegts", isLive: live, url }, {
      enableWorker: false,
      enableStashBuffer: true,
      stashInitialSize: 384 * 1024,
      liveBufferLatencyChasing: false,
      autoCleanupSourceBuffer: true,
      lazyLoad: false,
    });
    let userPaused = false, lastTime = -1, stuck = 0, destroyed = false;
    player.attachMediaElement(video);
    player.on(mpegts.Events.ERROR, (type, detail, info) => { if (!destroyed && onError) onError(type, detail, info); });
    player.on(mpegts.Events.MEDIA_INFO, () => { if (!destroyed && onStart) onStart(); });
    player.load();
    const tryPlay = () => { const p = video.play(); if (p && p.catch) p.catch(() => {}); };
    tryPlay();

    const watchdog = setInterval(() => {
      if (destroyed || userPaused) return;
      const b = video.buffered;
      if (!b.length) return;
      const t = video.currentTime;
      // Before the first buffered range (start-up): jump in.
      if (t < b.start(0) - 0.05) { video.currentTime = b.start(0) + 0.05; return; }
      if (video.paused) { tryPlay(); return; } // paused by the browser, not by the user
      if (Math.abs(t - lastTime) < 0.01) stuck++; else stuck = 0;
      lastTime = t;
      if (stuck < 2) return;
      // Stalled: move to the next buffered data, or nudge forward.
      for (let i = 0; i < b.length; i++) {
        if (b.start(i) > t + 0.01) { video.currentTime = b.start(i) + 0.05; stuck = 0; return; }
      }
      if (b.end(b.length - 1) - t > 0.5) { video.currentTime = t + 0.3; stuck = 0; }
    }, 1000);

    return {
      pause() { userPaused = true; video.pause(); },
      play() { userPaused = false; stuck = 0; tryPlay(); },
      toggle() { if (userPaused || video.paused) this.play(); else this.pause(); },
      // Browsers block autoplay with sound until the user interacts; a key
      // or click resumes such a pause, never one the user asked for.
      resumeIfBlocked() { if (!userPaused && video.paused) tryPlay(); },
      get paused() { return userPaused || video.paused; },
      // Seconds behind the newest buffered data (live time-shift).
      behind() { const b = video.buffered; return b.length ? Math.max(0, b.end(b.length - 1) - video.currentTime) : 0; },
      goLive() { const b = video.buffered; if (b.length) video.currentTime = Math.max(b.start(b.length - 1), b.end(b.length - 1) - 1.5); this.play(); },
      seek(delta) {
        const b = video.buffered;
        if (!b.length) return;
        video.currentTime = Math.min(Math.max(video.currentTime + delta, b.start(0) + 0.1), b.end(b.length - 1) - 0.5);
      },
      destroy() {
        destroyed = true;
        clearInterval(watchdog);
        try { player.pause(); player.unload(); player.detachMediaElement(); player.destroy(); } catch { /* already gone */ }
      },
    };
  }

  // ---- Server problems (VPN down, failed sync, expiring account) ----
  // A small notice in a corner, on every page, linking to the settings.
  let statusWatched = false;
  function watchStatus() {
    if (statusWatched) return;
    statusWatched = true;
    const check = async () => {
      let problems = [];
      try { problems = (await api("GET", "/api/v1/status")).problems; } catch { return; }
      let box = document.getElementById("server-problems");
      if (!problems.length) { if (box) box.remove(); return; }
      if (!box) {
        box = Object.assign(document.createElement("a"), { id: "server-problems", href: "/settings.html?view=system" });
        document.body.append(box);
      }
      const error = problems.some((p) => p.level === "error");
      box.className = error ? "error" : "warning";
      box.textContent = "⚠ " + problems[0].text + (problems.length > 1 ? " (e altri " + (problems.length - 1) + ")" : "");
    };
    check();
    setInterval(check, 30000);
  }

  return { store, viewer, token, withToken, askToken, api, keyOf, clock, hhmm, tsPlayer, watchStatus };
})();
