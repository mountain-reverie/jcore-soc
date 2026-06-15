// Renders ECP5 board synth trends from github-action-benchmark BENCHMARK_DATA.
// One line per metric over commit date, one series per board (by the metric's
// "<board>/..." name). Dashed budget lines for the 85F caps. Adapted+trimmed
// from jcore-cpu dashboard/app.js (board-keyed, no variant overlay).

window.__SIZE__ = null;
window.__SPEED__ = null;

// 85F resource budgets keyed by the metric base name suffix.
var BUDGET = {
  "LUT4": 83640,
  "DP16KD": 208,
  "MULT18X18D": 156,
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

function boardOf(extra, name) {
  if (extra) return extra;
  // fall back to the "<board>/..." prefix after the " · "
  var parts = (name || "").split(" · ");
  var tail = parts.length > 1 ? parts[1] : name;
  return (tail || "").split("/")[0];
}

// -> { metricName: { board: [ {x: date, y: value} ] } }
function seriesByName(data) {
  var out = {};
  if (!data || !data.entries) return out;
  Object.keys(data.entries).forEach(function (suite) {
    data.entries[suite].forEach(function (run) {
      run.benches.forEach(function (b) {
        var board = boardOf(b.extra, b.name);
        if (b.unit) METRIC_UNIT[b.name] = b.unit;
        if (!out[b.name]) out[b.name] = {};
        if (!out[b.name][board]) out[b.name][board] = [];
        out[b.name][board].push({ x: run.date, y: b.value });
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
  var boards = Object.keys(boardMap).sort();
  var datasets = boards.map(function (board, i) {
    var pts = boardMap[board].slice().sort(function (a, b) { return a.x - b.x; });
    var color = BOARD_COLOR[i % BOARD_COLOR.length];
    return { label: board, data: pts, borderColor: color, backgroundColor: color,
             tension: 0.2, pointRadius: 3 };
  });
  var budget = BUDGET[metricSuffix(name)];
  if (budget) {
    var xs = [];
    datasets.forEach(function (d) { d.data.forEach(function (p) { xs.push(p.x); }); });
    if (xs.length) {
      datasets.push({ label: "85F budget (" + budget + ")",
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
  var grid = document.getElementById("trends-ecp5");
  var any = false;
  Object.keys(all).sort().forEach(function (name) {
    any = true;
    lineCard(grid, name, all[name]);
  });
  if (any) document.getElementById("fpga-section").hidden = false;
  renderLatest(all);
}

function renderLatest(all) {
  var rows = {}, boards = {};
  Object.keys(all).forEach(function (name) {
    Object.keys(all[name]).forEach(function (board) {
      var pts = all[name][board];
      if (!pts || !pts.length) return;
      var sorted = pts.slice().sort(function (a, b) { return a.x - b.x; });
      var last = sorted[sorted.length - 1];
      boards[board] = true;
      (rows[name] = rows[name] || {})[board] = last.y;
    });
  });
  var blist = Object.keys(boards).sort();
  var html = "<table border=1 cellpadding=4 style='border-collapse:collapse'><tr><th>metric</th>" +
    blist.map(function (b) { return "<th>" + htmlEsc(b) + "</th>"; }).join("") + "</tr>";
  Object.keys(rows).sort().forEach(function (name) {
    html += "<tr><td>" + htmlEsc(name) + "</td>" +
      blist.map(function (b) { return "<td>" + (rows[name][b] != null ? rows[name][b] : "—") + "</td>"; }).join("") +
      "</tr>";
  });
  html += "</table>";
  document.getElementById("latest").innerHTML = html;
}

boot();
