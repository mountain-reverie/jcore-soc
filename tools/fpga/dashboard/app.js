// Renders ECP5 board synth trends from github-action-benchmark BENCHMARK_DATA.
// One line per metric over commit date, one series per board (by the metric's
// "<board>/..." name). Dashed budget lines for the 85F caps. Adapted+trimmed
// from jcore-cpu dashboard/app.js (board-keyed, no variant overlay).

if (window.__SIZE__ === undefined) window.__SIZE__ = null;
if (window.__SPEED__ === undefined) window.__SPEED__ = null;

// Resource budgets keyed by the metric base name suffix.
var BUDGET = {
  "LUT4": 83640,
  "DP16KD": 208,
  "MULT18X18D": 156,
  "LC": 5280,
  "RAM": 30,
};
var BOARD_COLOR = ["#1f77b4", "#2ca02c", "#e6b800", "#d62728"]; // assigned per board

// Unit (MHz/cells) per full metric name, captured from the canonical `unit`
// field as data is read (so we don't infer it from the name).
var METRIC_UNIT = {};

function htmlEsc(s) {
  return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function loadData(src) {
  return new Promise(function (resolve, reject) {
    var s = document.createElement("script");
    s.src = src;
    s.onload = function () { resolve(window.BENCHMARK_DATA); };
    s.onerror = function () { reject(new Error("failed to load " + src)); };
    document.head.appendChild(s);
  });
}

function boot() {
  loadData("./bench-size/data.js")
    .then(function (d) { window.__SIZE__ = d; }, function () {})
    .then(function () { return loadData("./bench-speed/data.js"); })
    .then(function (d) { window.__SPEED__ = d; }, function () {})
    .then(function () {
      if (window.__SIZE__ || window.__SPEED__) return;
      return loadData("./fixtures/data.js").then(function (d) {
        window.__SIZE__ = d; window.__SPEED__ = d;
      });
    })
    .then(render)
    .catch(function (e) { console.error("no benchmark data available", e); });
}

// Variant suffix: "ulx3s [j4-rom]/LUT4" (after the " · ") -> "j4-rom"; "" if none.
function variantOf2(name) {
  var m = /\[([^\]]+)\]\//.exec(name || "");
  return m ? m[1] : "";
}
// Chart key: strip the " [variant]" so all variants of a board+metric group on one chart.
function baseName2(name) {
  return (name || "").replace(/\s\[[^\]]+\]\//, "/");
}

// FPGA family from the metric's target prefix ("ecp5-lfe5u-85f · …" -> "ecp5").
function familyOf(name) {
  var target = (name || "").split(" · ")[0];
  if (/^ecp5/.test(target)) return "ecp5";
  if (/^ice40/.test(target)) return "ice40";
  return "";
}

// Line label within a chart: the variant, or the board when there is no variant.
// Legacy bare-"ulx3s" metrics predate the variant suffix and ARE the j2-direct
// design, so fold them into the "j2-direct" line -> one continuous series.
function seriesLabel(name, extra) {
  var v = variantOf2(name);
  if (v) return v;
  var board = boardOf(extra, name);
  if (board === "ulx3s") return "j2-direct";
  return board;
}

function boardOf(extra, name) {
  if (extra) return extra;
  // fall back to the "<board>/..." prefix after the " · "
  var parts = (name || "").split(" · ");
  var tail = parts.length > 1 ? parts[1] : name;
  return (tail || "").split("/")[0];
}

// -> { baseName: { seriesLabel: [ {x: date, y: value} ] } }
function seriesByName(data) {
  var out = {};
  if (!data || !data.entries) return out;
  Object.keys(data.entries).forEach(function (suite) {
    data.entries[suite].forEach(function (run) {
      run.benches.forEach(function (b) {
        var chart = baseName2(b.name);
        var line = seriesLabel(b.name, b.extra);
        if (b.unit) METRIC_UNIT[chart] = b.unit;
        if (!out[chart]) out[chart] = {};
        if (!out[chart][line]) out[chart][line] = [];
        out[chart][line].push({ x: run.date, y: b.value });
      });
    });
  });
  return out;
}

function metricSuffix(name) {
  // "ecp5-lfe5u-85f · ulx3s/LUT4" -> "LUT4"
  var slash = name.lastIndexOf("/");
  return slash >= 0 ? name.slice(slash + 1) : name;
}

function lineCard(parent, name, boardMap) {
  var card = document.createElement("div"); card.className = "card";
  var cv = document.createElement("canvas"); card.appendChild(cv); parent.appendChild(card);
  var lines = Object.keys(boardMap).sort();
  var datasets = lines.map(function (line, i) {
    var pts = boardMap[line].slice().sort(function (a, b) { return a.x - b.x; });
    var color = BOARD_COLOR[i % BOARD_COLOR.length];
    return { label: line, data: pts, borderColor: color, backgroundColor: color,
             tension: 0.2, pointRadius: 3 };
  });
  var budget = BUDGET[metricSuffix(name)];
  if (budget) {
    var xs = [];
    datasets.forEach(function (d) { d.data.forEach(function (p) { xs.push(p.x); }); });
    if (xs.length) {
      datasets.push({ label: "budget (" + budget + ")",
        data: [{ x: Math.min.apply(null, xs), y: budget },
               { x: Math.max.apply(null, xs), y: budget }],
        borderColor: "#999", borderDash: [6, 4], pointRadius: 0, fill: false, tension: 0 });
    }
  }
  var unit = METRIC_UNIT[name] || (name.indexOf("Fmax") >= 0 ? "MHz" : "cells");
  new Chart(cv, {
    type: "line", data: { datasets: datasets },
    options: {
      parsing: false,
      scales: { x: { type: "linear",
        ticks: { callback: function (v) { return new Date(v).toISOString().slice(0, 10); } } } },
      plugins: {
        legend: { display: true },
        title: { display: true, text: metricSuffix(name) + " (" + unit + ")" },
        tooltip: { callbacks: { title: function (it) {
          return new Date(it[0].parsed.x).toISOString().slice(0, 10); } } }
      }
    }
  });
}

function render() {
  var all = Object.assign({}, seriesByName(window.__SIZE__), seriesByName(window.__SPEED__));
  var grids = { ecp5: document.getElementById("trends-ecp5"),
                ice40: document.getElementById("trends-ice40") };
  var seen = { ecp5: false, ice40: false };
  Object.keys(all).sort().forEach(function (name) {
    var fam = familyOf(name);
    var grid = grids[fam];
    if (!grid) return;
    seen[fam] = true;
    lineCard(grid, name, all[name]);
  });
  if (seen.ecp5) document.getElementById("sec-ecp5").hidden = false;
  if (seen.ice40) document.getElementById("sec-ice40").hidden = false;
  renderLatest(all);
}

function renderLatest(all) {
  renderLatestFamily(all, "ecp5", "latest-ecp5");
  renderLatestFamily(all, "ice40", "latest-ice40");
}

function renderLatestFamily(all, fam, elId) {
  var el = document.getElementById(elId);
  if (!el) return;
  var rows = {}, cols = {};
  Object.keys(all).forEach(function (name) {
    if (familyOf(name) !== fam) return;
    Object.keys(all[name]).forEach(function (line) {
      var pts = all[name][line];
      if (!pts || !pts.length) return;
      var sorted = pts.slice().sort(function (a, b) { return a.x - b.x; });
      var last = sorted[sorted.length - 1];
      cols[line] = true;
      (rows[name] = rows[name] || {})[line] = last.y;
    });
  });
  var clist = Object.keys(cols).sort();
  if (!clist.length) { el.innerHTML = ""; return; }
  var html = "<table border=1 cellpadding=4 style='border-collapse:collapse'><tr><th>metric</th>" +
    clist.map(function (c) { return "<th>" + htmlEsc(c) + "</th>"; }).join("") + "</tr>";
  Object.keys(rows).sort().forEach(function (name) {
    html += "<tr><td>" + htmlEsc(name) + "</td>" +
      clist.map(function (c) { return "<td>" + (rows[name][c] != null ? rows[name][c] : "—") + "</td>"; }).join("") +
      "</tr>";
  });
  html += "</table>";
  el.innerHTML = html;
}

boot();
