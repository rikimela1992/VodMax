// Interface languages. Texts are written in Italian in the code and shown
// through t("Testo con {nome}", { nome }); each language adds its
// dictionary (i18n/<lang>-*.js, keyed by the Italian text). A text without
// a translation stays in Italian.
//
// Static text in the pages: an element with data-i18n has its text nodes
// translated; data-i18n-attr="title,placeholder,aria-label" translates
// those attributes.
"use strict";

(() => {
  const LANGS = { it: "Italiano", en: "English" };
  const LOCALES = { it: "it-IT", en: "en-GB" };
  const KEY = "vodmax.lang";
  const dicts = {};

  let saved = null;
  try { saved = JSON.parse(localStorage.getItem(KEY)); } catch { /* private mode */ }
  const device = String(navigator.language || "").slice(0, 2).toLowerCase();
  // Saved choice, else the device's language, else English.
  const lang = LANGS[saved] ? saved : LANGS[device] ? device : "en";
  document.documentElement.lang = lang;

  function t(text, vars) {
    let s = (dicts[lang] && dicts[lang][text]) || text;
    if (vars) s = s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
    return s;
  }

  function add(code, entries) { dicts[code] = Object.assign(dicts[code] || {}, entries); }

  function setLang(code) {
    if (!LANGS[code] || code === lang) return;
    try { localStorage.setItem(KEY, JSON.stringify(code)); } catch { /* private mode */ }
    location.reload();
  }

  // Translates the static text of root (the page, or a part added later).
  function apply(root = document) {
    for (const el of root.querySelectorAll("[data-i18n]")) {
      const walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      for (let n = walk.nextNode(); n; n = walk.nextNode()) {
        const text = n.nodeValue.trim();
        if (!text) continue;
        if (n.vodmaxKey === undefined) n.vodmaxKey = text;
        n.nodeValue = n.nodeValue.replace(text, t(n.vodmaxKey));
      }
    }
    for (const el of root.querySelectorAll("[data-i18n-attr]")) {
      for (const a of el.dataset.i18nAttr.split(",")) {
        const name = a.trim(), key = "i18n" + name.replace(/(^|-)(\w)/g, (m, d, c) => c.toUpperCase());
        if (!el.hasAttribute(name)) continue;
        if (el.dataset[key] === undefined) el.dataset[key] = el.getAttribute(name);
        el.setAttribute(name, t(el.dataset[key]));
      }
    }
    if (root === document && document.title) document.title = t(document.title);
  }

  // A row of buttons to choose the language (settings, profile choice).
  function picker(className = "") {
    const row = document.createElement("div");
    row.className = "lang-picker " + className;
    for (const [code, name] of Object.entries(LANGS)) {
      const b = document.createElement("button");
      b.type = "button";
      b.className = "focusable" + (code === lang ? " active" : "");
      b.lang = code;
      b.textContent = (code === lang ? "✓ " : "") + name;
      b.onclick = () => setLang(code);
      row.append(b);
    }
    return row;
  }

  // Dictionaries load after this file: translate when the page is parsed.
  document.addEventListener("DOMContentLoaded", () => apply());

  window.VodMaxI18n = { lang, locale: LOCALES[lang], langs: LANGS, t, add, setLang, apply, picker };
})();
