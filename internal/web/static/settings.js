// VodMax settings: subscriptions, VPN connections, catalog selection and
// categories. Secrets are written, never read back.
"use strict";

(() => {
  const { store, api, hhmm } = window.VodMax;
  const $ = (id) => document.getElementById(id);
  const el = (tag, props = {}, ...children) => {
    const e = Object.assign(document.createElement(tag), props);
    e.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
    return e;
  };
  let statusTimer;
  function flash(text) {
    $("status").textContent = text;
    $("status").hidden = false;
    clearTimeout(statusTimer);
    statusTimer = setTimeout(() => { $("status").hidden = true; }, 5000);
  }
  const button = (label, onclick, cls = "") => el("button", { className: "small focusable " + cls, type: "button", onclick }, label);
  const field = (label, input) => [el("label", {}, label), input];
  const input = (props) => el("input", { className: "focusable", ...props });
  const when = (t) => {
    if (!t) return "mai";
    const d = new Date(typeof t === "number" ? t * 1000 : t);
    return d.toLocaleDateString("it-IT", { day: "numeric", month: "short" }) + " " + hhmm(d.getTime() / 1000);
  };

  const asked = new URLSearchParams(location.search).get("view");
  const state = { view: asked || store.get("settingsView", "subscriptions"), sub: store.get("settingsSub", 0), kind: "live" };

  async function render() {
    for (const t of document.querySelectorAll(".tab[data-view]")) t.classList.toggle("active", t.dataset.view === state.view);
    store.set("settingsView", state.view);
    const main = $("content");
    main.replaceChildren(el("p", { className: "note" }, "Caricamento…"));
    try {
      await ({ subscriptions: renderSubscriptions, vpn: renderVPN, rules: renderRules, categories: renderCategories, system: renderSystem, profiles: renderProfiles, devices: renderDevices })[state.view](main);
    } catch (e) {
      main.replaceChildren(el("p", { className: "note bad" }, e.message));
    }
  }

  // ---- Subscriptions ----
  async function renderSubscriptions(main) {
    const [subs, vpns] = await Promise.all([api("GET", "/api/v1/subscriptions"), api("GET", "/api/v1/vpn")]);
    const vpnName = Object.fromEntries(vpns.map((v) => [v.id, v.name]));
    const parts = [el("h2", {}, "Abbonamenti"),
      el("p", { className: "note" }, "Playlist dei provider (Xtream Codes o M3U). Ogni abbonamento passa dalla sua VPN: senza VPN il provider non viene mai contattato.")];
    for (const s of subs) parts.push(subscriptionBox(s, vpns, vpnName));
    parts.push(el("h2", {}, "Aggiungi un abbonamento"));
    parts.push(vpns.length ? subscriptionForm(vpns) : el("p", { className: "note" }, "Prima aggiungi una connessione VPN (scheda VPN)."));
    main.replaceChildren(...parts);
  }

  function subscriptionBox(s, vpns, vpnName) {
    const settings = s.settings || {};
    const a = s.account || {};
    const facts = el("div", { className: "facts" },
      el("span", {}, "Tipo ", el("b", {}, s.kind === "xtream" ? "Xtream Codes" : "Playlist M3U")),
      settings.server ? el("span", {}, "Server ", el("b", {}, settings.server)) : null,
      settings.username ? el("span", {}, "Utente ", el("b", {}, settings.username)) : null,
      settings.host ? el("span", {}, "Host ", el("b", {}, settings.host)) : null,
      el("span", {}, "VPN ", el("b", {}, vpnName[s.vpn_id] || "?")),
      a.status ? el("span", {}, "Account ", el("b", { className: a.active ? "ok" : "bad" }, a.active ? "attivo" : a.status)) : null,
      a.expires > 0 ? el("span", {}, "Scadenza ", el("b", {}, new Date(a.expires * 1000).toLocaleDateString("it-IT"))) : null,
      a.max_connections ? el("span", {}, "Connessioni ", el("b", {}, (a.active_cons || 0) + " / " + a.max_connections)) : null);
    const counts = s.counts || {};
    const sync = el("div", { className: "facts" },
      el("span", {}, "Ultima sincronizzazione ", el("b", { className: s.sync_status === "error" ? "bad" : "" }, when(s.synced_at) +
        (s.sync_status === "error" ? " (errore)" : ""))),
      el("span", {}, "Canali ", el("b", {}, counts.live || 0)), el("span", {}, "Film ", el("b", {}, counts.movie || 0)),
      el("span", {}, "Serie ", el("b", {}, counts.series || 0)));
    const box = el("section", { className: "box" }, el("h3", {}, s.name), facts, sync,
      s.sync_error ? el("p", { className: "note bad" }, s.sync_error) : null);
    const syncButton = button(s.syncing ? "Sincronizzazione in corso…" : "Sincronizza ora", async () => {
      syncButton.disabled = true;
      syncButton.textContent = "Sincronizzazione in corso… (anche un minuto)";
      try {
        const r = await api("POST", "/api/v1/subscriptions/" + s.id + "/sync");
        const sel = r.selection || {};
        flash("Sincronizzato: " + ["live", "movie", "series"].map((k) => (sel[k] ? sel[k].cards : 0) + " " +
          { live: "canali", movie: "film", series: "serie" }[k]).join(", ") + (r.guide ? ", guida " + r.guide.programmes + " programmi" : ""));
      } catch (e) { flash(e.message); }
      render();
    }, "primary");
    syncButton.disabled = s.syncing;
    const edit = button("Modifica", () => {
      edit.disabled = true;
      box.append(subscriptionForm(vpns, s));
    });
    const del = button("Elimina", async () => {
      if (!confirm("Eliminare l'abbonamento “" + s.name + "” e il suo catalogo? Preferiti e avanzamenti dei suoi titoli andranno persi.")) return;
      try { await api("DELETE", "/api/v1/subscriptions/" + s.id); flash("Abbonamento eliminato"); } catch (e) { flash(e.message); }
      render();
    }, "danger");
    const select = button("Selezione del catalogo", () => { state.sub = s.id; store.set("settingsSub", s.id); state.view = "rules"; render(); });
    box.append(el("div", { className: "buttons" }, syncButton, select, edit, del));
    return box;
  }

  // Add (existing undefined) or edit a subscription.
  function subscriptionForm(vpns, existing) {
    const kind = el("select", { className: "focusable" }, el("option", { value: "xtream" }, "Xtream Codes"), el("option", { value: "m3u" }, "Playlist M3U"));
    const name = input({ value: existing ? existing.name : "", placeholder: "es. Provider casa" });
    const vpn = el("select", { className: "focusable" }, ...vpns.map((v) => el("option", { value: v.id }, v.name)));
    const set = (existing && existing.settings) || {};
    const server = input({ value: set.server || "", placeholder: "http://server:porta" });
    const user = input({ value: set.username || "", autocomplete: "off" });
    const pass = input({ type: "password", autocomplete: "new-password", placeholder: existing ? "lascia vuoto per non cambiarla" : "" });
    const url = input({ placeholder: existing ? "lascia vuoto per non cambiarlo" : "http://…/get.php?…" });
    if (existing) { kind.value = existing.kind; kind.disabled = true; vpn.value = existing.vpn_id; }
    const xtreamRows = [...field("Server", server), ...field("Utente", user), ...field("Password", pass)];
    const m3uRows = [...field("Indirizzo della playlist", url)];
    const form = el("form", { className: "form" }, ...field("Tipo", kind), ...field("Nome", name), ...field("VPN", vpn), ...xtreamRows, ...m3uRows);
    const toggle = () => {
      for (const e of xtreamRows) e.hidden = kind.value !== "xtream";
      for (const e of m3uRows) e.hidden = kind.value !== "m3u";
    };
    kind.onchange = toggle;
    toggle();
    const save = el("button", { className: "small primary focusable full", type: "submit" }, existing ? "Salva le modifiche" : "Aggiungi");
    form.append(save);
    form.onsubmit = async (e) => {
      e.preventDefault();
      save.disabled = true;
      try {
        if (existing) {
          const patch = { name: name.value, vpn_id: Number(vpn.value) };
          if (existing.kind === "xtream") Object.assign(patch, { server: server.value, username: user.value, password: pass.value });
          else patch.url = url.value;
          await api("PATCH", "/api/v1/subscriptions/" + existing.id, patch);
          flash("Abbonamento aggiornato");
        } else {
          const body = { name: name.value, kind: kind.value, vpn_id: Number(vpn.value) };
          if (kind.value === "xtream") Object.assign(body, { server: server.value, username: user.value, password: pass.value });
          else body.url = url.value;
          await api("POST", "/api/v1/subscriptions", body);
          flash("Abbonamento aggiunto: premi “Sincronizza ora” per scaricare il catalogo");
        }
        render();
      } catch (err) { flash(err.message); save.disabled = false; }
    };
    return form;
  }

  // ---- VPN ----
  async function renderVPN(main) {
    const vpns = await api("GET", "/api/v1/vpn");
    const parts = [el("h2", {}, "Connessioni VPN"),
      el("p", { className: "note" }, "VPN WireGuard interne a VodMax, una o più. Si importa il file .conf del proprio servizio (Proton, Mullvad, NordVPN…). La chiave privata resta cifrata e non viene mai mostrata.")];
    for (const v of vpns) parts.push(vpnBox(v));
    parts.push(el("h2", {}, "Aggiungi una VPN"), vpnForm());
    main.replaceChildren(...parts);
  }

  function vpnBox(v) {
    const st = v.status || {};
    const connected = v.running && st.connected;
    const box = el("section", { className: "box" }, el("h3", {}, v.name), el("div", { className: "facts" },
      el("span", {}, "Server ", el("b", {}, v.endpoint || "?")),
      el("span", {}, "Stato ", el("b", { className: connected ? "ok" : "" }, connected ? "collegata" : v.running ? "in collegamento" : "non collegata")),
      st.last_handshake && st.last_handshake !== "0001-01-01T00:00:00Z" ? el("span", {}, "Ultimo handshake ", el("b", {}, when(st.last_handshake))) : null,
      v.running ? el("span", {}, "Traffico ", el("b", {}, ((st.rx_bytes || 0) / 1e6).toFixed(1) + " MB ricevuti")) : null,
      v.health && v.health.reconnects > 0 ? el("span", {}, "Ricollegamenti automatici ", el("b", {}, v.health.reconnects)) : null),
      v.health && !v.health.ok ? el("p", { className: "note bad" }, "⚠ " + v.health.error + " (da " + when(v.health.since) + ")") : null);
    const connect = button(v.running ? "Scollega" : "Collega", async () => {
      connect.disabled = true;
      try {
        if (v.running) { await api("POST", "/api/v1/vpn/" + v.id + "/disconnect"); flash("VPN scollegata"); }
        else { const s = await api("POST", "/api/v1/vpn/" + v.id + "/connect"); flash(s.connected ? "VPN collegata" : "Nessuna risposta dal server VPN"); }
      } catch (e) { flash(e.message); }
      render();
    }, v.running ? "" : "primary");
    const edit = button("Modifica", () => { edit.disabled = true; box.append(vpnForm(v)); });
    const del = button("Elimina", async () => {
      if (!confirm("Eliminare la VPN “" + v.name + "”?")) return;
      try { await api("DELETE", "/api/v1/vpn/" + v.id); flash("VPN eliminata"); } catch (e) { flash(e.message); }
      render();
    }, "danger");
    box.append(el("div", { className: "buttons" }, connect, edit, del));
    return box;
  }

  function vpnForm(existing) {
    const name = input({ value: existing ? existing.name : "", placeholder: "es. Proton Italia" });
    const conf = el("textarea", { className: "focusable", spellcheck: false,
      placeholder: existing ? "Incolla un nuovo .conf per sostituirlo, oppure lascia vuoto" : "[Interface]\nPrivateKey = …\nAddress = …\nDNS = …\n\n[Peer]\nPublicKey = …\nEndpoint = …" });
    const file = input({ type: "file", accept: ".conf,text/plain" });
    file.onchange = async () => { if (file.files[0]) conf.value = await file.files[0].text(); };
    const save = el("button", { className: "small primary focusable full", type: "submit" }, existing ? "Salva le modifiche" : "Aggiungi");
    const form = el("form", { className: "form" }, ...field("Nome", name), ...field("File .conf", file), ...field("Configurazione", conf), save);
    form.onsubmit = async (e) => {
      e.preventDefault();
      save.disabled = true;
      try {
        if (existing) await api("PATCH", "/api/v1/vpn/" + existing.id, { name: name.value, config: conf.value });
        else await api("POST", "/api/v1/vpn", { name: name.value, config: conf.value });
        flash(existing ? "VPN aggiornata" : "VPN aggiunta");
        render();
      } catch (err) { flash(err.message); save.disabled = false; }
    };
    return form;
  }

  // ---- Subscription picker for selection pages ----
  async function pickSubscription(main, title, note) {
    const subs = await api("GET", "/api/v1/subscriptions");
    if (!subs.length) { main.replaceChildren(el("p", { className: "note" }, "Nessun abbonamento: aggiungine uno nella scheda Abbonamenti.")); return null; }
    if (!subs.some((s) => s.id === state.sub)) state.sub = subs[0].id;
    const pick = el("select", { className: "pick focusable" }, ...subs.map((s) => el("option", { value: s.id }, s.name)));
    pick.value = state.sub;
    pick.onchange = () => { state.sub = Number(pick.value); store.set("settingsSub", state.sub); render(); };
    main.replaceChildren(el("h2", {}, title), el("p", { className: "note" }, note), el("div", { className: "toolbar" }, el("span", {}, "Abbonamento"), pick));
    return subs.find((s) => s.id === state.sub);
  }

  // ---- Rules ----
  async function renderRules(main) {
    const sub = await pickSubscription(main, "Selezione del catalogo",
      "Cosa tenere della lista del provider. Le regole valgono a ogni sincronizzazione; le correzioni su singole categorie (scheda Categorie) prevalgono.");
    if (!sub) return;
    const [rules, cats, current] = await Promise.all([
      api("GET", "/api/v1/subscriptions/" + sub.id + "/rules"),
      api("GET", "/api/v1/subscriptions/" + sub.id + "/categories?kind=live"),
      api("GET", "/api/v1/subscriptions/" + sub.id + "/selection")]);
    const countries = {};
    for (const c of cats) {
      const country = c.country_override != null ? c.country_override : c.country;
      if (country) countries[country] = (countries[country] || 0) + c.items;
    }
    const adult = input({ type: "checkbox", checked: rules.adult });
    const lowres = input({ type: "checkbox", checked: rules.low_res });
    const movieYear = input({ type: "number", min: 1900, max: 2100, placeholder: "nessun limite", value: rules.movie_min_year || "" });
    const seriesYear = input({ type: "number", min: 1900, max: 2100, placeholder: "nessun limite", value: rules.series_min_year || "" });
    const unknownYear = input({ type: "checkbox", checked: rules.hide_unknown_year });
    const countryBoxes = Object.entries(countries).sort((a, b) => b[1] - a[1]).map(([name, n]) => {
      const cb = input({ type: "checkbox", checked: (rules.countries || []).includes(name), value: name });
      return el("label", { className: "check" }, cb, name + " (" + n + ")");
    });
    const table = el("table", { className: "counts" });
    const readRules = () => ({
      countries: countryBoxes.map((l) => l.firstChild).filter((c) => c.checked).map((c) => c.value),
      adult: adult.checked, low_res: lowres.checked,
      movie_min_year: Number(movieYear.value) || 0, series_min_year: Number(seriesYear.value) || 0,
      hide_unknown_year: unknownYear.checked,
    });
    const showCounts = (preview) => {
      const rows = [["live", "Canali"], ["movie", "Film"], ["series", "Serie"]].map(([k, label]) => {
        const now = current[k] || {}, next = (preview || current)[k] || {};
        const cell = (v, before) => el("td", { className: preview && v !== before ? "changed" : "" }, (v || 0).toLocaleString("it-IT"));
        return el("tr", {}, el("td", {}, label), el("td", {}, (now.available || 0).toLocaleString("it-IT")),
          cell(next.visible, now.visible), cell(next.cards, now.cards));
      });
      table.replaceChildren(el("tr", {}, el("th", {}, ""), el("th", {}, "Dal provider"), el("th", {}, preview ? "Visibili (anteprima)" : "Visibili"),
        el("th", {}, preview ? "Schede (anteprima)" : "Schede")), ...rows);
    };
    showCounts(null);
    let previewTimer;
    const preview = () => {
      clearTimeout(previewTimer);
      previewTimer = setTimeout(async () => {
        try { showCounts(await api("POST", "/api/v1/subscriptions/" + sub.id + "/rules/preview", readRules())); }
        catch (e) { flash(e.message); }
      }, 300);
    };
    const form = el("form", { className: "form" },
      el("label", {}, "Paesi esteri da tenere"), el("div", { className: "countries" }, ...(countryBoxes.length ? countryBoxes : [el("span", { className: "note" }, "Nessun canale estero riconosciuto")])),
      el("label", {}, "Contenuti per adulti"), el("label", { className: "check" }, adult, "Mostra"),
      el("label", {}, "Doppioni a bassa qualità"), el("label", { className: "check" }, lowres, "Mostra (es. “SD Connessioni lente”)"),
      ...field("Film dall'anno", movieYear), ...field("Serie dall'anno", seriesYear),
      el("label", {}, "Titoli senza anno"), el("label", { className: "check" }, unknownYear, "Nascondi quando c'è un anno minimo"));
    form.addEventListener("input", preview);
    form.addEventListener("change", preview);
    const save = button("Salva le regole", async () => {
      save.disabled = true;
      try { await api("PUT", "/api/v1/subscriptions/" + sub.id + "/rules", readRules()); flash("Regole salvate e applicate"); render(); }
      catch (e) { flash(e.message); save.disabled = false; }
    }, "primary");
    main.append(form, table, el("div", { className: "buttons" }, save));
  }

  // ---- Categories ----
  async function renderCategories(main) {
    const sub = await pickSubscription(main, "Categorie",
      "Includi o escludi le categorie una per una e scegli in quale sezione della TV vanno i canali. “Automatico” segue le regole della scheda Selezione.");
    if (!sub) return;
    const kinds = el("div", { className: "toolbar" }, ...[["live", "Canali"], ["movie", "Film"], ["series", "Serie"]].map(([k, label]) => {
      const b = button(label, () => { state.kind = k; render(); }, state.kind === k ? "primary" : "");
      return b;
    }));
    const filter = input({ type: "search", placeholder: "Filtra per nome", className: "pick focusable" });
    kinds.append(filter);
    const cats = await api("GET", "/api/v1/subscriptions/" + sub.id + "/categories?kind=" + state.kind);
    const table = el("table", { className: "cats" });
    const head = el("tr", {}, el("th", {}, "Categoria"), el("th", {}, "Titoli"), el("th", {}, "Inclusa"),
      state.kind === "live" ? el("th", {}, "Sezione") : null, el("th", {}, "Stato"));
    const rows = cats.map((c) => {
      const include = el("select", { className: "focusable" }, el("option", { value: "" }, "Automatico"),
        el("option", { value: "true" }, "Sempre"), el("option", { value: "false" }, "Mai"));
      include.value = c.enabled_override == null ? "" : String(c.enabled_override);
      let section = null;
      if (state.kind === "live") {
        section = el("select", { className: "focusable" }, el("option", { value: "" }, "Automatico (" + ({ tv: "TV", sport: "Sport", radio: "Radio" }[c.effective.section || c.section] || "TV") + ")"),
          el("option", { value: "tv" }, "TV"), el("option", { value: "sport" }, "Sport"), el("option", { value: "radio" }, "Radio"));
        section.value = c.section_override || "";
      }
      const status = el("td", {}, c.effective.enabled ? el("span", { className: "ok" }, "Inclusa") : el("span", { className: "why" }, c.effective.reason || "Esclusa"));
      const row = el("tr", { className: c.effective.enabled ? "" : "off" }, el("td", { className: "name" }, c.name),
        el("td", { className: "n" }, c.items.toLocaleString("it-IT")), el("td", {}, include), section ? el("td", {}, section) : null, status);
      row.dataset.name = c.name.toLowerCase();
      const save = async () => {
        const body = { enabled: include.value === "" ? null : include.value === "true", section: section && section.value ? section.value : null,
          country: c.country_override == null ? null : c.country_override, adult: c.adult_override == null ? null : c.adult_override };
        try {
          await api("PUT", "/api/v1/subscriptions/" + sub.id + "/categories/" + state.kind + "/" + encodeURIComponent(c.id), body);
          const fresh = (await api("GET", "/api/v1/subscriptions/" + sub.id + "/categories?kind=" + state.kind)).find((x) => x.id === c.id);
          if (fresh) {
            row.className = fresh.effective.enabled ? "" : "off";
            status.replaceChildren(fresh.effective.enabled ? el("span", { className: "ok" }, "Inclusa") : el("span", { className: "why" }, fresh.effective.reason || "Esclusa"));
          }
          flash("Salvato: " + c.name);
        } catch (e) { flash(e.message); }
      };
      include.onchange = save;
      if (section) section.onchange = save;
      return row;
    });
    table.append(head, ...rows);
    filter.oninput = () => {
      const q = filter.value.trim().toLowerCase();
      for (const r of rows) r.hidden = q && !r.dataset.name.includes(q);
    };
    const included = cats.filter((c) => c.effective.enabled).length;
    main.append(kinds, el("p", { className: "note" }, included + " categorie incluse su " + cats.length), table);
  }

  // ---- System: problems and schedule ----
  async function renderSystem(main) {
    const status = await api("GET", "/api/v1/status");
    const parts = [el("h2", {}, "Stato")];
    if (!status.problems.length) parts.push(el("p", { className: "ok" }, "✓ Nessun problema: VPN collegate, abbonamenti attivi, guide aggiornate."));
    for (const p of status.problems) parts.push(el("p", { className: p.level === "error" ? "bad" : "" }, "⚠ " + p.text));
    const sc = status.schedule;
    const hour = el("select", { className: "focusable" }, el("option", { value: -1 }, "Mai (solo a mano)"),
      ...Array.from({ length: 24 }, (_, h) => el("option", { value: h }, String(h).padStart(2, "0") + ":00")));
    hour.value = sc.catalog_hour;
    const guide = el("select", { className: "focusable" }, el("option", { value: 0 }, "Mai (solo con il catalogo)"),
      ...[1, 2, 3, 4, 6, 8, 12, 24].map((h) => el("option", { value: h }, "Ogni " + h + (h === 1 ? " ora" : " ore"))));
    guide.value = sc.guide_hours;
    const save = el("button", { className: "small primary focusable full", type: "submit" }, "Salva");
    const form = el("form", { className: "form" }, ...field("Catalogo, ogni notte alle", hour), ...field("Guida TV", guide), save);
    form.onsubmit = async (e) => {
      e.preventDefault();
      try {
        await api("PUT", "/api/v1/settings/schedule", { catalog_hour: Number(hour.value), guide_hours: Number(guide.value) });
        flash("Pianificazione salvata");
        render();
      } catch (err) { flash(err.message); }
    };
    parts.push(el("h2", {}, "Aggiornamenti automatici"),
      el("p", { className: "note" }, "Il catalogo di ogni abbonamento (con la guida) si aggiorna ogni notte; se non riesce riprova ogni ora. La guida si aggiorna anche tra una notte e l'altra." +
        (status.next_catalog ? " Prossimo aggiornamento del catalogo: " + when(status.next_catalog) + "." : "")), form);
    parts.push(...(await metadataSection()).filter(Boolean), ...await backupSection());
    main.replaceChildren(...parts);
  }

  // ---- Covers and details (TMDB) ----
  async function metadataSection() {
    const m = await api("GET", "/api/v1/settings/metadata");
    const key = input({ type: "password", autocomplete: "off", spellcheck: false,
      placeholder: m.tmdb ? "configurata: incolla una nuova chiave per sostituirla" : "Chiave API (v3) o token di accesso in lettura (v4)" });
    const save = el("button", { className: "small primary focusable full", type: "submit" }, "Salva la chiave");
    const form = el("form", { className: "form" }, ...field("Chiave TMDB", key), save);
    form.onsubmit = async (e) => {
      e.preventDefault();
      save.disabled = true;
      try { await api("PUT", "/api/v1/settings/metadata", { tmdb_key: key.value }); flash("Chiave TMDB verificata e salvata"); render(); }
      catch (err) { flash(err.message); save.disabled = false; }
    };
    const remove = m.tmdb ? button("Togli la chiave", async () => {
      try { await api("PUT", "/api/v1/settings/metadata", { tmdb_key: "" }); flash("Chiave tolta"); render(); } catch (e) { flash(e.message); }
    }, "danger") : null;
    return [el("h2", {}, "Copertine e schede (TMDB)"),
      el("p", { className: "note" }, "Le schede di film e serie vengono dal provider. Con una chiave gratuita di The Movie Database si aggiungono " +
        "la locandina dove il provider non ce l'ha o l'ha rotta, la trama in italiano quando manca, lo sfondo e il cast. " +
        "La chiave si crea su themoviedb.org → Impostazioni → API. Resta cifrata sul server e non viene mai mostrata."),
      el("p", {}, m.tmdb ? el("span", { className: "ok" }, "✓ TMDB attivo") : el("span", { className: "note" }, "TMDB non configurato")),
      form, remove ? el("div", { className: "buttons" }, remove) : null];
  }

  // ---- Backup ----
  async function download(method, path, body, headers = {}) {
    const res = await fetch(path, { method, body, headers: { Authorization: "Bearer " + window.VodMax.token(), ...headers } });
    if (!res.ok) {
      const data = await res.json().catch(() => null);
      throw new Error(data && data.error ? data.error : "errore " + res.status);
    }
    const name = (/filename="([^"]+)"/.exec(res.headers.get("Content-Disposition") || "") || [])[1] || "vodmax.vodmaxbackup";
    const url = URL.createObjectURL(await res.blob());
    const a = el("a", { href: url, download: name });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10000);
  }

  async function restoreFrom(send) {
    if (!confirm("Il ripristino sostituisce abbonamenti, VPN, regole, preferiti, avanzamenti e registrazioni programmate con quelli del backup. " +
      "Prima viene fatta una copia automatica dello stato attuale. Continuare?")) return;
    try {
      await send();
      flash("Ripristino fatto: i cataloghi si stanno scaricando di nuovo (circa un minuto per abbonamento)");
      render();
    } catch (e) { flash(e.message); }
  }

  async function backupSection() {
    const pass = input({ type: "password", autocomplete: "new-password", placeholder: "almeno 8 caratteri" });
    const pass2 = input({ type: "password", autocomplete: "new-password" });
    const make = el("button", { className: "small primary focusable full", type: "submit" }, "Scarica il backup");
    const exportForm = el("form", { className: "form" }, ...field("Password", pass), ...field("Ripeti la password", pass2), make);
    exportForm.onsubmit = async (e) => {
      e.preventDefault();
      if (pass.value !== pass2.value) { flash("Le due password non coincidono"); return; }
      make.disabled = true;
      try { await download("POST", "/api/v1/backup", JSON.stringify({ password: pass.value }), { "Content-Type": "application/json" }); flash("Backup scaricato"); }
      catch (err) { flash(err.message); }
      make.disabled = false;
    };
    const file = input({ type: "file", accept: ".vodmaxbackup" });
    const rpass = input({ type: "password", autocomplete: "off" });
    const restoreButton = el("button", { className: "small focusable full", type: "submit" }, "Ripristina da questo file");
    const importForm = el("form", { className: "form" }, ...field("File", file), ...field("Password del backup", rpass), restoreButton);
    importForm.onsubmit = (e) => {
      e.preventDefault();
      if (!file.files[0]) { flash("Scegli il file del backup"); return; }
      restoreFrom(async () => {
        const res = await fetch("/api/v1/restore", { method: "POST", body: file.files[0],
          headers: { Authorization: "Bearer " + window.VodMax.token(), "X-Backup-Password": rpass.value } });
        if (!res.ok) { const d = await res.json().catch(() => null); throw new Error(d && d.error ? d.error : "errore " + res.status); }
      });
    };
    const list = await api("GET", "/api/v1/backups");
    const rows = list.map((b) => el("tr", {}, el("td", {}, when(b.created)), el("td", { className: "n" }, Math.max(1, Math.round(b.size / 1024)) + " kB"),
      el("td", {}, button("Scarica", () => download("GET", "/api/v1/backups/" + b.name).catch((e) => flash(e.message))),
        " ", button("Ripristina", () => restoreFrom(() => api("POST", "/api/v1/backups/" + b.name + "/restore"))))));
    const now = button("Crea una copia adesso", async () => {
      try { await api("POST", "/api/v1/backups"); flash("Copia creata"); render(); } catch (e) { flash(e.message); }
    });
    return [
      el("h2", {}, "Backup"),
      el("p", { className: "note" }, "Contiene abbonamenti e VPN (con le credenziali), regole di selezione, categorie, titoli nascosti, preferiti, " +
        "avanzamenti, registrazioni programmate e impostazioni. Non contiene il catalogo né i file registrati: il catalogo si riscarica da solo dopo il ripristino."),
      el("h3", {}, "Backup da conservare altrove"),
      el("p", { className: "note" }, "Cifrato con una password tua: si può ripristinare anche su un'altra installazione. Senza la password non si apre: non c'è modo di recuperarla."),
      exportForm,
      el("h3", {}, "Ripristina da un file"),
      importForm,
      el("h3", {}, "Copie automatiche"),
      el("p", { className: "note" }, "Una al giorno, le ultime 14, sul server. Si aprono solo su questa installazione: per un guasto del disco serve il backup con password."),
      list.length ? el("table", { className: "cats" }, el("tr", {}, el("th", {}, "Creata"), el("th", {}, "Dimensione"), el("th", {}, "")), ...rows)
        : el("p", { className: "note" }, "Nessuna copia ancora."),
      el("div", { className: "buttons" }, now),
    ];
  }

  // ---- Profiles ----
  const KINDS = [["admin", "Amministratore"], ["standard", "Normale"], ["kids", "Bambini"]];
  async function renderProfiles(main) {
    const list = await api("GET", "/api/v1/profiles");
    const parts = [el("h2", {}, "Profili"),
      el("p", { className: "note" }, "Ogni profilo ha i suoi preferiti e il suo “Continua a guardare”. I profili Bambini non vedono mai contenuti per adulti " +
        "e non programmano registrazioni. Solo gli amministratori aprono le impostazioni: dai loro un PIN se in casa ci sono bambini.")];
    for (const p of list) {
      const box = el("section", { className: "box" }, el("h3", {}, p.name), el("div", { className: "facts" },
        el("span", {}, "Tipo ", el("b", {}, KINDS.find((k) => k[0] === p.kind)[1])),
        el("span", {}, "PIN ", el("b", {}, p.has_pin ? "sì" : "no"))));
      const edit = button("Modifica", () => { edit.disabled = true; box.append(profileForm(p)); });
      const del = button("Elimina", async () => {
        if (!confirm("Eliminare il profilo “" + p.name + "” con i suoi preferiti e i suoi avanzamenti?")) return;
        try { await api("DELETE", "/api/v1/profiles/" + p.id); flash("Profilo eliminato"); } catch (e) { flash(e.message); }
        render();
      }, "danger");
      box.append(el("div", { className: "buttons" }, edit, del));
      parts.push(box);
    }
    parts.push(el("h2", {}, "Nuovo profilo"), profileForm());
    main.replaceChildren(...parts);
  }

  function profileForm(existing) {
    const name = input({ value: existing ? existing.name : "", placeholder: "es. Giulia" });
    const kind = el("select", { className: "focusable" }, ...KINDS.map(([v, l]) => el("option", { value: v }, l)));
    kind.value = existing ? existing.kind : "standard";
    const pin = input({ type: "password", inputMode: "numeric", autocomplete: "new-password", maxLength: 8,
      placeholder: existing && existing.has_pin ? "lascia vuoto per non cambiarlo" : "facoltativo, 4-8 cifre" });
    const removePin = input({ type: "checkbox" });
    const save = el("button", { className: "small primary focusable full", type: "submit" }, existing ? "Salva" : "Crea il profilo");
    const form = el("form", { className: "form" }, ...field("Nome", name), ...field("Tipo", kind), ...field("PIN", pin),
      existing && existing.has_pin ? el("label", {}, "") : null, existing && existing.has_pin ? el("label", { className: "check" }, removePin, "Togli il PIN") : null, save);
    form.onsubmit = async (e) => {
      e.preventDefault();
      const body = { name: name.value, kind: kind.value };
      if (pin.value) body.pin = pin.value;
      else if (removePin.checked) body.pin = "";
      try {
        if (existing) await api("PATCH", "/api/v1/profiles/" + existing.id, body);
        else await api("POST", "/api/v1/profiles", body);
        flash(existing ? "Profilo aggiornato" : "Profilo creato");
        render();
      } catch (err) { flash(err.message); }
    };
    return form;
  }

  // ---- Devices ----
  async function renderDevices(main) {
    const [devices, profiles] = await Promise.all([api("GET", "/api/v1/devices"), api("GET", "/api/v1/profiles")]);
    const profileName = Object.fromEntries(profiles.map((p) => [p.id, p.name]));
    const lockSelect = (value) => {
      const s = el("select", { className: "focusable" }, el("option", { value: "" }, "Tutti i profili (si sceglie all'avvio)"),
        ...profiles.map((p) => el("option", { value: p.id }, "Solo " + p.name)));
      s.value = value == null ? "" : value;
      return s;
    };
    const code = input({ placeholder: "es. K7Q-29M", autocomplete: "off", spellcheck: false, maxLength: 7 });
    const name = input({ placeholder: "es. TV salotto" });
    const lock = lockSelect(null);
    const pairButton = el("button", { className: "small primary focusable full", type: "submit" }, "Abbina");
    const pairForm = el("form", { className: "form" }, ...field("Codice mostrato", code), ...field("Nome del dispositivo", name), ...field("Profili", lock), pairButton);
    pairForm.onsubmit = async (e) => {
      e.preventDefault();
      try {
        await api("POST", "/api/v1/pair/approve", { code: code.value, name: name.value, profile_lock: lock.value ? Number(lock.value) : null });
        flash("Dispositivo abbinato: entro pochi secondi si apre da solo");
        render();
      } catch (err) { flash(err.message); }
    };
    const rows = devices.map((d) => {
      const sel = lockSelect(d.profile_lock);
      sel.onchange = async () => {
        try { await api("PATCH", "/api/v1/devices/" + d.id, { name: d.name, profile_lock: sel.value ? Number(sel.value) : null }); flash("Salvato"); }
        catch (e) { flash(e.message); }
      };
      return el("tr", {}, el("td", { className: "name" }, d.name), el("td", {}, d.last_seen ? when(d.last_seen) : "mai"), el("td", {}, sel),
        el("td", {}, button("Revoca", async () => {
          if (!confirm("Revocare “" + d.name + "”? Dovrà essere abbinato di nuovo.")) return;
          try { await api("DELETE", "/api/v1/devices/" + d.id); flash("Dispositivo revocato"); } catch (e) { flash(e.message); }
          render();
        }, "danger")));
    });
    main.replaceChildren(
      el("h2", {}, "Abbina un dispositivo"),
      el("p", { className: "note" }, "Apri VodMax sul nuovo dispositivo (TV, telefono, PC): mostra un codice. Inseriscilo qui. " +
        "Con “Solo …” il dispositivo usa sempre quel profilo, per esempio il tablet dei bambini."),
      pairForm,
      el("h2", {}, "Dispositivi abbinati"),
      devices.length ? el("table", { className: "cats" }, el("tr", {}, el("th", {}, "Nome"), el("th", {}, "Ultimo uso"), el("th", {}, "Profili"), el("th", {}, "")), ...rows)
        : el("p", { className: "note" }, "Nessun dispositivo abbinato: per ora si entra con il token di amministrazione."),
      el("p", { className: "note" }, "Profili: " + Object.values(profileName).join(", ")));
  }

  for (const t of document.querySelectorAll(".tab[data-view]")) t.onclick = () => { state.view = t.dataset.view; render(); };
  window.VodMax.withToken(async () => {
    const who = await window.VodMax.me().catch(() => null);
    if (!who) return;
    if (!who.admin) {
      $("content").replaceChildren(el("h2", {}, "Impostazioni"),
        el("p", { className: "note" }, "Le impostazioni sono riservate ai profili amministratore."),
        button("Cambia profilo", () => { store.set("profile", ""); window.VodMax.showProfiles(); }, "primary"));
      for (const t of document.querySelectorAll(".tab[data-view]")) t.hidden = true;
      return;
    }
    render();
  });
})();
