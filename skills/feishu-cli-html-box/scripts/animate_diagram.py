#!/usr/bin/env python3
"""Generate a self-contained animated diagram HTML from pattern JSON."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


HTML_TEMPLATE = r"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>__TITLE__</title>
<style>
  :root {
    --background: oklch(0.995 0.002 270);
    --foreground: oklch(0.18 0.014 260);
    --card: oklch(1 0 0);
    --muted: oklch(0.965 0.005 260);
    --muted-foreground: oklch(0.52 0.014 260);
    --border: oklch(0.92 0.005 260);
    --brand: oklch(0.55 0.18 258);
    --brand-foreground: oklch(0.985 0.005 258);
    --brand-soft: oklch(0.95 0.04 258);
    --brand-ring: oklch(0.55 0.18 258 / 0.18);
    --warning: oklch(0.65 0.14 70);
    --font-sans: system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    --font-mono: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  }

  * { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; overflow: hidden; min-height: 100%; }
  body {
    background: var(--background);
    color: var(--foreground);
    font-family: var(--font-sans);
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
    display: flex;
    justify-content: center;
    align-items: center;
    min-height: 100vh;
    padding: clamp(8px, 2vw, 22px);
  }

  .viz {
    width: 100%;
    max-width: min(1180px, calc(100vw - 2 * clamp(8px, 2vw, 22px)));
    display: flex;
    flex-direction: column;
    gap: clamp(8px, 1.2vw, 14px);
    border: 1px solid var(--border);
    border-radius: 14px;
    background: var(--card);
    padding: clamp(10px, 1.5vw, 18px);
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.04);
  }

  .viz-head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .badge {
    display: inline-flex; align-items: center;
    border-radius: 999px;
    background: var(--brand-soft);
    color: var(--brand);
    border: 1px solid color-mix(in oklch, var(--brand) 30%, transparent);
    padding: 2px 10px;
    font-size: 11px; font-weight: 600;
    letter-spacing: 0.02em;
  }
  .viz-title { font-size: 13px; font-weight: 600; color: var(--foreground); }
  .viz-sub { font-size: 12px; color: var(--muted-foreground); }

  .canvas-wrap {
    position: relative;
    width: 100%;
    aspect-ratio: 900 / 420;
    overflow: hidden;
    border-radius: 12px;
    border: 1px solid var(--border);
    background: var(--card);
  }
  .corner {
    position: absolute; top: 12px;
    display: inline-flex; align-items: center; gap: 6px;
    font-family: var(--font-mono);
    font-size: 9px; letter-spacing: 0.16em; text-transform: uppercase;
    color: var(--muted-foreground);
    z-index: 2;
  }
  .corner.left { left: 12px; }
  .corner.right { right: 12px; }
  .live-dot { position: relative; display: inline-flex; width: 8px; height: 8px; }
  .live-dot .ping {
    position: absolute; inset: 0; border-radius: 999px;
    background: var(--brand); opacity: 0.75;
    animation: ping 1.4s cubic-bezier(0,0,0.2,1) infinite;
  }
  .live-dot.paused .ping { animation: none; opacity: 0; }
  .live-dot .core { position: relative; width: 8px; height: 8px; border-radius: 999px; background: var(--brand); }
  @keyframes ping { 75%, 100% { transform: scale(2); opacity: 0; } }

  .caption-wrap { display: flex; flex-direction: column; gap: 4px; min-height: 32px; }
  .step-no {
    font-family: var(--font-mono);
    font-size: 10px; font-weight: 500; letter-spacing: 0.16em; text-transform: uppercase;
    color: var(--muted-foreground); font-variant-numeric: tabular-nums;
  }
  .step-no .cur { color: var(--brand); font-weight: 600; }
  .step-no .tot { opacity: 0.5; }
  .caption { font-size: 12.5px; line-height: 1.45; color: var(--foreground); margin: 0; }
  .caption b { font-weight: 600; color: var(--brand); }
  .caption code {
    border-radius: 4px; background: var(--muted);
    padding: 1px 6px; font-family: var(--font-mono); font-size: 0.86em;
  }
  @keyframes capIn { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: none; } }

  .controls { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
  .ctrl {
    display: inline-flex; align-items: center; gap: 5px;
    height: 26px; padding: 0 10px; border-radius: 7px;
    border: 1px solid var(--border); background: var(--card); color: var(--foreground);
    font-family: var(--font-mono); font-size: 10.5px; font-weight: 500;
    cursor: pointer; user-select: none;
    transition: background .15s, border-color .15s, color .15s;
  }
  .ctrl:hover { border-color: var(--brand); color: var(--brand); }
  .ctrl.primary {
    min-width: 78px; justify-content: center;
    background: var(--brand); border-color: var(--brand); color: var(--brand-foreground);
  }
  .ctrl.primary:hover { filter: brightness(1.06); color: var(--brand-foreground); }
  .dots { display: flex; align-items: center; gap: 5px; flex: 1; min-width: 70px; padding: 0 4px; }
  .dot {
    width: 8px; height: 8px; border-radius: 999px; padding: 0; border: none;
    background: color-mix(in oklch, var(--muted-foreground) 32%, transparent);
    cursor: pointer; transition: transform .15s, background .15s, box-shadow .15s;
  }
  .dot:hover { transform: scale(1.25); }
  .dot.done { background: var(--brand); }
  .dot.cur { background: var(--brand); box-shadow: 0 0 0 4px var(--brand-ring); }

  @media (min-width: 960px) and (min-height: 640px) {
    .viz-head .badge { font-size: 12px; padding: 3px 11px; }
    .viz-title { font-size: 15px; }
    .viz-sub { font-size: 13px; }
    .caption { font-size: 14px; }
    .ctrl { height: 30px; font-size: 11px; padding: 0 12px; }
    .dot { width: 9px; height: 9px; }
  }

  svg.diagram { width: 100%; height: 100%; display: block; }
  svg.diagram .node-rect {
    fill: var(--card); stroke: var(--foreground); stroke-width: 1.25;
    transition: fill 280ms cubic-bezier(.34,1.56,.64,1),
                stroke 280ms cubic-bezier(.34,1.56,.64,1),
                filter 280ms cubic-bezier(.34,1.56,.64,1);
  }
  svg.diagram .node.kind-accent .node-rect { fill: var(--brand-soft); stroke: var(--brand); }
  svg.diagram .node.kind-dark .node-rect { fill: var(--foreground); stroke: var(--foreground); }
  svg.diagram .node.kind-store circle {
    fill: var(--brand-soft); stroke: var(--brand); stroke-width: 1.25;
    transition: filter 280ms ease, stroke 280ms ease;
  }
  svg.diagram .node.kind-bus .node-rect { stroke-dasharray: 5 4; fill: var(--card); }
  svg.diagram .node-label {
    font-family: var(--font-sans); font-size: 12px; font-weight: 500;
    fill: var(--foreground); text-anchor: middle; dominant-baseline: central;
    pointer-events: none; letter-spacing: -0.01em;
  }
  svg.diagram .node.kind-dark .node-label { fill: var(--background); }
  svg.diagram .node.kind-accent .node-label,
  svg.diagram .node.kind-store .node-label { fill: var(--brand); font-weight: 600; }
  svg.diagram .node-sub {
    font-family: var(--font-mono); font-size: 9.5px; fill: var(--muted-foreground);
    letter-spacing: 0.08em; text-transform: uppercase;
    text-anchor: middle; dominant-baseline: central; pointer-events: none;
  }
  svg.diagram .node.kind-dark .node-sub { fill: oklch(0.72 0.012 260); }
  svg.diagram .node.active .node-rect,
  svg.diagram .node.kind-store.active circle {
    stroke: var(--brand); stroke-width: 2;
    animation: nodePulse 1.6s ease-in-out infinite;
  }
  svg.diagram .node.dim { opacity: 0.35; transition: opacity 320ms ease; }
  @keyframes nodePulse {
    0%, 100% { filter: drop-shadow(0 0 6px var(--brand-ring)) drop-shadow(0 4px 14px var(--brand-ring)); }
    50% { filter: drop-shadow(0 0 14px var(--brand-ring)) drop-shadow(0 8px 22px var(--brand-ring)); }
  }
  svg.diagram .edge {
    stroke: var(--border); stroke-width: 1.25; fill: none;
    transition: stroke 240ms ease, stroke-width 240ms ease, opacity 240ms ease;
  }
  svg.diagram .edge.dashed { stroke-dasharray: 5 4; }
  svg.diagram .edge.firing { stroke: var(--brand); stroke-width: 2.25; }
  svg.diagram .edge.done { stroke: var(--brand); stroke-width: 1.5; opacity: 0.7; }
  svg.diagram .edge-label {
    font-family: var(--font-mono); font-size: 10.5px; fill: var(--muted-foreground);
    text-anchor: middle; dominant-baseline: central;
    opacity: 0; transition: opacity 240ms ease; letter-spacing: 0.02em;
  }
  svg.diagram .edge-label.show { opacity: 1; fill: var(--brand); }
  svg.diagram .edge-label-bg { fill: var(--card); }
  svg.diagram .msg-token {
    fill: var(--brand); pointer-events: none;
    filter: drop-shadow(0 0 6px var(--brand-ring));
  }
  svg.diagram .msg-token.return {
    fill: var(--warning);
    filter: drop-shadow(0 0 6px oklch(0.65 0.14 70 / 0.55));
  }
</style>
</head>
<body>
  <div class="viz">
    <div class="viz-head">
      <span class="badge">Animated topology</span>
      <span class="viz-title" id="vizTitle"></span>
      <span class="viz-sub" id="vizSub"></span>
    </div>
    <div class="canvas-wrap">
      <div class="corner left"><span>&middot;</span><span>Diagram</span></div>
      <div class="corner right">
        <span class="live-dot" id="liveDot"><span class="ping"></span><span class="core"></span></span>
        <span>Live</span>
      </div>
      <svg class="diagram" id="svg" viewBox="0 0 900 540" preserveAspectRatio="xMidYMid meet"></svg>
    </div>
    <div class="caption-wrap">
      <div class="step-no"><span class="cur" id="stepCur">1</span><span class="tot" id="stepTot"> / 1</span></div>
      <p class="caption" id="caption">-</p>
    </div>
    <div class="controls">
      <button class="ctrl" id="btnPrev" type="button" title="Previous">&#8249; Prev</button>
      <button class="ctrl primary" id="btnPlay" type="button" title="Play / Pause">&#10074;&#10074; Pause</button>
      <button class="ctrl" id="btnNext" type="button" title="Next">Next &#8250;</button>
      <div class="dots" id="dots"></div>
    </div>
  </div>

<script>
(function () {
  "use strict";
  var PATTERN = __PATTERN_JSON__;
  var SVG_NS = "http://www.w3.org/2000/svg";
  var NODE_W = 140, NODE_H = 54, ARROW_TRIM = 4;
  var TOKEN_COUNT = 5, TOKEN_STAGGER = 0.10, SPEED = 1;

  function nodeCenter(n) { return { cx: n.x + (n.w || NODE_W) / 2, cy: n.y + (n.h || NODE_H) / 2 }; }
  function trimToEdge(cx, cy, w, h, dx, dy) {
    if (Math.abs(dx) < 1e-6 && Math.abs(dy) < 1e-6) return { x: cx, y: cy };
    var halfW = w / 2, halfH = h / 2;
    var len = Math.hypot(dx, dy), ux = dx / len, uy = dy / len;
    var tx = Math.abs(ux) > 1e-6 ? halfW / Math.abs(ux) : Infinity;
    var ty = Math.abs(uy) > 1e-6 ? halfH / Math.abs(uy) : Infinity;
    var t = Math.min(tx, ty) + ARROW_TRIM;
    return { x: cx + ux * t, y: cy + uy * t };
  }
  function buildPath(edge, nodesById) {
    var from = nodesById[edge.from], to = nodesById[edge.to];
    if (!from || !to) return "";
    var f = nodeCenter(from), t = nodeCenter(to);
    var fw = from.w || NODE_W, fh = from.h || NODE_H, tw = to.w || NODE_W, th = to.h || NODE_H;
    var dx = t.cx - f.cx, dy = t.cy - f.cy;
    var start = trimToEdge(f.cx, f.cy, fw, fh, dx, dy);
    var end = trimToEdge(t.cx, t.cy, tw, th, -dx, -dy);
    var curve = edge.curve || 0;
    if (curve === 0) return "M" + start.x + "," + start.y + " L" + end.x + "," + end.y;
    var mx = (start.x + end.x) / 2, my = (start.y + end.y) / 2;
    var len2 = Math.hypot(end.x - start.x, end.y - start.y) || 1;
    var px = -(end.y - start.y) / len2, py = (end.x - start.x) / len2;
    return "M" + start.x + "," + start.y + " Q" + (mx + px * curve) + "," + (my + py * curve) + " " + end.x + "," + end.y;
  }
  function spawnTokens(pathEl, gBubbles, duration, reverse) {
    var len = pathEl.getTotalLength(), tokens = [];
    for (var i = 0; i < TOKEN_COUNT; i++) {
      var tk = document.createElementNS(SVG_NS, "circle");
      tk.setAttribute("r", "3.5");
      tk.setAttribute("class", "msg-token" + (reverse ? " return" : ""));
      tk.setAttribute("opacity", "0");
      gBubbles.appendChild(tk);
      tokens.push(tk);
    }
    var start = performance.now();
    var traverseTime = duration * (1 - TOKEN_STAGGER * (TOKEN_COUNT - 1));
    function tick(now) {
      var elapsed = now - start;
      for (var i = 0; i < tokens.length; i++) {
        var tk = tokens[i], tStart = i * TOKEN_STAGGER * duration;
        var t = (elapsed - tStart) / traverseTime;
        if (t < 0 || t > 1) { tk.setAttribute("opacity", "0"); continue; }
        var u = reverse ? (1 - t) : t;
        var pt = pathEl.getPointAtLength(u * len);
        tk.setAttribute("cx", String(pt.x));
        tk.setAttribute("cy", String(pt.y));
        var fade = Math.min(t * 5, 1, (1 - t) * 5);
        tk.setAttribute("opacity", String(Math.max(0, fade) * 0.95));
      }
      if (elapsed < duration + 200) requestAnimationFrame(tick);
      else tokens.forEach(function (tk) { tk.remove(); });
    }
    requestAnimationFrame(tick);
  }

  var svg = document.getElementById("svg");
  var gEdges = document.createElementNS(SVG_NS, "g");
  var gBubbles = document.createElementNS(SVG_NS, "g");
  var gNodes = document.createElementNS(SVG_NS, "g");
  svg.append(gEdges, gBubbles, gNodes);

  var nodesById = {}, edgePaths = {}, edgeLabels = {}, nodeEls = {};
  (PATTERN.nodes || []).forEach(function (n) { nodesById[n.id] = n; });

  Object.keys(PATTERN.edges || {}).forEach(function (eid) {
    var edge = PATTERN.edges[eid];
    var path = document.createElementNS(SVG_NS, "path");
    path.setAttribute("d", buildPath(edge, nodesById));
    path.setAttribute("class", "edge" + (edge.dashed ? " dashed" : ""));
    path.setAttribute("fill", "none");
    gEdges.appendChild(path);
    edgePaths[eid] = path;
    if (edge.label) requestAnimationFrame(function () {
      var len = path.getTotalLength(), mid = path.getPointAtLength(len / 2);
      var charW = 6.2, padX = 6, padY = 2, fontSize = 10.5;
      var w = edge.label.length * charW + padX * 2, h = fontSize + padY * 2;
      var bg = document.createElementNS(SVG_NS, "rect");
      bg.setAttribute("x", String(mid.x - w / 2));
      bg.setAttribute("y", String(mid.y - h / 2));
      bg.setAttribute("width", String(w));
      bg.setAttribute("height", String(h));
      bg.setAttribute("rx", "4");
      bg.setAttribute("class", "edge-label-bg");
      gEdges.insertBefore(bg, path);
      var txt = document.createElementNS(SVG_NS, "text");
      txt.setAttribute("x", String(mid.x));
      txt.setAttribute("y", String(mid.y));
      txt.setAttribute("class", "edge-label");
      txt.textContent = edge.label;
      gEdges.appendChild(txt);
      edgeLabels[eid] = txt;
    });
  });

  (PATTERN.nodes || []).forEach(function (n) {
    var w = n.w || NODE_W, h = n.h || NODE_H;
    var g = document.createElementNS(SVG_NS, "g");
    g.setAttribute("class", "node kind-" + (n.kind || "plain"));
    if (n.kind === "store") {
      var circle = document.createElementNS(SVG_NS, "circle");
      circle.setAttribute("cx", String(n.x + w / 2));
      circle.setAttribute("cy", String(n.y + h / 2));
      circle.setAttribute("r", String(w / 2));
      circle.setAttribute("class", "node-rect");
      g.appendChild(circle);
    } else {
      var rect = document.createElementNS(SVG_NS, "rect");
      rect.setAttribute("x", String(n.x));
      rect.setAttribute("y", String(n.y));
      rect.setAttribute("width", String(w));
      rect.setAttribute("height", String(h));
      rect.setAttribute("class", "node-rect");
      rect.setAttribute("rx", n.kind === "user" ? "28" : "8");
      rect.setAttribute("ry", n.kind === "user" ? "28" : "8");
      g.appendChild(rect);
    }
    var labelY = n.sub ? n.y + h / 2 - 8 : n.y + h / 2;
    var lbl = document.createElementNS(SVG_NS, "text");
    lbl.setAttribute("x", String(n.x + w / 2));
    lbl.setAttribute("y", String(labelY));
    lbl.setAttribute("class", "node-label");
    lbl.textContent = n.label;
    g.appendChild(lbl);
    if (n.sub) {
      var sub = document.createElementNS(SVG_NS, "text");
      sub.setAttribute("x", String(n.x + w / 2));
      sub.setAttribute("y", String(n.y + h / 2 + 12));
      sub.setAttribute("class", "node-sub");
      sub.textContent = n.sub;
      g.appendChild(sub);
    }
    gNodes.appendChild(g);
    nodeEls[n.id] = g;
  });

  var tl = PATTERN.timeline || [];
  var step = 0, playing = false, doneSet = {}, nextTimer = null, pendingTimers = [];
  var capEl = document.getElementById("caption");
  var stepCurEl = document.getElementById("stepCur");
  var stepTotEl = document.getElementById("stepTot");
  var liveDot = document.getElementById("liveDot");
  var btnPlay = document.getElementById("btnPlay");
  var dotsWrap = document.getElementById("dots");
  document.getElementById("vizTitle").textContent = PATTERN.title || "Animated Diagram";
  document.getElementById("vizSub").textContent = PATTERN.sub || "";
  stepTotEl.textContent = " / " + Math.max(tl.length, 1);

  var dotEls = [];
  tl.forEach(function (_, i) {
    var d = document.createElement("button");
    d.type = "button";
    d.className = "dot";
    d.title = "Step " + (i + 1);
    d.addEventListener("click", function () { gotoStep(i); });
    dotsWrap.appendChild(d);
    dotEls.push(d);
  });

  function clearPending() { pendingTimers.forEach(clearTimeout); pendingTimers = []; }
  function clearNext() { if (nextTimer) { clearTimeout(nextTimer); nextTimer = null; } }
  function setCaption(s) {
    capEl.innerHTML = s.caption || "-";
    capEl.style.animation = "none";
    void capEl.offsetWidth;
    capEl.style.animation = "capIn 0.26s cubic-bezier(.22,1,.36,1)";
  }
  function setNodes(s) {
    var act = {}, dim = {};
    (s.activate || []).forEach(function (id) { act[id] = true; });
    (s.dim || []).forEach(function (id) { dim[id] = true; });
    Object.keys(nodeEls).forEach(function (id) {
      nodeEls[id].classList.toggle("active", !!act[id]);
      nodeEls[id].classList.toggle("dim", !!dim[id]);
    });
  }
  function paintEdgesBase() {
    Object.keys(edgePaths).forEach(function (eid) {
      edgePaths[eid].classList.remove("firing");
      edgePaths[eid].classList.toggle("done", !!doneSet[eid]);
    });
    Object.keys(edgeLabels).forEach(function (eid) { edgeLabels[eid].classList.remove("show"); });
  }
  function rebuildDone(uptoExclusive) {
    doneSet = {};
    for (var k = 0; k < uptoExclusive; k++) {
      (tl[k].fire || []).forEach(function (raw) {
        doneSet[raw.charAt(0) === "!" ? raw.slice(1) : raw] = true;
      });
    }
  }
  function updateUI() {
    stepCurEl.textContent = String(step + 1);
    dotEls.forEach(function (d, i) {
      d.classList.toggle("cur", i === step);
      d.classList.toggle("done", i < step);
    });
    btnPlay.innerHTML = playing ? "&#10074;&#10074; Pause" : "&#9654; Play";
    liveDot.classList.toggle("paused", !playing);
  }
  function fireStep(idx) {
    var s = tl[idx];
    if (!s) return 1500;
    setCaption(s);
    setNodes(s);
    paintEdgesBase();
    var firingEids = [];
    (s.fire || []).forEach(function (raw) {
      var reverse = raw.charAt(0) === "!";
      var eid = reverse ? raw.slice(1) : raw;
      var path = edgePaths[eid];
      if (!path) return;
      path.classList.add("firing");
      path.classList.remove("done");
      if (edgeLabels[eid]) edgeLabels[eid].classList.add("show");
      spawnTokens(path, gBubbles, Math.min((1500 / SPEED) * 0.85, 1400), reverse);
      firingEids.push(eid);
    });
    var dur = (s.duration || 1500) / SPEED;
    pendingTimers.push(setTimeout(function () {
      firingEids.forEach(function (eid) {
        var path = edgePaths[eid];
        if (path) { path.classList.remove("firing"); path.classList.add("done"); doneSet[eid] = true; }
        if (edgeLabels[eid]) edgeLabels[eid].classList.remove("show");
      });
    }, Math.min(dur * 0.85, 1400)));
    return dur;
  }
  function renderStatic(idx) {
    var s = tl[idx];
    if (!s) return;
    setCaption(s);
    setNodes(s);
    paintEdgesBase();
    updateUI();
  }
  function scheduleNext(idx, dur) {
    nextTimer = setTimeout(function () {
      if (!playing) return;
      var next = (idx + 1) % tl.length;
      if (next === 0) doneSet = {};
      step = next;
      updateUI();
      scheduleNext(next, fireStep(next));
    }, dur);
  }
  function play() {
    if (playing || !tl.length) return;
    playing = true;
    updateUI();
    scheduleNext(step, fireStep(step));
  }
  function pause() {
    playing = false;
    clearNext();
    clearPending();
    updateUI();
  }
  function gotoStep(i) {
    pause();
    if (!tl.length) return;
    step = ((i % tl.length) + tl.length) % tl.length;
    rebuildDone(step);
    renderStatic(step);
  }

  btnPlay.addEventListener("click", function () { playing ? pause() : play(); });
  document.getElementById("btnPrev").addEventListener("click", function () { gotoStep(step - 1); });
  document.getElementById("btnNext").addEventListener("click", function () { gotoStep(step + 1); });
  requestAnimationFrame(function () { requestAnimationFrame(function () { updateUI(); play(); }); });
})();
</script>
</body>
</html>
"""


def validate_pattern(pattern: dict) -> None:
    required = ["title", "nodes", "edges", "timeline"]
    missing = [key for key in required if key not in pattern]
    if missing:
        raise ValueError(f"missing required field(s): {', '.join(missing)}")

    node_ids = set()
    for i, node in enumerate(pattern["nodes"]):
        for key in ["id", "x", "y", "label"]:
            if key not in node:
                raise ValueError(f"nodes[{i}] missing {key!r}")
        if node["id"] in node_ids:
            raise ValueError(f"duplicate node id: {node['id']}")
        node_ids.add(node["id"])

    for edge_id, edge in pattern["edges"].items():
        for key in ["from", "to"]:
            if key not in edge:
                raise ValueError(f"edge {edge_id!r} missing {key!r}")
        if edge["from"] not in node_ids:
            raise ValueError(f"edge {edge_id!r} references missing from node {edge['from']!r}")
        if edge["to"] not in node_ids:
            raise ValueError(f"edge {edge_id!r} references missing to node {edge['to']!r}")

    edge_ids = set(pattern["edges"].keys())
    for i, step in enumerate(pattern["timeline"]):
        if "caption" not in step:
            raise ValueError(f"timeline[{i}] missing 'caption'")
        for raw in step.get("fire", []):
            edge_id = raw[1:] if raw.startswith("!") else raw
            if edge_id not in edge_ids:
                raise ValueError(f"timeline[{i}] references missing edge {edge_id!r}")
        for field in ["activate", "dim"]:
            for node_id in step.get(field, []):
                if node_id not in node_ids:
                    raise ValueError(f"timeline[{i}].{field} references missing node {node_id!r}")


def render(pattern: dict) -> str:
    pattern_json = json.dumps(pattern, ensure_ascii=False, separators=(",", ":"))
    title = str(pattern.get("title", "Animated Diagram"))
    return (
        HTML_TEMPLATE
        .replace("__TITLE__", title.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;"))
        .replace("__PATTERN_JSON__", pattern_json)
    )


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pattern", required=True, help="Path to pattern JSON")
    parser.add_argument("--out", required=True, help="Output HTML path")
    parser.add_argument("--no-validate", action="store_true", help="Skip schema validation")
    args = parser.parse_args(argv)

    pattern_path = Path(args.pattern)
    out_path = Path(args.out)
    pattern = json.loads(pattern_path.read_text(encoding="utf-8"))
    if not args.no_validate:
        validate_pattern(pattern)

    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(render(pattern), encoding="utf-8")
    print(f"Wrote {out_path} ({out_path.stat().st_size} bytes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
