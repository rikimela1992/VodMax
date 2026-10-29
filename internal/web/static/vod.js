// VodMax home: films, series, recordings and the way to TV, browsed with a
// remote control; playback with resume and "continue watching".
"use strict";

(() => {
  const { store, viewer, api, keyOf, clock, hhmm } = window.VodMax;
  const $ = (id) => document.getElementById(id);
  const el = (tag, props = {}, ...children) => {
    const e = Object.assign(document.createElement(tag), props);
    e.append(...children.filter((c) => c !== null && c !== undefined));
    return e;
  };

  const askedView = new URLSearchParams(location.search).get("view");
  const state = { view: ["home", "movie", "series", "recordings", "tv"].includes(askedView) ? askedView : store.get("vodView", "home"), query: "", detail: null, play: null };
  if (askedView) history.replaceState(null, "", "/");
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
    card.hero = heroFromItem(item);
    return card;
  }

  function continueCard(c) {
    const title = c.media === "episode" ? c.series_title : c.title;
    const ep = "S" + c.season + "E" + c.number + " · " + c.title;
    const sub = c.next ? "Prossima puntata · " + ep : c.media === "episode" ? ep : "Mancano " + clock(c.progress.duration - c.progress.position);
    const pct = c.next ? 0 : c.progress.duration ? (c.progress.position / c.progress.duration) * 100 : 0;
    const card = el("button", { className: "card wide focusable" }, poster({ logo: c.image }, title),
      el("div", { className: "bar" }, el("div", { style: "width:" + pct + "%" })),
      el("span", { className: "label" }, title), el("span", { className: "sub" }, sub));
    card.onclick = () => play(c.media, c.id, title + (c.media === "episode" ? " · " + ep : ""), c.next ? 0 : c.progress.position);
    card.hero = heroFromContinue(c);
    return card;
  }

  // ---- Views ----
  function setTabs() {
    for (const t of document.querySelectorAll(".tab[data-view]")) t.classList.toggle("active", t.dataset.view === state.view);
    window.VodMax.bottomNav(state.view === "recordings" ? "more" : state.view);
  }

  async function render() {
    setTabs();
    $("search").placeholder = state.view === "tv" ? "Cerca un canale (nome o numero)" : "Cerca film e serie";
    store.set("vodView", state.view);
    const main = $("content");
    main.replaceChildren(el("p", { className: "empty" }, "Caricamento…"));
    try {
      if (state.view === "tv") await renderTV(main);
      else if (state.query) await renderSearch(main);
      else if (state.view === "home") await renderHome(main);
      else if (state.view === "recordings") await renderRecordings(main);
      else await renderGrid(main, state.view);
    } catch (e) {
      main.replaceChildren(el("p", { className: "empty" }, e.message));
    }
    focusFirst(main);
  }

  // ---- TV: the channel list, then full screen ----
  const TV_LISTS = [["", "Tutti"], ["tv", "TV"], ["sport", "Sport"], ["radio", "Radio"], ["favorites", "Preferiti"]];
  async function renderTV(main) {
    const query = state.query.toLowerCase();
    const list = store.get("tvList", "");
    // A search looks in every channel, whatever the list.
    const q = query ? "" : list === "favorites" ? "?list=favorites" : list ? "?section=" + list : "";
    let channels = await api("GET", "/api/v1/tv/channels" + q);
    if (query) channels = channels.filter((c) => c.name.toLowerCase().includes(query) || String(c.number) === query);
    const now = Date.now() / 1000;
    const chips = TV_LISTS.map(([id, label]) => {
      const b = el("button", { className: "chip focusable" + (id === list && !query ? " active" : "") }, label);
      b.onclick = () => { store.set("tvList", id); state.query = ""; $("search").value = ""; render(); };
      return b;
    });
    const guide = el("a", { className: "chip focusable", href: "/guide.html" }, "Guida TV");
    const items = channels.slice(0, 600).map((c) => {
      const p = c.now;
      const pct = p ? Math.min(100, Math.max(0, ((now - p.start) / (p.stop - p.start)) * 100)) : 0;
      const b = el("button", { className: "focusable" }, el("span", { className: "num" }, c.number || ""),
        c.logo ? el("img", { src: c.logo, alt: "", loading: "lazy" }) : el("span", { className: "nologo" }),
        el("span", { className: "what" }, el("strong", {}, (c.favorite ? "★ " : "") + c.name),
          el("span", {}, p ? hhmm(p.start) + " " + p.title : "Nessuna informazione"),
          c.next ? el("span", {}, "Dopo: " + hhmm(c.next.start) + " " + c.next.title) : null,
          p ? el("div", { className: "bar" }, el("div", { style: "width:" + pct + "%" })) : null));
      b.onclick = () => { location.href = "/tv.html?channel=" + c.id; };
      return el("li", {}, b);
    });
    main.replaceChildren(el("h2", {}, "TV"), el("div", { className: "tv-tools" }, ...chips, guide),
      query ? el("p", { className: "empty" }, channels.length + " canali per “" + state.query + "” (nome o numero)") : "",
      items.length ? el("ol", { className: "channels" }, ...items) : el("p", { className: "empty" }, list === "favorites"
        ? "Nessun preferito: in TV premi il tasto giallo (F) su un canale." : "Nessun canale."));
  }

  // ---- Home: a featured title on top (it follows the focused card),
  // then rows. ----
  const detailCache = new Map();
  function detailsOf(id) {
    if (!detailCache.has(id)) detailCache.set(id, api("GET", "/api/v1/items/" + id + "/details").catch(() => ({})));
    return detailCache.get(id);
  }

  function skeletonRow(wide) {
    return el("div", { className: "row" }, ...Array.from({ length: 7 }, () => el("div", { className: "card" + (wide ? " wide" : "") },
      el("div", { className: "poster skeleton" }))));
  }

  async function renderHome(main) {
    main.replaceChildren(el("section", { className: "hero skeleton" }), el("h2", {}, "Continua a guardare"), skeletonRow(true),
      el("h2", {}, "Nuovi film"), skeletonRow(false));
    const [cont, films, series, favorites, recordings] = await Promise.all([
      api("GET", "/api/v1/continue"),
      api("GET", "/api/v1/catalog/movie?sort=recent&limit=24"),
      api("GET", "/api/v1/catalog/series?sort=recent&limit=24"),
      api("GET", "/api/v1/tv/channels?list=favorites").catch(() => []),
      api("GET", "/api/v1/recordings").catch(() => []),
    ]);
    const hero = el("section", { className: "hero" });
    const parts = [hero, el("h2", {}, "Continua a guardare")];
    parts.push(cont.length ? el("div", { className: "row" }, ...cont.map(continueCard))
      : el("p", { className: "empty" }, "Qui compaiono i film e gli episodi iniziati e la prossima puntata delle serie che stai seguendo."));
    if (favorites.length) parts.push(el("h2", {}, "In onda sui tuoi canali"), el("div", { className: "row" }, ...favorites.slice(0, 20).map(channelCard)));
    parts.push(el("h2", {}, "Nuovi film"), el("div", { className: "row" }, ...films.items.map(itemCard)));
    parts.push(el("h2", {}, "Nuove serie"), el("div", { className: "row" }, ...series.items.map(itemCard)));
    const done = recordings.filter((r) => r.status === "completed").slice(0, 12);
    if (done.length) parts.push(el("h2", {}, "Registrazioni recenti"), el("div", { className: "row" }, ...done.map(recordingCard)));
    main.replaceChildren(...parts);
    // The featured title: the first thing to continue, else the newest film.
    const first = cont[0] ? heroFromContinue(cont[0]) : films.items[0] ? heroFromItem(films.items[0]) : null;
    if (first) showHero(hero, first);
    let heroTimer;
    main.addEventListener("focusin", (e) => {
      const card = e.target.closest(".card");
      if (!card || !card.hero) return;
      clearTimeout(heroTimer);
      heroTimer = setTimeout(() => showHero(hero, card.hero), 350);
    });
  }

  function heroFromItem(item) {
    return { id: item.id, title: item.display_title, logo: item.logo,
      meta: [item.year, { 4: "4K", 3: "Full HD" }[item.quality], item.rating ? "★ " + Number(item.rating).toLocaleString("it-IT", { maximumFractionDigits: 1 }) : ""],
      actions: [["primary", "play", "Guarda", () => openDetail(item)], ["", "info", "Dettagli", () => openDetail(item)]] };
  }
  function heroFromContinue(c) {
    const episode = c.media === "episode";
    const title = episode ? c.series_title : c.title;
    const ep = episode ? "S" + c.season + "E" + c.number + " · " + c.title : "";
    return { id: episode ? c.series_id : c.id, title, logo: c.image,
      meta: [c.next ? "Prossima puntata" : episode ? "Continua" : "Mancano " + clock(c.progress.duration - c.progress.position), ep],
      actions: [["primary", "play", c.next ? "Guarda la puntata" : "Riprendi", () => play(c.media, c.id, title + (episode ? " · " + ep : ""), c.next ? 0 : c.progress.position)]] };
  }

  let heroShown = null;
  async function showHero(hero, h) {
    heroShown = h;
    const buttons = h.actions.map(([kind, icon, text, fn]) => {
      const b = el("button", { className: "button focusable " + kind });
      window.VodMax.label(b, icon, text);
      b.onclick = fn;
      return b;
    });
    const plot = el("p", { className: "hero-plot" });
    const art = el("div", { className: "hero-art" });
    hero.replaceChildren(art, el("div", { className: "hero-text" }, el("h1", {}, h.title),
      el("p", { className: "hero-meta" }, h.meta.filter(Boolean).join(" · ")), plot, el("div", { className: "hero-actions" }, ...buttons)));
    if (h.logo) art.style.backgroundImage = "url(" + JSON.stringify(h.logo) + ")";
    const d = await detailsOf(h.id);
    if (heroShown !== h) return;
    plot.textContent = d.plot || "";
    if (d.backdrop) {
      const img = new Image();
      img.onload = () => { if (heroShown === h) { art.style.backgroundImage = "url(" + JSON.stringify(d.backdrop) + ")"; art.classList.add("wide"); } };
      img.src = d.backdrop;
    }
  }

  function channelCard(c) {
    const p = c.now;
    const pct = p ? Math.min(100, Math.max(0, ((Date.now() / 1000 - p.start) / (p.stop - p.start)) * 100)) : 0;
    const art = el("div", { className: "poster channel-art" }, c.logo ? el("img", { src: c.logo, alt: "", loading: "lazy" }) : el("span", {}, c.name));
    const card = el("button", { className: "card wide focusable" }, art,
      p ? el("div", { className: "bar" }, el("div", { style: "width:" + pct + "%" })) : null,
      el("span", { className: "label" }, c.number + " · " + c.name), el("span", { className: "sub" }, p ? p.title : "Nessuna informazione"));
    card.onclick = () => { location.href = "/tv.html?channel=" + c.id; };
    return card;
  }

  function recordingCard(r) {
    const card = el("button", { className: "card wide focusable" }, el("div", { className: "poster rec-art" }, el("span", {}, r.channel)),
      el("span", { className: "label" }, r.title),
      el("span", { className: "sub" }, new Date(r.start * 1000).toLocaleDateString("it-IT", { weekday: "short", day: "numeric", month: "short" }) + " " + hhmm(r.start)));
    card.onclick = () => playRecording(r);
    return card;
  }


  // ---- Films and series: filters (order, rating, category), then a
  // grid that loads more as asked. ----
  const SORTS = [["recent", "Novità"], ["title", "A–Z"], ["rating", "Più votati"]];
  const RATINGS = [[0, "Tutti"], [6, "6+"], [7, "7+"], [8, "8+"]];
  function browseState(kind) {
    return { sort: "recent", minRating: 0, category: "", categoryName: "", ...store.get("browse." + kind, {}) };
  }

  async function renderGrid(main, kind, cursor, grid) {
    const b = browseState(kind);
    const q = new URLSearchParams({ limit: 60, sort: b.sort });
    if (b.minRating) q.set("min_rating", b.minRating);
    if (b.category) q.set("category", b.category);
    if (cursor) q.set("cursor", cursor);
    if (!grid) main.replaceChildren(el("h2", {}, kind === "movie" ? "Film" : "Serie"), filterBar(kind, b), skeletonRow(false));
    const page = await api("GET", "/api/v1/catalog/" + kind + "?" + q);
    if (!grid) {
      grid = el("div", { className: "grid" });
      main.lastElementChild.replaceWith(page.items.length ? grid : el("p", { className: "empty" }, "Nessun titolo con questi filtri."));
    }
    grid.append(...page.items.map(itemCard));
    const oldMore = main.querySelector(".more");
    if (oldMore) oldMore.remove();
    if (page.next) {
      const more = el("button", { className: "button more focusable" }, "Altri titoli");
      more.onclick = async () => { more.remove(); const n = grid.children.length; await renderGrid(main, kind, page.next, grid); focusFirst(grid.children[n]); };
      main.append(more);
    }
  }

  function filterBar(kind, b) {
    // After a choice the focus stays on the chip that was pressed.
    const set = (change) => {
      const pressed = document.activeElement && document.activeElement.closest(".filters .chip");
      const index = pressed ? [...pressed.parentElement.querySelectorAll(".chip")].indexOf(pressed) : -1;
      store.set("browse." + kind, { ...b, ...change });
      render().then(() => {
        const chips = $("content").querySelectorAll(".filters .chip");
        if (index >= 0 && chips[index]) chips[index].focus();
      });
    };
    const chip = (text, active, fn, icon) => {
      const c = el("button", { className: "chip focusable" + (active ? " active" : "") });
      window.VodMax.label(c, icon, text);
      c.onclick = fn;
      return c;
    };
    return el("div", { className: "chips filters" },
      el("span", { className: "chip-label" }, "Ordina"),
      ...SORTS.map(([v, t]) => chip(t, b.sort === v, () => set({ sort: v }))),
      el("span", { className: "chip-label" }, "Voto"),
      ...RATINGS.map(([v, t]) => chip(t, b.minRating === v, () => set({ minRating: v }))),
      el("span", { className: "chip-label" }, "Categoria"),
      chip(b.category ? b.categoryName : "Tutte", Boolean(b.category), () => pickCategory(kind, b, set), "list"),
      b.category ? chip("Togli", false, () => set({ category: "", categoryName: "" }), "x") : null);
  }

  // A panel to choose among hundreds of provider categories: type to
  // filter, arrows and OK to choose, Back to close.
  async function pickCategory(kind, b, set) {
    const back = document.activeElement;
    const list = await api("GET", "/api/v1/browse/" + kind + "/categories");
    const filter = el("input", { type: "search", className: "focusable", placeholder: "Cerca una categoria", autocomplete: "off" });
    const options = el("div", { className: "options" });
    const scrim = el("div", { className: "scrim" });
    const panel = el("section", { className: "picker", id: "picker" }, el("h2", {}, "Categoria"), filter, options);
    const close = () => { panel.remove(); scrim.remove(); if (back) back.focus(); };
    scrim.onclick = close;
    panel.close = close;
    const draw = () => {
      const f = filter.value.trim().toLowerCase();
      const all = el("button", { className: "chip focusable" + (b.category ? "" : " active") }, "Tutte");
      all.onclick = () => { close(); set({ category: "", categoryName: "" }); };
      // Names that start with the text first ("azione" before "animazione").
      const found = list.filter((c) => !f || c.name.toLowerCase().includes(f))
        .sort((x, y) => (f ? Number(!x.name.toLowerCase().startsWith(f)) - Number(!y.name.toLowerCase().startsWith(f)) : 0));
      options.replaceChildren(...(f ? [] : [all]), ...found.map((c) => {
        const o = el("button", { className: "chip focusable" + (c.id === b.category ? " active" : "") }, c.name + " ", el("small", {}, c.titles));
        o.onclick = () => { close(); set({ category: c.id, categoryName: c.name }); };
        return o;
      }));
    };
    filter.oninput = draw;
    draw();
    document.body.append(scrim, panel);
    (options.querySelector(".active") || options.firstElementChild || filter).focus();
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
    const [list, space, who] = await Promise.all([api("GET", "/api/v1/recordings"), api("GET", "/api/v1/recordings/space"),
      window.VodMax.me().catch(() => null)]);
    const kids = who && who.profile && who.profile.kind === "kids";
    const gb = (b) => (b / 1e9).toLocaleString("it-IT", { maximumFractionDigits: 1 }) + " GB";
    const day = (t) => new Date(t * 1000).toLocaleDateString("it-IT", { weekday: "short", day: "numeric", month: "short" });
    const usage = "Usati " + gb(space.used_bytes) + (space.free_bytes >= 0 ? " · liberi " + gb(space.free_bytes) : "") +
      (space.max_gb ? " · limite " + space.max_gb + " GB" : "") + (space.keep_days ? " · si tengono " + space.keep_days + " giorni" : "");
    const parts = [el("h2", {}, "Registrazioni"), el("p", { className: "empty" }, usage)];
    if (!list.length) parts.push(el("p", { className: "empty" }, "Nessuna registrazione. In TV premi R (tasto blu) sul programma in onda, " +
      "oppure registra dalla Guida, oppure crea una regola qui sotto."));
    else {
      parts.push(el("p", { className: "empty" }, "OK guarda · Giallo (P) protegge dalla cancellazione automatica · Rosso (Canc) elimina"));
      parts.push(el("ol", { className: "recs" }, ...list.map((r) => {
        const b = el("button", { className: "focusable" },
          el("span", {}, day(r.start) + " " + hhmm(r.start)),
          el("span", {}, el("strong", {}, r.title), " · " + r.channel, r.rule_id ? el("small", { className: "tag" }, " regola") : null),
          el("span", { className: "state " + r.status }, (r.protected ? "🔒 " : "") + (recState[r.status] || r.status)),
          el("span", { className: "state" }, r.bytes ? gb(r.bytes) : ""));
        b.onclick = () => r.status === "scheduled" ? flash("Non ancora iniziata") : playRecording(r);
        b.dataset.recording = r.id;
        b.dataset.protected = r.protected ? "1" : "";
        b.title = r.error || "";
        return el("li", {}, b);
      })));
    }
    if (!kids) parts.push(...await rulesSection());
    main.replaceChildren(...parts);
  }

  async function toggleProtected(id, now) {
    try { await api("PUT", "/api/v1/recordings/" + id + "/protected", { protected: !now }); flash(now ? "Protezione tolta" : "Protetta dalla cancellazione automatica"); render(); }
    catch (e) { flash(e.message); }
  }

  // ---- Recording rules ----
  const words = (v) => v.split(",").map((w) => w.trim()).filter(Boolean);
  const MATCH = { new: "da registrare", scheduled: "in programma", cancelled: "annullata a mano", conflict: "non registrabile", too_long: "troppo lunga" };

  async function rulesSection() {
    const rules = await api("GET", "/api/v1/recording-rules");
    const out = [el("h2", {}, "Regole automatiche"),
      el("p", { className: "empty" }, "Una regola registra da sola i programmi della guida che contengono certe parole: per esempio tutte le partite " +
        "della Juve, o gara e qualifiche di Formula 1. Si applica ogni 10 minuti ai prossimi giorni di guida.")];
    for (const r of rules) {
      const desc = ["tutte: " + r.spec.all.join(", "), r.spec.any && r.spec.any.length ? "almeno una: " + r.spec.any.join(", ") : "",
        r.spec.none && r.spec.none.length ? "escluse: " + r.spec.none.join(", ") : "", r.channel_name ? "solo " + r.channel_name : "",
        r.spec.section ? "sezione " + r.spec.section : "", r.keep ? "tiene le ultime " + r.keep : ""].filter(Boolean).join(" · ");
      const box = el("div", { className: "rule" }, el("strong", {}, r.name + (r.enabled ? "" : " (sospesa)")), el("p", { className: "empty" }, desc),
        r.last_note ? el("p", { className: "empty" }, "Ultimo controllo: " + r.last_note) : null);
      const edit = el("button", { className: "button secondary focusable" }, "Modifica");
      edit.onclick = () => { edit.disabled = true; box.append(ruleForm(r)); };
      const toggle = el("button", { className: "button secondary focusable" }, r.enabled ? "Sospendi" : "Riattiva");
      toggle.onclick = async () => {
        try { await api("PUT", "/api/v1/recording-rules/" + r.id, ruleInput(r, { enabled: !r.enabled })); render(); } catch (e) { flash(e.message); }
      };
      const del = el("button", { className: "button secondary focusable" }, "Elimina");
      del.onclick = async () => {
        if (!confirm("Eliminare la regola “" + r.name + "”? Le registrazioni già fatte restano; quelle in programma vengono tolte.")) return;
        try { await api("DELETE", "/api/v1/recording-rules/" + r.id); render(); } catch (e) { flash(e.message); }
      };
      box.append(el("div", { className: "actions" }, edit, toggle, del));
      out.push(box);
    }
    out.push(el("h3", {}, "Nuova regola"), ruleForm());
    return out;
  }

  function ruleInput(r, extra) {
    return { name: r.name, all: r.spec.all, any: r.spec.any || [], none: r.spec.none || [], section: r.spec.section || "", keep: r.keep,
      channel_id: 0, enabled: r.enabled, ...extra };
  }

  let channelsCache;
  function ruleForm(existing) {
    const f = (label, input) => el("label", { className: "rule-field" }, el("span", {}, label), input);
    const name = el("input", { className: "focusable", value: existing ? existing.name : "", placeholder: "es. Juventus" });
    const all = el("input", { className: "focusable", value: existing ? existing.spec.all.join(", ") : "", placeholder: "es. juventus" });
    const any = el("input", { className: "focusable", value: existing && existing.spec.any ? existing.spec.any.join(", ") : "", placeholder: "es. gara, qualifiche (facoltativo)" });
    const none = el("input", { className: "focusable", value: existing && existing.spec.none ? existing.spec.none.join(", ") : "", placeholder: "es. replica, highlights" });
    const section = el("select", { className: "focusable" }, el("option", { value: "" }, "Tutti i canali"), el("option", { value: "sport" }, "Solo Sport"), el("option", { value: "tv" }, "Solo TV"));
    section.value = existing ? existing.spec.section || "" : "";
    const channel = el("select", { className: "focusable" }, el("option", { value: "0" }, "Qualsiasi canale"));
    if (!channelsCache) channelsCache = api("GET", "/api/v1/tv/channels");
    channelsCache.then((list) => {
      channel.append(...list.map((c) => el("option", { value: c.id }, c.number + " " + c.name)));
      if (existing && existing.channel_name) {
        const o = [...channel.options].find((o) => o.textContent.endsWith(" " + existing.channel_name));
        if (o) channel.value = o.value;
      }
    }).catch(() => {});
    const keep = el("input", { className: "focusable", type: "number", min: 0, value: existing ? existing.keep || "" : "", placeholder: "tutte" });
    const result = el("ol", { className: "matches" });
    const read = () => ({ name: name.value || "Anteprima", all: words(all.value), any: words(any.value), none: words(none.value),
      section: section.value, channel_id: Number(channel.value), keep: Number(keep.value) || 0, enabled: existing ? existing.enabled : true });
    let timer;
    const refresh = () => {
      clearTimeout(timer);
      timer = setTimeout(async () => {
        if (!words(all.value).length) { result.replaceChildren(); return; }
        try {
          const m = await api("POST", "/api/v1/recording-rules/preview", read());
          result.replaceChildren(...(m.length ? m.map((x) => el("li", { className: "match " + x.state },
            day(x.start) + " " + hhmm(x.start) + " · " + x.channel + " · " + x.title + " — " + (MATCH[x.state] || x.state) + (x.note ? ": " + x.note : "")))
            : [el("li", { className: "empty" }, "Nessun programma corrispondente nella guida dei prossimi giorni")]));
        } catch (e) { result.replaceChildren(el("li", { className: "empty" }, e.message)); }
      }, 400);
    };
    const day = (t) => new Date(t * 1000).toLocaleDateString("it-IT", { weekday: "short", day: "numeric" });
    const save = el("button", { className: "button focusable", type: "submit" }, existing ? "Salva la regola" : "Crea la regola");
    const form = el("form", { className: "rule-form" }, f("Nome", name), f("Deve contenere (tutte)", all), f("E almeno una di", any),
      f("Escludi se contiene", none), f("Sezione", section), f("Canale", channel), f("Tieni le ultime", keep),
      el("p", { className: "empty" }, "Anteprima di cosa registrerebbe:"), result, save);
    form.addEventListener("input", refresh);
    form.addEventListener("change", refresh);
    form.onsubmit = async (e) => {
      e.preventDefault();
      try {
        const body = { ...read(), name: name.value };
        const r = existing ? await api("PUT", "/api/v1/recording-rules/" + existing.id, body) : await api("POST", "/api/v1/recording-rules", body);
        flash("Regola salvata: " + (r.last_note || ""));
        render();
      } catch (err) { flash(err.message); }
    };
    if (existing) refresh();
    return form;
  }

  async function deleteRecording(id) {
    if (!confirm("Eliminare la registrazione?")) return;
    try { await api("DELETE", "/api/v1/recordings/" + id); flash("Registrazione eliminata"); render(); }
    catch (e) { flash(e.message); }
  }

  // ---- Detail ----
  // Plot, credits and backdrop: from the provider (and TMDB when
  // configured), loaded after the page opens so it is never slower.
  async function showDetails(item) {
    const panel = $("detail");
    $("detail-plot").textContent = "";
    $("detail-credits").replaceChildren();
    panel.classList.remove("with-backdrop");
    panel.style.backgroundImage = "";
    let d;
    try { d = await api("GET", "/api/v1/items/" + item.id + "/details"); } catch { return; }
    if (state.detail !== item) return;
    if (d.title && d.title.toLowerCase() !== item.display_title.toLowerCase()) $("detail-title").textContent = d.title;
    const meta = $("detail-meta").textContent;
    const extra = [d.duration ? Math.round(d.duration / 60) + " min" : "", d.genre].filter(Boolean).join(" · ");
    if (extra) $("detail-meta").textContent = [meta, extra].filter(Boolean).join(" · ");
    $("detail-plot").textContent = d.plot || "";
    const rows = [["Regia", d.director], ["Cast", d.cast], ["Uscita", d.released], ["Paese", d.country],
      ["Titolo originale", d.original_title && d.original_title !== item.display_title ? d.original_title : ""]];
    for (const [k, v] of rows) if (v) $("detail-credits").append(el("dt", {}, k), el("dd", {}, v));
    if (d.trailer) {
      const a = el("a", { className: "button focusable trailer", href: "https://www.youtube.com/watch?v=" + encodeURIComponent(d.trailer),
        target: "_blank", rel: "noopener noreferrer" });
      window.VodMax.label(a, "circle-play", "Trailer");
      $("detail-actions").append(a);
    }
    if (d.backdrop) {
      const img = new Image();
      img.onload = () => {
        if (state.detail !== item) return;
        panel.style.backgroundImage = "url(" + JSON.stringify(d.backdrop) + ")";
        panel.classList.add("with-backdrop");
      };
      img.src = d.backdrop;
    }
  }

  async function openDetail(item) {
    state.detail = item;
    $("detail-title").textContent = item.display_title;
    $("detail-meta").textContent = [item.year, { 4: "4K", 3: "Full HD", 2: "HD", 1: "SD" }[item.quality],
      item.variant_count > 1 ? item.variant_count + " versioni" : "",
      item.rating ? "★ " + Number(item.rating).toLocaleString("it-IT", { maximumFractionDigits: 1 }) : ""].filter(Boolean).join(" · ");
    $("detail-image").hidden = !item.logo;
    if (item.logo) $("detail-image").src = item.logo;
    showDetails(item);
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
      const r = el("button", { className: "button primary focusable" });
      window.VodMax.label(r, "play", "Riprendi da " + clock(resume.position));
      r.onclick = () => play("movie", item.id, item.display_title, resume.position);
      actions.append(r);
    }
    const start = el("button", { className: "button focusable" + (resume ? "" : " primary") });
    window.VodMax.label(start, resume ? "undo-2" : "play", resume ? "Dall'inizio" : "Guarda");
    start.onclick = () => play("movie", item.id, item.display_title, 0);
    actions.append(start);
    if (item.variant_count > 1) {
      const v = el("button", { className: "button focusable" });
      v.dataset.variants = "1";
      window.VodMax.label(v, "layers", "Altre versioni");
      v.onclick = () => showVariants(item);
      actions.append(v);
    }
    // The trailer (added when the details arrive) always comes last.
    const trailer = actions.querySelector(".trailer");
    if (trailer) actions.append(trailer);
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

  $("detail-back").onclick = () => closeDetail();
  function closeDetail() {
    $("detail").hidden = true;
    state.detail = null;
    focusFirst($("content"));
  }

  // ---- Player ----
  const video = $("video");
  let saveTimer, idleTimer;

  async function play(media, id, title, startAt, mode, keep) {
    diagCount = 0;
    let sess;
    flash(mode === "remux" ? "Preparazione della conversione…" : "Avvio…");
    const native = window.VodMax.native;
    try {
      sess = media === "catchup"
        ? await api("POST", "/api/v1/catchup/" + id + "/session", { viewer, ...state.catchup })
        // The Android player reads MKV, AC-3, HEVC: the file as it is.
        : await api("POST", "/api/v1/vod/" + media + "/" + id + "/session", { viewer, mode: native ? "direct" : mode || "" });
    } catch (e) { flash(e.message); return; }
    if (native) { playNative({ media, id, title: title || sess.title, sess, startAt: startAt || 0 }); return; }
    stopPlayback(false);
    const tracks = chooseTracks(sess, keep);
    state.play = { media, id, title, sess, offset: 0, duration: sess.duration || 0, fellBack: mode === "remux", ...tracks };
    $("player-title").textContent = title || sess.title;
    $("player-live").hidden = true;
    $("player-audio").hidden = sess.audio.length < 2;
    $("player-subs").hidden = sess.subtitles.length === 0;
    $("player").hidden = false;
    // The remembered language needs another track than the direct file plays.
    if (sess.mode === "direct" && (tracks.audio !== 0 || tracks.sub >= 0)) {
      play(media, id, title, startAt, "remux", tracks);
      return;
    }
    load(startAt || 0);
    saveTimer = setInterval(saveProgress, 15000);
  }

  // ---- The Android app: its native player plays, the position comes
  // back here and is saved like in the browser. ----
  let nativeCtx = null;
  function playNative(ctx) {
    nativeCtx = ctx;
    let url = ctx.sess.url, start = ctx.startAt;
    if (ctx.sess.mode === "remux") { // a converted stream starts where asked
      url += "?t=" + Math.floor(start) + "&audio=" + (ctx.sess.default_audio || 0) + "&sub=-1";
      ctx.offset = Math.floor(start);
      start = 0;
    }
    $("status").hidden = true;
    window.VodMax.native.play(JSON.stringify({ url, title: ctx.title, start, live: Boolean(ctx.live) }));
  }
  window.VodMaxNativeDone = (r) => {
    const c = nativeCtx;
    nativeCtx = null;
    if (!c) return;
    if (r.error) flash("Riproduzione non riuscita: " + r.error);
    const position = (c.offset || 0) + r.position;
    const total = (c.sess && c.sess.duration) || r.duration;
    if ((c.media === "movie" || c.media === "episode") && total > 0 && position > 5) {
      api("PUT", "/api/v1/progress/" + c.media + "/" + c.id, { position: Math.floor(position), duration: Math.floor(total) })
        .then(() => { if (state.view === "home" && $("detail").hidden) render(); }).catch(() => {});
    }
    if (c.media === "catchup" && state.catchupBack) location.href = state.catchupBack;
  };
  window.VodMaxNativeError = (m) => flash(m);
  // Android "Back": close what is open, then the menu's home; false lets
  // the app go to the background.
  window.VodMaxNativeBack = () => {
    if (window.VodMax.sheetOpen()) { window.VodMax.closeSheet(); return true; }
    if ($("picker") || !$("player").hidden || !$("detail").hidden || state.query) {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      return true;
    }
    if (state.view !== "home") { state.view = "home"; render(); return true; }
    return false;
  };

  // ---- Audio and subtitles ----
  // The chosen languages are remembered per profile; a file without them
  // falls back to the server's suggestion (Italian audio, no subtitles).
  const langKey = () => "lang." + (store.get("profile", "") || "").split(".")[0];
  function chooseTracks(sess, keep) {
    if (keep) return keep;
    const pref = store.get(langKey(), {});
    let audio = sess.default_audio || 0, sub = -1;
    const a = pref.audio && sess.audio.find((t) => t.language === pref.audio);
    if (a) audio = a.index;
    const t = pref.sub && sess.subtitles.find((t) => t.language === pref.sub && t.supported);
    if (t) sub = t.index;
    return { audio, sub };
  }

  const LANG = { ita: "Italiano", it: "Italiano", eng: "Inglese", en: "Inglese", spa: "Spagnolo", fre: "Francese", fra: "Francese",
    ger: "Tedesco", deu: "Tedesco", por: "Portoghese", jpn: "Giapponese", kor: "Coreano", rus: "Russo", ara: "Arabo", alb: "Albanese" };
  function trackLabel(t, kind) {
    const lang = LANG[t.language] || (t.language ? t.language.toUpperCase() : "Traccia " + (t.index + 1));
    const extra = [t.title && t.title !== lang ? t.title : "", kind === "audio" && t.channels > 2 ? t.channels === 6 ? "5.1" : t.channels + " canali" : "",
      t.forced ? "forzati" : "", kind === "sub" && !t.supported ? "immagine, non nel browser" : ""].filter(Boolean);
    return lang + (extra.length ? " (" + extra.join(", ") + ")" : "");
  }

  function openMenu(kind) {
    const p = state.play;
    if (!p) return;
    const menu = $("player-menu");
    if (!menu.hidden && menu.dataset.kind === kind) { closeMenu(); return; }
    menu.dataset.kind = kind;
    const items = kind === "audio"
      ? p.sess.audio.map((t) => ({ label: trackLabel(t, "audio"), on: t.index === p.audio, ok: true, pick: () => switchTracks(t.index, p.sub, { audio: t.language }) }))
      : [{ label: "Nessuno", on: p.sub < 0, ok: true, pick: () => switchTracks(p.audio, -1, { sub: "" }) },
        ...p.sess.subtitles.map((t) => ({ label: trackLabel(t, "sub"), on: t.index === p.sub, ok: t.supported, pick: () => switchTracks(p.audio, t.index, { sub: t.language }) }))];
    const buttons = items.map((it) => {
      const b = document.createElement("button");
      b.className = "focusable" + (it.on ? " on" : "");
      b.textContent = (it.on ? "✓ " : "") + it.label;
      b.disabled = !it.ok;
      b.onclick = () => { closeMenu(); it.pick(); };
      return b;
    });
    const title = Object.assign(document.createElement("div"), { className: "menu-title", textContent: kind === "audio" ? "Lingua dell'audio" : "Sottotitoli" });
    menu.replaceChildren(title, ...buttons);
    menu.hidden = false;
    showBar();
    (buttons.find((b) => b.classList.contains("on")) || buttons[0]).focus();
  }
  function closeMenu() { $("player-menu").hidden = true; }

  function switchTracks(audio, sub, remember) {
    const p = state.play;
    if (!p || (audio === p.audio && sub === p.sub)) return;
    store.set(langKey(), { ...store.get(langKey(), {}), ...remember });
    const at = position();
    if (p.sess.mode === "direct") {
      // Another audio track or subtitles: the server converts the file.
      play(p.media, p.id, p.title, at, "remux", { audio, sub });
      return;
    }
    p.audio = audio;
    p.sub = sub;
    load(at);
  }

  // Subtitles: the remux writes WebVTT as it goes; new cues are added to
  // a text track every two seconds. Cue times start with the remux, like
  // the video element's clock.
  let subTimer, subTrack;
  function startSubtitles() {
    clearInterval(subTimer);
    if (!subTrack) { subTrack = video.addTextTrack("subtitles", "Sottotitoli", "it"); }
    while (subTrack.cues && subTrack.cues.length) subTrack.removeCue(subTrack.cues[0]);
    const p = state.play;
    if (!p || p.sub < 0 || p.sess.mode !== "remux") { subTrack.mode = "disabled"; return; }
    subTrack.mode = "showing";
    let added = 0;
    const url = p.sess.subtitles_url;
    const pull = async () => {
      if (state.play !== p) { clearInterval(subTimer); return; }
      const text = await fetch(url, { cache: "no-store" }).then((r) => (r.ok ? r.text() : "")).catch(() => "");
      const cues = parseVTT(text);
      for (; added < cues.length; added++) {
        const c = cues[added];
        try { subTrack.addCue(new VTTCue(c.start, c.end, c.text)); } catch { /* bad cue */ }
      }
    };
    pull();
    subTimer = setInterval(pull, 2000);
  }

  function parseVTT(text) {
    const out = [];
    const time = (t) => { const p = t.trim().split(":").map(Number); return p.length === 3 ? p[0] * 3600 + p[1] * 60 + p[2] : p[0] * 60 + p[1]; };
    for (const block of text.replace(/\r/g, "").split(/\n\n+/)) {
      const lines = block.split("\n");
      const i = lines.findIndex((l) => l.includes("-->"));
      if (i < 0) continue;
      const [a, b] = lines[i].split("-->");
      const end = time(b.trim().split(/\s+/)[0]);
      const textLines = lines.slice(i + 1).join("\n").trim();
      // The last block may still be being written.
      if (!textLines || !isFinite(end)) continue;
      out.push({ start: time(a), end, text: textLines.replace(/<[^>]+>/g, "") });
    }
    return out;
  }

  function load(position, force) {
    const p = state.play;
    const extra = force ? "force=1" : "";
    if (p.sess.mode === "remux") {
      p.offset = position;
      video.src = p.sess.url + "?t=" + Math.floor(position) + "&audio=" + p.audio + "&sub=" + p.sub + (extra ? "&" + extra : "");
      startSubtitles();
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

  function toggle() {
    const p = state.play;
    if (!p) return;
    if (p.ts) p.ts.toggle(); else if (video.paused) video.play(); else video.pause();
    updateToggle();
    showBar();
  }

  function updateToggle() {
    const p = state.play;
    const paused = p && (p.ts ? p.ts.paused : video.paused);
    window.VodMax.label($("player-toggle"), paused ? "play" : "pause", paused ? "Riprendi" : "Pausa");
  }
  video.addEventListener("play", updateToggle);
  video.addEventListener("pause", updateToggle);

  function seek(delta) {
    const p = state.play;
    if (!p) return;
    if (p.ts) { p.ts.seek(delta); showBar(); return; }
    const target = Math.max(0, Math.min(position() + delta, (duration() || Infinity) - 5));
    if (p.sess.mode === "direct") video.currentTime = target;
    else load(target);
    showBar();
  }

  // ---- Seek bar: click or drag ----
  function seekTo(target) {
    const p = state.play;
    if (!p) return;
    const d = duration();
    target = Math.max(0, Math.min(target, (d || Infinity) - 3));
    if (p.ts) p.ts.seek(target - position());
    else if (p.sess.mode === "direct") video.currentTime = target;
    else load(target);
    showBar();
  }
  {
    const bar = $("player-seek");
    let dragging = false;
    const at = (e) => {
      const r = bar.getBoundingClientRect();
      return Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
    };
    const preview = (e) => {
      const d = duration();
      if (!d) return;
      const f = at(e);
      $("player-hover").textContent = clock(f * d);
      $("player-hover").style.left = f * 100 + "%";
      $("player-hover").hidden = false;
      if (dragging) $("player-fill").style.width = f * 100 + "%";
      showBar();
    };
    bar.addEventListener("pointerdown", (e) => {
      if (!duration()) return;
      dragging = true;
      bar.classList.add("dragging");
      bar.setPointerCapture(e.pointerId);
      preview(e);
    });
    bar.addEventListener("pointermove", preview);
    bar.addEventListener("pointerleave", () => { if (!dragging) $("player-hover").hidden = true; });
    const finish = (e) => {
      if (!dragging) return;
      dragging = false;
      bar.classList.remove("dragging");
      $("player-hover").hidden = true;
      seekTo(at(e) * duration());
    };
    bar.addEventListener("pointerup", finish);
    bar.addEventListener("pointercancel", () => { dragging = false; bar.classList.remove("dragging"); $("player-hover").hidden = true; });
    state.seekDragging = () => dragging;
  }

  video.addEventListener("timeupdate", () => {
    const d = duration();
    $("player-pos").textContent = clock(position());
    $("player-dur").textContent = d ? clock(d) : "";
    if (!(state.seekDragging && state.seekDragging())) $("player-fill").style.width = d ? Math.min(100, (position() / d) * 100) + "%" : "0";
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

  // ---- Diagnostics: what the player does, sent to the server log ----
  let diagCount = 0;
  function diag(event, extra) {
    const p = state.play;
    if (!p || diagCount++ > 300) return;
    const b = video.buffered;
    const ranges = [];
    for (let i = 0; i < b.length && i < 4; i++) ranges.push(b.start(i).toFixed(1) + "-" + b.end(i).toFixed(1));
    api("POST", "/api/v1/debug/player", { event, detail: {
      media: p.media, id: p.id, mode: p.sess && p.sess.mode, t: Number(video.currentTime.toFixed(2)), ready: video.readyState,
      net: video.networkState, paused: video.paused, buffered: ranges.join(" "),
      error: video.error ? video.error.code + " " + (video.error.message || "") : "", ...extra } }).catch(() => {});
  }
  for (const ev of ["loadstart", "loadedmetadata", "canplay", "playing", "waiting", "stalled", "pause", "play", "seeking", "seeked", "error", "emptied", "abort"]) {
    video.addEventListener(ev, () => diag(ev));
  }
  setInterval(() => { if (state.play && !video.paused) diag("tick"); }, 10000);

  function saveProgress() {
    const p = state.play;
    if (!p || p.recording || p.media === "catchup" || !duration()) return;
    api("PUT", "/api/v1/progress/" + p.media + "/" + p.id, { position: Math.floor(position()), duration: Math.floor(duration()) })
      .catch(() => {});
  }

  function stopPlayback(save) {
    if (save) saveProgress();
    // An archive opened from the TV or the guide: closing goes back there.
    if (save && state.play && state.play.media === "catchup" && state.catchupBack) {
      location.href = state.catchupBack;
      return;
    }
    clearInterval(saveTimer);
    clearInterval(subTimer);
    if (subTrack) subTrack.mode = "disabled";
    closeMenu();
    if (state.play && state.play.ts) state.play.ts.destroy();
    video.pause();
    video.removeAttribute("src");
    video.load();
    state.play = null;
    $("player").hidden = true;
  }

  function showBar() {
    $("player-bar").classList.remove("idle");
    $("player").classList.remove("cursor-hidden");
    updateToggle();
    clearTimeout(idleTimer);
    idleTimer = setTimeout(() => {
      const p = state.play;
      if (p && (p.ts ? p.ts.paused : video.paused)) return; // stays visible while paused
      $("player-bar").classList.add("idle");
      $("player").classList.add("cursor-hidden");
    }, 4000);
  }

  async function playRecording(r) {
    let url;
    try { url = (await api("POST", "/api/v1/recordings/" + r.id + "/ticket")).url; } catch (e) { flash(e.message); return; }
    if (window.VodMax.native) {
      playNative({ media: "recording", id: r.id, title: r.title + " · " + r.channel, sess: { mode: "direct", url }, startAt: 0, live: r.status === "recording" });
      return;
    }
    stopPlayback(false);
    state.play = { recording: r, title: r.title, sess: { mode: "direct", url }, offset: 0, duration: r.stop - r.start };
    $("player-audio").hidden = $("player-subs").hidden = true;
    $("player-title").textContent = r.title + " · " + r.channel;
    $("player").hidden = false;
    // Finished recordings are MP4 (seekable); recordings in progress, or
    // left in MPEG-TS, go through mpegts.js.
    const isTS = r.status === "recording" || !(await fetch(url, { headers: { Range: "bytes=0-0" } })
      .then((res) => { res.body && res.body.cancel(); return (res.headers.get("Content-Type") || "").includes("mp4"); }).catch(() => false));
    if (isTS && window.mpegts && mpegts.isSupported()) {
      state.play.ts = window.VodMax.tsPlayer(video, url, {
        live: r.status === "recording",
        onError: (type, detail) => flash("Registrazione non riproducibile (" + detail + ")"),
      });
      $("player-live").hidden = r.status !== "recording";
    } else {
      video.src = url;
      video.play().catch(() => flash("Premi OK per avviare"));
    }
    showBar();
  }

  // ---- Remote control navigation ----
  function focusables(root = document) {
    const layer = $("picker") || (!$("player").hidden ? $("player") : !$("detail").hidden ? $("detail") : document);
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
      // Moving among the on-screen buttons and track menu.
      const inBar = document.activeElement && $("player-bar").contains(document.activeElement);
      if (inBar) {
        if (key.startsWith("Arrow")) { moveFocus(key); showBar(); e.preventDefault(); return; }
        if (key === "OK") { document.activeElement.click(); showBar(); e.preventDefault(); return; }
        if (key === "Back" || key === "Backspace") {
          if (!$("player-menu").hidden) { closeMenu(); $("player-toggle").focus(); } else document.activeElement.blur();
          e.preventDefault();
          return;
        }
      }
      if (key === "ArrowDown") { showBar(); $("player-toggle").focus(); e.preventDefault(); return; }
      const handled = {
        OK: toggle, PlayPause: toggle, " ": toggle,
        Play: () => { if (state.play && (state.play.ts ? state.play.ts.paused : video.paused)) toggle(); },
        Pause: () => { if (state.play && !(state.play.ts ? state.play.ts.paused : video.paused)) toggle(); },
        ArrowLeft: () => seek(-10), ArrowRight: () => seek(30), ArrowUp: showBar, ArrowDown: showBar,
        Back: () => stopPlayback(true), Stop: () => stopPlayback(true), Backspace: () => stopPlayback(true),
      }[key];
      if (handled) { handled(); e.preventDefault(); }
      return;
    }
    if ($("picker")) {
      const typing = document.activeElement && document.activeElement.tagName === "INPUT";
      if (key === "Back" || (key === "Backspace" && !typing)) { $("picker").close(); e.preventDefault(); return; }
      if (key.startsWith("Arrow") && !(typing && (key === "ArrowLeft" || key === "ArrowRight"))) { moveFocus(key); e.preventDefault(); }
      return;
    }
    const active = document.activeElement;
    if (active && active.tagName === "INPUT" && active !== $("search")) return; // e.g. the token form
    const typing = active === $("search");
    if (typing && !["ArrowDown", "Back", "OK"].includes(key)) return;
    if (key.startsWith("Arrow")) { moveFocus(key); e.preventDefault(); return; }
    if (key === "OK" && typing) { $("search").blur(); focusFirst($("content")); e.preventDefault(); return; }
    if (key === "Back" || (key === "Backspace" && !typing)) {
      if (!$("detail").hidden) closeDetail();
      else if (state.query) { state.query = ""; $("search").value = ""; render(); }
      e.preventDefault();
      return;
    }
    if (key === "Green" || key === "g") { location.href = "/tv.html"; return; }
    if ((key === "Yellow" || key === "p") && document.activeElement && document.activeElement.dataset.recording) {
      toggleProtected(document.activeElement.dataset.recording, document.activeElement.dataset.protected === "1");
      e.preventDefault();
      return;
    }
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

  // Mouse and touch: buttons in the bar, click on the picture to pause.
  $("player-bar").addEventListener("click", (e) => {
    const b = e.target.closest("button[data-action]");
    if (!b) return;
    ({
      close: () => stopPlayback(true), back10: () => seek(-10), fwd30: () => seek(30), toggle,
      audio: () => openMenu("audio"), subs: () => openMenu("subs"),
      live: () => { if (state.play && state.play.ts) { state.play.ts.goLive(); showBar(); } },
    })[b.dataset.action]();
  });
  video.addEventListener("click", toggle);
  $("player").addEventListener("mousemove", showBar);

  addEventListener("pagehide", () => saveProgress());
  // From the TV ("from the start") or the guide ("watch again"):
  // /?catchup=CHANNEL&start=…&stop=…&title=…&back=…
  const asked = new URLSearchParams(location.search);
  window.VodMax.withToken(() => {
    render();
    if (asked.get("catchup")) {
      state.catchup = { start: Number(asked.get("start")), stop: Number(asked.get("stop")), title: asked.get("title") || "" };
      const back = asked.get("back") || "";
      state.catchupBack = back.startsWith("/") && !back.startsWith("//") ? back : "";
      history.replaceState(null, "", "/");
      play("catchup", Number(asked.get("catchup")), state.catchup.title, 0, "remux");
    }
  });
})();
