// VodMax Film e Serie: browsing with a remote control, playback with
// resume and "continue watching", recordings.
"use strict";

(() => {
  const { store, viewer, api, keyOf, clock, hhmm } = window.VodMax;
  const $ = (id) => document.getElementById(id);
  const el = (tag, props = {}, ...children) => {
    const e = Object.assign(document.createElement(tag), props);
    e.append(...children.filter((c) => c !== null && c !== undefined));
    return e;
  };
  if (!window.VodMax.token()) { location.href = "/"; return; }

  const state = { view: store.get("vodView", "home"), query: "", detail: null, play: null };
  let statusTimer;
  function flash(text) {
    $("status").textContent = text;
    $("status").hidden = false;
    clearTimeout(statusTimer);
    statusTimer = setTimeout(() => { $("status").hidden = true; }, 4000);
  }

  // ---- Cards ----
  function poster(item, title) {
    if (item.logo) {
      const img = el("img", { className: "poster", src: item.logo, alt: "", loading: "lazy" });
      img.onerror = () => img.replaceWith(el("div", { className: "poster" }, title));
      return img;
    }
    return el("div", { className: "poster" }, title);
  }

  function itemCard(item) {
    const title = item.display_title;
    const card = el("button", { className: "card focusable" }, poster(item, title),
      el("span", { className: "label" }, title),
      el("span", { className: "sub" }, [item.year, item.variant_count > 1 ? item.variant_count + " versioni" : ""].filter(Boolean).join(" · ")));
    const q = { 4: "4K", 3: "FHD" }[item.quality];
    if (q) card.append(el("span", { className: "badge" }, q));
    card.onclick = () => openDetail(item);
    return card;
  }

  function continueCard(c) {
    const title = c.media === "episode" ? c.series_title : c.title;
    const sub = c.media === "episode" ? "S" + c.season + "E" + c.number + " · " + c.title : "Mancano " + clock(c.progress.duration - c.progress.position);
    const pct = c.progress.duration ? (c.progress.position / c.progress.duration) * 100 : 0;
    const card = el("button", { className: "card wide focusable" }, poster({ logo: c.image }, title),
      el("div", { className: "bar" }, el("div", { style: "width:" + pct + "%" })),
      el("span", { className: "label" }, title), el("span", { className: "sub" }, sub));
    card.onclick = () => play(c.media, c.id, title + (c.media === "episode" ? " · " + sub : ""), c.progress.position);
    return card;
  }

  // ---- Views ----
  function setTabs() {
    for (const t of document.querySelectorAll(".tab[data-view]")) t.classList.toggle("active", t.dataset.view === state.view);
  }

  async function render() {
    setTabs();
    store.set("vodView", state.view);
    const main = $("content");
    main.replaceChildren(el("p", { className: "empty" }, "Caricamento…"));
    try {
      if (state.query) await renderSearch(main);
      else if (state.view === "home") await renderHome(main);
      else if (state.view === "recordings") await renderRecordings(main);
      else await renderGrid(main, state.view);
    } catch (e) {
      main.replaceChildren(el("p", { className: "empty" }, e.message));
    }
    focusFirst(main);
  }

  async function renderHome(main) {
    const [cont, films, series] = await Promise.all([
      api("GET", "/api/v1/continue"),
      api("GET", "/api/v1/catalog/movie?sort=recent&limit=24"),
      api("GET", "/api/v1/catalog/series?sort=recent&limit=24"),
    ]);
    const parts = [];
    if (cont.length) parts.push(el("h2", {}, "Continua a guardare"), el("div", { className: "row" }, ...cont.map(continueCard)));
    parts.push(el("h2", {}, "Nuovi film"), el("div", { className: "row" }, ...films.items.map(itemCard)));
    parts.push(el("h2", {}, "Nuove serie"), el("div", { className: "row" }, ...series.items.map(itemCard)));
    main.replaceChildren(...parts);
  }

  async function renderGrid(main, kind, cursor, grid) {
    const page = await api("GET", "/api/v1/catalog/" + kind + "?limit=60" + (cursor ? "&cursor=" + encodeURIComponent(cursor) : ""));
    if (!grid) {
      grid = el("div", { className: "grid" });
      main.replaceChildren(el("h2", {}, kind === "movie" ? "Film" : "Serie"), grid);
    }
    grid.append(...page.items.map(itemCard));
    main.querySelector(".more")?.remove();
    if (page.next) {
      const more = el("button", { className: "button secondary more focusable" }, "Altri titoli");
      more.onclick = async () => { more.remove(); await renderGrid(main, kind, page.next, grid); focusFirst(grid.lastElementChild); };
      main.append(more);
    }
  }

  async function renderSearch(main) {
    const q = encodeURIComponent(state.query);
    const [films, series] = await Promise.all([
      api("GET", "/api/v1/catalog/movie?limit=40&q=" + q), api("GET", "/api/v1/catalog/series?limit=40&q=" + q)]);
    const sections = [["Film", films.items], ["Serie", series.items]];
    if (state.view === "series") sections.reverse(); // the current tab's kind first
    const parts = [];
    for (const [label, items] of sections) {
      if (items.length) parts.push(el("h2", {}, label), el("div", { className: "grid" }, ...items.map(itemCard)));
    }
    if (!parts.length) parts.push(el("p", { className: "empty" }, "Nessun risultato per “" + state.query + "”."));
    main.replaceChildren(...parts);
  }

  const recState = { scheduled: "In programma", recording: "● In registrazione", completed: "Registrata", failed: "Non riuscita" };
  async function renderRecordings(main) {
    const list = await api("GET", "/api/v1/recordings");
    if (!list.length) {
      main.replaceChildren(el("p", { className: "empty" }, "Nessuna registrazione. In TV premi R (tasto blu) su un canale per registrare il programma in onda."));
      return;
    }
    const day = (t) => new Date(t * 1000).toLocaleDateString("it-IT", { weekday: "short", day: "numeric", month: "short" });
    main.replaceChildren(el("h2", {}, "Registrazioni"), el("p", { className: "empty" }, "OK guarda · Rosso (Canc) elimina"),
      el("ol", { className: "recs" }, ...list.map((r) => {
        const b = el("button", { className: "focusable" },
          el("span", {}, day(r.start) + " " + hhmm(r.start)),
          el("span", {}, el("strong", {}, r.title), " · " + r.channel),
          el("span", { className: "state " + r.status }, recState[r.status] || r.status),
          el("span", { className: "state" }, (r.bytes / 1e9).toFixed(2) + " GB"));
        b.onclick = () => r.status === "scheduled" ? flash("Non ancora iniziata") : playRecording(r);
        b.dataset.recording = r.id;
        return el("li", {}, b);
      })));
  }

  async function deleteRecording(id) {
    if (!confirm("Eliminare la registrazione?")) return;
    try { await api("DELETE", "/api/v1/recordings/" + id); flash("Registrazione eliminata"); render(); }
    catch (e) { flash(e.message); }
  }

  // ---- Detail ----
  async function openDetail(item) {
    state.detail = item;
    $("detail-title").textContent = item.display_title;
    $("detail-meta").textContent = [item.year, { 4: "4K", 3: "Full HD", 2: "HD", 1: "SD" }[item.quality],
      item.variant_count > 1 ? item.variant_count + " versioni" : "",
      item.rating ? "★ " + Number(item.rating).toLocaleString("it-IT", { maximumFractionDigits: 1 }) : ""].filter(Boolean).join(" · ");
    $("detail-image").hidden = !item.logo;
    if (item.logo) $("detail-image").src = item.logo;
    $("seasons").replaceChildren();
    $("episodes").replaceChildren();
    const actions = $("detail-actions");
    actions.replaceChildren();
    $("detail").hidden = false;
    if (item.kind === "series") {
      actions.append(el("p", { className: "empty" }, "Caricamento episodi…"));
      let eps;
      try { eps = await api("GET", "/api/v1/series/" + item.id + "/episodes"); }
      catch (e) { actions.replaceChildren(el("p", { className: "empty" }, e.message)); return; }
      actions.replaceChildren();
      renderSeasons(item, eps);
      return;
    }
    // Only the saved position: a playback session is created on "play",
    // because preparing it may already use the provider connection.
    const saved = await api("GET", "/api/v1/progress/movie/" + item.id).catch(() => null);
    const resume = saved && !saved.completed && saved.position > 30 ? saved : null;
    if (resume) {
      const r = el("button", { className: "button focusable" }, "Riprendi da " + clock(resume.position));
      r.onclick = () => play("movie", item.id, item.display_title, resume.position);
      actions.append(r);
    }
    const start = el("button", { className: "button focusable" + (resume ? " secondary" : "") }, resume ? "Dall'inizio" : "Guarda");
    start.onclick = () => play("movie", item.id, item.display_title, 0);
    actions.append(start);
    if (item.variant_count > 1) {
      const v = el("button", { className: "button secondary focusable" }, "Altre versioni");
      v.onclick = () => showVariants(item);
      actions.append(v);
    }
    focusFirst(actions);
  }

  async function showVariants(item) {
    const list = await api("GET", "/api/v1/items/" + item.id + "/variants");
    $("episodes").replaceChildren(...list.map((v) => {
      const b = el("button", { className: "focusable" }, el("span", { className: "n" }, { 4: "4K", 3: "FHD", 2: "HD", 1: "SD" }[v.quality] || ""),
        el("span", {}, v.title), el("span", {}));
      b.onclick = () => play("movie", v.id, v.display_title, 0);
      return el("li", {}, b);
    }));
    focusFirst($("episodes"));
  }

  function renderSeasons(item, eps) {
    const seasons = [...new Set(eps.map((e) => e.season))];
    const next = eps.find((e) => e.progress && !e.progress.completed) || eps.find((e) => !e.progress) || eps[0];
    const showSeason = (season) => {
      for (const b of $("seasons").children) b.classList.toggle("active", Number(b.dataset.season) === season);
      $("episodes").replaceChildren(...eps.filter((e) => e.season === season).map((e) => {
        const pct = e.progress ? (e.progress.completed ? 100 : (e.progress.position / (e.progress.duration || 1)) * 100) : 0;
        const b = el("button", { className: "focusable" }, el("span", { className: "n" }, e.number),
          el("span", {}, e.title || "Episodio " + e.number, e.duration ? el("span", { className: "empty" }, " · " + clock(e.duration)) : null),
          el("span", { className: "bar" }, el("div", { style: "width:" + pct + "%" })));
        b.onclick = () => play("episode", e.id, item.display_title + " · S" + e.season + "E" + e.number + " " + e.title,
          e.progress && !e.progress.completed ? e.progress.position : 0);
        return el("li", {}, b);
      }));
    };
    $("seasons").replaceChildren(...seasons.map((s) => {
      const b = el("button", { className: "tab focusable" }, "Stagione " + s);
      b.dataset.season = s;
      b.onclick = () => showSeason(s);
      return b;
    }));
    if (next) {
      const go = el("button", { className: "button focusable" },
        (next.progress && !next.progress.completed ? "Riprendi " : "Guarda ") + "S" + next.season + "E" + next.number);
      go.onclick = () => play("episode", next.id, item.display_title + " · S" + next.season + "E" + next.number + " " + next.title,
        next.progress && !next.progress.completed ? next.progress.position : 0);
      $("detail-actions").append(go);
      showSeason(next.season);
    }
    focusFirst($("detail-actions"));
  }

  function closeDetail() {
    $("detail").hidden = true;
    state.detail = null;
    focusFirst($("content"));
  }

  // ---- Player ----
  const video = $("video");
  let saveTimer, idleTimer;

  async function play(media, id, title, startAt, mode) {
    let sess;
    flash(mode === "remux" ? "Preparazione della conversione…" : "Avvio…");
    try {
      sess = await api("POST", "/api/v1/vod/" + media + "/" + id + "/session", { viewer, mode: mode || "" });
    } catch (e) { flash(e.message); return; }
    stopPlayback(false);
    state.play = { media, id, title, sess, offset: 0, duration: sess.duration || 0, fellBack: mode === "remux" };
    $("player-title").textContent = title || sess.title;
    $("player").hidden = false;
    load(startAt || 0);
    saveTimer = setInterval(saveProgress, 15000);
  }

  function load(position, force) {
    const p = state.play;
    const extra = force ? "force=1" : "";
    if (p.sess.mode === "remux") {
      p.offset = position;
      video.src = p.sess.url + "?t=" + Math.floor(position) + (extra ? "&" + extra : "");
    } else {
      p.offset = 0;
      video.src = p.sess.url + (extra ? "?" + extra : "");
      if (position > 0) video.addEventListener("loadedmetadata", () => { video.currentTime = position; }, { once: true });
    }
    video.play().catch(() => flash("Premi OK per avviare"));
    showBar();
  }

  function position() { return state.play ? state.play.offset + (video.currentTime || 0) : 0; }
  function duration() {
    const p = state.play;
    if (!p) return 0;
    return p.sess.mode === "direct" && isFinite(video.duration) ? video.duration : p.duration;
  }

  function seek(delta) {
    const p = state.play;
    if (!p) return;
    const target = Math.max(0, Math.min(position() + delta, (duration() || Infinity) - 5));
    if (p.sess.mode === "direct") video.currentTime = target;
    else load(target);
    showBar();
  }

  video.addEventListener("timeupdate", () => {
    const d = duration();
    $("player-pos").textContent = clock(position());
    $("player-dur").textContent = d ? clock(d) : "";
    $("player-fill").style.width = d ? Math.min(100, (position() / d) * 100) + "%" : "0";
  });

  video.addEventListener("error", async () => {
    const p = state.play;
    if (!p || p.recording) { if (p && p.recording) flash("Registrazione non riproducibile in questo browser"); return; }
    // Find out why: a busy connection answers 409.
    const status = await fetch(p.sess.url + (p.sess.mode === "remux" ? "?t=0" : ""), { headers: { Range: "bytes=0-0" } })
      .then((r) => { r.body && r.body.cancel(); return r.status; }).catch(() => 0);
    if (status === 409) {
      if (confirm("La connessione dell'abbonamento è in uso su un altro dispositivo. Prendere il controllo?")) load(position(), true);
      else stopPlayback(true);
      return;
    }
    if (p.sess.mode === "direct" && !p.fellBack) {
      flash("Formato non leggibile dal browser: conversione sul server…");
      play(p.media, p.id, p.title, position(), "remux");
      return;
    }
    flash("Riproduzione non riuscita");
  });

  video.addEventListener("ended", () => { saveProgress(); stopPlayback(true); });

  function saveProgress() {
    const p = state.play;
    if (!p || p.recording || !duration()) return;
    api("PUT", "/api/v1/progress/" + p.media + "/" + p.id, { position: Math.floor(position()), duration: Math.floor(duration()) })
      .catch(() => {});
  }

  function stopPlayback(save) {
    if (save) saveProgress();
    clearInterval(saveTimer);
    if (state.play && state.play.mpegts) { try { state.play.mpegts.destroy(); } catch { /* gone */ } }
    video.pause();
    video.removeAttribute("src");
    video.load();
    state.play = null;
    $("player").hidden = true;
  }

  function showBar() {
    $("player-bar").classList.remove("idle");
    clearTimeout(idleTimer);
    idleTimer = setTimeout(() => $("player-bar").classList.add("idle"), 4000);
  }

  async function playRecording(r) {
    let url;
    try { url = (await api("POST", "/api/v1/recordings/" + r.id + "/ticket")).url; } catch (e) { flash(e.message); return; }
    stopPlayback(false);
    state.play = { recording: r, title: r.title, sess: { mode: "direct", url }, offset: 0, duration: r.stop - r.start };
    $("player-title").textContent = r.title + " · " + r.channel;
    $("player").hidden = false;
    // Finished recordings are MP4 (seekable); recordings in progress, or
    // left in MPEG-TS, go through mpegts.js.
    const isTS = r.status === "recording" || !(await fetch(url, { headers: { Range: "bytes=0-0" } })
      .then((res) => { res.body && res.body.cancel(); return (res.headers.get("Content-Type") || "").includes("mp4"); }).catch(() => false));
    if (isTS && window.mpegts && mpegts.isSupported()) {
      const player = mpegts.createPlayer({ type: "mpegts", isLive: r.status === "recording", url }, { enableWorker: false });
      player.attachMediaElement(video);
      player.load();
      player.play();
      state.play.mpegts = player;
    } else {
      video.src = url;
      video.play().catch(() => flash("Premi OK per avviare"));
    }
    showBar();
  }

  // ---- Remote control navigation ----
  function focusables(root = document) {
    const layer = !$("player").hidden ? $("player") : !$("detail").hidden ? $("detail") : document;
    return [...(root === document ? layer : root).querySelectorAll(".focusable")].filter((e) => e.offsetParent !== null);
  }

  function focusFirst(root) {
    const list = root ? [...root.querySelectorAll(".focusable")].filter((e) => e.offsetParent !== null) : [];
    const target = root && root.classList && root.classList.contains("focusable") ? root : list[0];
    if (target) target.focus({ preventScroll: false });
  }

  // Moves focus to the nearest element in the arrow's direction.
  function moveFocus(dir) {
    const current = document.activeElement;
    const all = focusables();
    if (!current || !all.includes(current)) { all[0] && all[0].focus(); return; }
    const a = current.getBoundingClientRect();
    const ax = a.left + a.width / 2, ay = a.top + a.height / 2;
    let best = null, bestScore = Infinity;
    for (const e of all) {
      if (e === current) continue;
      const b = e.getBoundingClientRect();
      const bx = b.left + b.width / 2, by = b.top + b.height / 2;
      const dx = bx - ax, dy = by - ay;
      const ok = { ArrowLeft: dx < -4, ArrowRight: dx > 4, ArrowUp: dy < -4, ArrowDown: dy > 4 }[dir];
      if (!ok) continue;
      const along = dir === "ArrowLeft" || dir === "ArrowRight" ? Math.abs(dx) : Math.abs(dy);
      const across = dir === "ArrowLeft" || dir === "ArrowRight" ? Math.abs(dy) : Math.abs(dx);
      const score = along + across * 3;
      if (score < bestScore) { bestScore = score; best = e; }
    }
    if (best) best.focus();
  }

  document.addEventListener("keydown", (e) => {
    const key = keyOf(e);
    if (!$("player").hidden) {
      const handled = {
        OK: () => { video.paused ? video.play() : video.pause(); showBar(); },
        PlayPause: () => { video.paused ? video.play() : video.pause(); showBar(); },
        Play: () => video.play(), Pause: () => video.pause(),
        ArrowLeft: () => seek(-10), ArrowRight: () => seek(30), ArrowUp: showBar, ArrowDown: showBar,
        Back: () => stopPlayback(true), Stop: () => stopPlayback(true), Backspace: () => stopPlayback(true),
      }[key];
      if (handled) { handled(); e.preventDefault(); }
      return;
    }
    const typing = document.activeElement === $("search");
    if (typing && !["ArrowDown", "Back", "OK"].includes(key)) return;
    if (key.startsWith("Arrow")) { moveFocus(key); e.preventDefault(); return; }
    if (key === "OK" && typing) { $("search").blur(); focusFirst($("content")); e.preventDefault(); return; }
    if (key === "Back" || (key === "Backspace" && !typing)) {
      if (!$("detail").hidden) closeDetail();
      else if (state.query) { state.query = ""; $("search").value = ""; render(); }
      e.preventDefault();
      return;
    }
    if (key === "Green" || key === "g") { location.href = "/"; return; }
    if ((key === "Red" || key === "Delete") && document.activeElement && document.activeElement.dataset.recording) {
      deleteRecording(document.activeElement.dataset.recording);
      e.preventDefault();
    }
  });

  for (const t of document.querySelectorAll(".tab[data-view]")) {
    t.onclick = () => { state.view = t.dataset.view; state.query = ""; $("search").value = ""; closeDetailSilently(); render(); };
  }
  function closeDetailSilently() { $("detail").hidden = true; state.detail = null; }

  let searchTimer;
  $("search").addEventListener("input", () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => { state.query = $("search").value.trim(); render().then(() => $("search").focus()); }, 350);
  });

  addEventListener("pagehide", () => saveProgress());
  render();
})();
