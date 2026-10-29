// VodMax for LG webOS: remembers the server address, checks that the
// server answers, then opens its interface (the same as in a browser).
// Written for old webOS browsers too (no async/await, no fetch).
(function () {
  "use strict";
  var KEY = "vodmax.server";
  var $ = function (id) { return document.getElementById(id); };

  function normalize(s) {
    s = (s || "").replace(/^\s+|\s+$/g, "");
    if (!s) return null;
    if (!/^https?:\/\//.test(s)) s = "http://" + s;
    return s.replace(/\/+$/, "");
  }

  function check(base, done) {
    var x = new XMLHttpRequest();
    var finished = false;
    var end = function (ok) { if (!finished) { finished = true; done(ok); } };
    x.open("GET", base + "/api/v1/health", true);
    x.timeout = 6000;
    x.onload = function () { end(x.status === 200); };
    x.onerror = x.ontimeout = function () { end(false); };
    x.send();
  }

  function open(base) {
    $("message").textContent = "Collegamento a " + base + "…";
    $("message").className = "";
    check(base, function (ok) {
      if (ok) {
        window.location.replace(base + "/");
        return;
      }
      showForm(base, "Il server non risponde a " + base + ": controlla che sia acceso e nella stessa rete.");
    });
  }

  function showForm(value, error) {
    $("form").hidden = false;
    $("retry").hidden = !value;
    $("server").value = value || "";
    $("message").textContent = error || "Inserisci l'indirizzo del server.";
    $("message").className = error ? "bad" : "";
    $("server").focus();
  }

  $("form").addEventListener("submit", function (e) {
    e.preventDefault();
    var base = normalize($("server").value);
    if (!base) return;
    try { localStorage.setItem(KEY, base); } catch (err) { /* private storage off */ }
    open(base);
  });
  $("retry").addEventListener("click", function () { open(normalize($("server").value)); });

  // Remote: Up/Down between the field and the buttons.
  document.addEventListener("keydown", function (e) {
    var order = [$("server"), $("connect"), $("retry")].filter(function (el) { return !el.hidden; });
    var i = order.indexOf(document.activeElement);
    if (e.keyCode === 40 && i < order.length - 1) { order[i + 1].focus(); e.preventDefault(); }
    if (e.keyCode === 38 && i > 0) { order[i - 1].focus(); e.preventDefault(); }
  });

  var saved = null;
  try { saved = localStorage.getItem(KEY); } catch (err) { /* nothing saved */ }
  if (saved) open(saved);
  else showForm("", null);
})();
