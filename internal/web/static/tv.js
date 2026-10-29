// VodMax TV: live channels driven by a remote control, like a set-top box.
//
// P+/P-           change channel
// Up/Down         browse what is on other channels without changing (OK tunes)
// OK              channel list; Left/Right switch list
// Yellow / F      add or remove favourite
// Red / P         zap through favourites only
// digits          go to channel number
// Info / I        show the banner
// Green / G       menu (films, series, recordings)
// Blue / R        record the programme on air
// Back            close / cancel; with nothing open, back to the menu
"use strict";

(() => {
  const $ = (id) => document.getElementById(id);
  const store = {
    get(key, fallback) {
      try { const v = localStorage.getItem("vodmax." + key); return v === null ? fallback : JSON.parse(v); }
      catch { return fallback; }
    },
    set(key, value) { try { localStorage.setItem("vodmax." + key, JSON.stringify(value)); } catch { /* private mode */ } },
  };

  // ---- Authentication: admin token for now (pairing comes later) ----
  const fromHash = new URLSearchParams(location.hash.slice(1)).get("token");
  if (fromHash) {
    store.set("token", fromHash);
    history.replaceState(null, "", location.pathname);
  }
  let token = store.get("token", "");
  const viewer = store.get("viewer", "") || (() => {
    const id = "v" + Array.from(crypto.getRandomValues(new Uint8Array(12)), (b) => b.toString(16).padStart(2, "0")).join("");
    store.set("viewer", id);
    return id;
  })();

  async function api(method, path, body) {
    const res = await fetch(path, {
      method,
      headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (res.status === 401) {
      showSetup();
      throw new Error("token non valido");
    }
    const data = res.status === 204 ? null : await res.json();
    if (!res.ok) throw Object.assign(new Error(data && data.error ? data.error : "errore " + res.status), { status: res.status });
    return data;
  }

  // ---- State ----
  const LISTS = [
    { id: "all", label: "Tutti", section: "tv", favorites: false },
    { id: "favorites", label: "Preferiti", section: "", favorites: true },
    { id: "sport", label: "Sport", section: "sport", favorites: false },
    { id: "radio", label: "Radio", section: "radio", favorites: false },
  ];
  const state = {
    channels: [],          // the zapping list
    listIndex: store.get("list", 0),
    favoritesOnly: store.get("favoritesOnly", false),
    current: null,         // channel being watched
    browse: null,          // index shown in the banner while browsing
    overlay: null,         // { list, channels, selected }
    player: null,
    digits: "",
    timers: {},
    retries: {},           // network retries per channel
  };

  function zapList() { return state.favoritesOnly ? LISTS[1] : LISTS[state.listIndex === 1 ? 0 : state.listIndex]; }

  async function load(list) {
    const q = new URLSearchParams();
    if (list.section) q.set("section", list.section);
    if (list.favorites) q.set("list", "favorites");
    return api("GET", "/api/v1/tv/channels?" + q);
  }

  async function refreshChannels() {
    state.channels = await load(zapList());
    if (state.favoritesOnly && state.channels.length === 0) {
      state.favoritesOnly = false;
      store.set("favoritesOnly", false);
      flash("Nessun preferito: elenco completo");
      state.channels = await load(zapList());
    }
    if (state.current) {
      const fresh = state.channels.find((c) => c.id === state.current.id);
      if (fresh) state.current = fresh;
    }
  }

  // ---- Playback ----
  // One provider connection per zap: the player opens the relay directly
  // and a refused stream (409, connection busy) is reported as an error.
  async function tune(channel, force) {
    if (!channel) return;
    state.current = channel;
    store.set("last", channel.id);
    showBanner(channel, false);
    stopPlayer();
    let url;
    try {
      url = (await api("POST", "/api/v1/live/" + channel.id + "/ticket", { viewer })).url;
    } catch (e) {
      flash(e.message);
      return;
    }
    if (state.current !== channel) return; // zapped meanwhile
    if (!window.mpegts || !mpegts.isSupported()) {
      flash("Questo dispositivo non supporta la riproduzione MPEG-TS nel browser");
      return;
    }
    const player = mpegts.createPlayer({ type: "mpegts", isLive: true, url: url + (force ? "?force=1" : "") }, {
      enableWorker: false,
      enableStashBuffer: false,
      liveBufferLatencyChasing: true,
      liveBufferLatencyMaxLatency: 3.0,
      liveBufferLatencyMinRemain: 0.8,
      lazyLoad: false,
    });
    player.attachMediaElement($("video"));
    player.on(mpegts.Events.ERROR, (type, detail, info) => {
      if (state.player !== player) return;
      stopPlayer();
      const code = info && info.code;
      if (code === 409) {
        confirmDialog("Tutte le connessioni dell'abbonamento sono in uso su un altro dispositivo.\n\nPrendere il controllo?",
          () => tune(channel, true));
        return;
      }
      // Only network failures are worth retrying; a format the device
      // cannot decode would just reopen the channel at the provider.
      const attempt = (state.retries[channel.id] || 0) + 1;
      if (type !== mpegts.ErrorTypes.NETWORK_ERROR || attempt > 3) {
        flash(type === mpegts.ErrorTypes.MEDIA_ERROR
          ? "Questo dispositivo non riesce a decodificare il canale (" + detail + ")"
          : "Canale non disponibile (" + (code || detail) + ")");
        return;
      }
      state.retries[channel.id] = attempt;
      flash("Flusso interrotto: nuovo tentativo " + attempt + " di 3…");
      state.timers.retry = setTimeout(() => state.current === channel && tune(channel), 1500 * attempt);
    });
    player.on(mpegts.Events.MEDIA_INFO, () => { delete state.retries[channel.id]; });
    player.load();
    const started = player.play();
    if (started && started.catch) started.catch(() => flash("Premi un tasto o clicca per avviare la riproduzione"));
    state.player = player;
  }

  function stopPlayer() {
    clearTimeout(state.timers.retry);
    if (state.player) {
      try { state.player.pause(); state.player.unload(); state.player.detachMediaElement(); state.player.destroy(); } catch { /* already gone */ }
      state.player = null;
    }
  }

  function step(delta) {
    const list = state.channels;
    if (list.length === 0) return;
    let i = list.findIndex((c) => state.current && c.id === state.current.id);
    i = i < 0 ? 0 : (i + delta + list.length) % list.length;
    tune(list[i]);
  }

  // ---- Banner and browsing ----
  const pad = (n) => String(n).padStart(2, "0");
  const hhmm = (t) => { const d = new Date(t * 1000); return pad(d.getHours()) + ":" + pad(d.getMinutes()); };

  function showBanner(channel, browsing) {
    if (!channel) return;
    const banner = $("banner");
    banner.classList.toggle("browsing", browsing);
    $("banner-number").textContent = channel.number;
    $("banner-name").textContent = channel.name;
    const logo = $("banner-logo");
    logo.hidden = !channel.logo;
    if (channel.logo) logo.src = channel.logo;
    const badges = $("banner-badges");
    badges.replaceChildren();
    const quality = { 4: "4K", 3: "FHD", 2: "HD", 1: "SD" }[channel.quality];
    for (const [text, cls] of [[quality, ""], [channel.favorite ? "★ Preferito" : "", "fav"],
      [channel.catchup_days ? "Replay " + channel.catchup_days + "g" : "", ""]]) {
      if (!text) continue;
      const span = document.createElement("span");
      span.textContent = text;
      if (cls) span.className = cls;
      badges.append(span);
    }
    $("banner-mode").textContent = state.favoritesOnly ? "Solo preferiti" : "";
    $("banner-mode").hidden = !state.favoritesOnly;
    const now = channel.now, next = channel.next;
    $("banner-now").textContent = now ? now.title : "Nessuna informazione sul programma";
    $("banner-now-time").textContent = now ? hhmm(now.start) + " – " + hhmm(now.stop) : "";
    const pct = now ? Math.min(100, Math.max(0, (Date.now() / 1000 - now.start) / (now.stop - now.start) * 100)) : 0;
    $("banner-progress").style.width = pct + "%";
    $("banner-next").textContent = next ? next.title : "";
    $("banner-next-time").textContent = next ? hhmm(next.start) : "";
    $("banner-hint").textContent = browsing ? "OK per guardare · Indietro per tornare a " + state.current.name : "";
    banner.hidden = false;
    clearTimeout(state.timers.banner);
    state.timers.banner = setTimeout(endBrowse, browsing ? 8000 : 5000);
  }

  function endBrowse() {
    state.browse = null;
    $("banner").hidden = true;
  }

  function browseStep(delta) {
    const list = state.channels;
    if (list.length === 0) return;
    if (state.browse === null) {
      const i = list.findIndex((c) => state.current && c.id === state.current.id);
      state.browse = i < 0 ? 0 : i;
    }
    state.browse = (state.browse + delta + list.length) % list.length;
    showBanner(list[state.browse], true);
  }

  // ---- Channel list overlay ----
  async function openList(listIndex) {
    const list = LISTS[listIndex];
    let channels;
    try { channels = await load(list); } catch (e) { flash(e.message); return; }
    let selected = channels.findIndex((c) => state.current && c.id === state.current.id);
    state.overlay = { listIndex, channels, selected: selected < 0 ? 0 : selected };
    renderList();
    $("list").hidden = false;
    $("banner").hidden = true;
  }

  function renderList() {
    const o = state.overlay;
    const tabs = $("list-tabs");
    tabs.replaceChildren(...LISTS.map((l, i) => {
      const s = document.createElement("span");
      s.textContent = l.label;
      if (i === o.listIndex) s.className = "active";
      return s;
    }));
    const ol = $("list-items");
    if (o.channels.length === 0) {
      const li = document.createElement("li");
      li.textContent = o.listIndex === 1 ? "Nessun preferito: premi Giallo su un canale per aggiungerlo." : "Nessun canale.";
      ol.replaceChildren(li);
      return;
    }
    // Render a window around the selection: lists can hold hundreds of channels.
    const rows = Math.max(5, Math.floor((window.innerHeight * 0.8) / 64));
    const start = Math.max(0, Math.min(o.selected - Math.floor(rows / 2), o.channels.length - rows));
    const items = o.channels.slice(start, start + rows).map((c, k) => {
      const li = document.createElement("li");
      if (start + k === o.selected) li.classList.add("selected");
      if (state.current && c.id === state.current.id) li.classList.add("current");
      const n = document.createElement("span"); n.className = "n"; n.textContent = c.number;
      const logo = document.createElement("img"); logo.className = "l"; logo.alt = "";
      if (c.logo) { logo.src = c.logo; logo.onerror = () => { logo.style.visibility = "hidden"; }; } else logo.style.visibility = "hidden";
      const name = document.createElement("span"); name.className = "c"; name.textContent = c.name;
      if (c.favorite) { const st = document.createElement("span"); st.className = "star"; st.textContent = "★"; name.append(st); }
      const p = document.createElement("span"); p.className = "p";
      p.textContent = c.now ? hhmm(c.now.start) + " " + c.now.title + (c.next ? "  ·  " + hhmm(c.next.start) + " " + c.next.title : "") : "";
      li.append(n, logo, name, p);
      return li;
    });
    ol.replaceChildren(...items);
  }

  function closeList() {
    state.overlay = null;
    $("list").hidden = true;
  }

  // ---- Favourites ----
  async function toggleFavorite(channel) {
    if (!channel) return;
    try {
      await api("PUT", "/api/v1/tv/favorites/" + channel.id, { favorite: !channel.favorite });
    } catch (e) { flash(e.message); return; }
    channel.favorite = !channel.favorite;
    flash(channel.favorite ? "Aggiunto ai preferiti: " + channel.name : "Tolto dai preferiti: " + channel.name);
    await refreshChannels();
    if (state.overlay) {
      const keep = state.overlay.channels[state.overlay.selected];
      state.overlay.channels = await load(LISTS[state.overlay.listIndex]);
      const i = state.overlay.channels.findIndex((c) => keep && c.id === keep.id);
      state.overlay.selected = Math.max(0, Math.min(i < 0 ? state.overlay.selected : i, state.overlay.channels.length - 1));
      renderList();
    }
  }

  async function toggleFavoritesOnly() {
    state.favoritesOnly = !state.favoritesOnly;
    store.set("favoritesOnly", state.favoritesOnly);
    await refreshChannels();
    flash(state.favoritesOnly ? "P+/P− scorrono solo i preferiti" : "P+/P− scorrono tutti i canali");
  }

  // ---- Recording ----
  async function record(channel) {
    if (!channel) return;
    try {
      const r = await api("POST", "/api/v1/recordings", { channel_id: channel.id });
      flash("Registrazione di “" + r.title + "” fino alle " + hhmm(r.stop) + " (Verde → Registrazioni)");
    } catch (e) { flash(e.message); }
  }

  // ---- Direct number entry ----
  function digit(d) {
    state.digits = (state.digits + d).slice(-4);
    $("digits").textContent = state.digits;
    $("digits").hidden = false;
    clearTimeout(state.timers.digits);
    state.timers.digits = setTimeout(commitDigits, 1500);
  }

  async function commitDigits() {
    const n = parseInt(state.digits, 10);
    state.digits = "";
    $("digits").hidden = true;
    clearTimeout(state.timers.digits);
    if (!n) return;
    let channel = state.channels.find((c) => c.number === n);
    if (!channel) {
      const all = await load({ section: "", favorites: false }).catch(() => []);
      channel = all.find((c) => c.number === n);
    }
    if (channel) tune(channel); else flash("Canale " + n + " non presente");
  }

  // ---- Messages ----
  function flash(text) {
    const el = $("status");
    el.textContent = text;
    el.hidden = false;
    clearTimeout(state.timers.status);
    state.timers.status = setTimeout(() => { el.hidden = true; }, 3500);
  }

  let dialogAction = null;
  function confirmDialog(text, action) {
    $("dialog-text").textContent = text;
    $("dialog").hidden = false;
    dialogAction = action;
  }

  // ---- Keys: browser, LG webOS and Android TV ----
  function keyOf(e) {
    const byCode = { 33: "ChannelUp", 34: "ChannelDown", 403: "Red", 404: "Green", 405: "Yellow", 406: "Blue",
      457: "Info", 461: "Back", 427: "ChannelUp", 428: "ChannelDown" };
    if (byCode[e.keyCode]) return byCode[e.keyCode];
    const byKey = { PageUp: "ChannelUp", PageDown: "ChannelDown", "+": "ChannelUp", "-": "ChannelDown",
      ColorF0Red: "Red", ColorF1Green: "Green", ColorF2Yellow: "Yellow", ColorF3Blue: "Blue",
      Escape: "Back", Backspace: "Back", BrowserBack: "Back", GoBack: "Back", Enter: "OK", " ": "OK",
      f: "Yellow", F: "Yellow", p: "Red", P: "Red", i: "Info", I: "Info", g: "Green", G: "Green", r: "Blue", R: "Blue" };
    return byKey[e.key] || e.key;
  }

  // Browsers block autoplay with sound until the user interacts: any key
  // or click resumes a paused channel.
  function resumeIfBlocked() {
    const video = $("video");
    if (state.player && video.paused) video.play().catch(() => {});
  }
  document.addEventListener("click", resumeIfBlocked);

  document.addEventListener("keydown", (e) => {
    if (!$("setup").hidden) return;
    resumeIfBlocked();
    const key = keyOf(e);
    let handled = true;

    if (!$("dialog").hidden) {
      if (key === "OK") { $("dialog").hidden = true; const a = dialogAction; dialogAction = null; a && a(); }
      else if (key === "Back") { $("dialog").hidden = true; dialogAction = null; }
      e.preventDefault();
      return;
    }

    if (/^[0-9]$/.test(key)) { digit(key); e.preventDefault(); return; }
    if (state.digits && key === "OK") { commitDigits(); e.preventDefault(); return; }

    const o = state.overlay;
    if (o) {
      switch (key) {
        case "ArrowUp": o.selected = Math.max(0, o.selected - 1); renderList(); break;
        case "ArrowDown": o.selected = Math.min(o.channels.length - 1, o.selected + 1); renderList(); break;
        case "ChannelUp": o.selected = Math.max(0, o.selected - 10); renderList(); break;
        case "ChannelDown": o.selected = Math.min(o.channels.length - 1, o.selected + 10); renderList(); break;
        case "ArrowLeft": openList((o.listIndex + LISTS.length - 1) % LISTS.length); break;
        case "ArrowRight": openList((o.listIndex + 1) % LISTS.length); break;
        case "OK": {
          const c = o.channels[o.selected];
          // Zapping continues in the list the channel was picked from.
          if (o.listIndex === 1) { state.favoritesOnly = true; }
          else { state.favoritesOnly = false; state.listIndex = o.listIndex; }
          store.set("favoritesOnly", state.favoritesOnly);
          store.set("list", state.listIndex);
          closeList();
          refreshChannels().then(() => tune(c));
          break;
        }
        case "Yellow": toggleFavorite(o.channels[o.selected]); break;
        case "Blue": record(o.channels[o.selected]); break;
        case "Back": closeList(); break;
        default: handled = false;
      }
    } else {
      switch (key) {
        // P+ goes to the next number up, as on a set-top box.
        case "ChannelUp": endBrowse(); step(1); break;
        case "ChannelDown": endBrowse(); step(-1); break;
        case "ArrowUp": browseStep(-1); break;
        case "ArrowDown": browseStep(1); break;
        case "OK":
          if (state.browse !== null) { const c = state.channels[state.browse]; endBrowse(); tune(c); }
          else openList(state.favoritesOnly ? 1 : state.listIndex);
          break;
        case "Back":
          // Closes the banner first; with nothing open, back to the menu.
          if (state.browse !== null || !$("banner").hidden) endBrowse();
          else { stopPlayer(); location.href = "/"; }
          break;
        case "Yellow": toggleFavorite(state.browse !== null ? state.channels[state.browse] : state.current); break;
        case "Red": toggleFavoritesOnly(); break;
        case "Green": stopPlayer(); location.href = "/"; break;
        case "Blue": record(state.browse !== null ? state.channels[state.browse] : state.current); break;
        case "Info": showBanner(state.current, false); break;
        default: handled = false;
      }
    }
    if (handled) e.preventDefault();
  });

  $("banner-logo").onerror = () => { $("banner-logo").hidden = true; };

  // Keep "now / next" fresh.
  setInterval(() => { refreshChannels().catch(() => {}); }, 60000);

  // ---- Start ----
  function showSetup() {
    $("setup").hidden = false;
    $("setup-token").focus();
  }

  $("setup-form").addEventListener("submit", (e) => {
    e.preventDefault();
    token = $("setup-token").value.trim();
    store.set("token", token);
    $("setup").hidden = true;
    start();
  });

  async function start() {
    if (!token) { showSetup(); return; }
    try { await refreshChannels(); } catch (e) { if (e.message !== "token non valido") flash(e.message); return; }
    if (state.channels.length === 0) { flash("Nessun canale: sincronizza un abbonamento"); return; }
    const last = store.get("last", null);
    tune(state.channels.find((c) => c.id === last) || state.channels[0]);
  }

  start();
})();
