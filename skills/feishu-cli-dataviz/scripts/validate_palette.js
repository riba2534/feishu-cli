/**
 * 飞书可视化色板校验器 —— 用计算代替肉眼判断配色是否合格。
 *
 * 对一组 categorical（系列身份）颜色执行四项可计算检查：
 *   1. 亮度带     — OKLCH L 需落在模式对应区间（light 0.43–0.77 / dark 0.48–0.67）
 *   2. 饱和度下限 — OKLCH C ≥ 0.10（再低就退化成灰色，无法承担身份区分）
 *   3. 色盲区分度 — Machado-2009 模拟红色盲/绿色盲后，相邻色对 CIE76 ΔE ≥ 12
 *                   （8–12 为地板区，仅在配有直接标签/留白间隔/纹理等二级编码时合法；
 *                   蓝黄色盲 tritan 极罕见，仅随报告输出参考值、不作门槛）
 *   4. 表面对比度 — 每个色相对图表底色 WCAG 对比度 ≥ 3:1
 *                   （不足 3:1 为"救济区"：必须提供可见数值标签或表格视图）
 *
 * 另有 --ordinal 模式校验单色 ramp（有序类别：漏斗阶段/等级/分桶）：
 *   单一色相、亮度单调、相邻步 ΔL ≥ 0.06、最浅步对底色对比度 ≥ 2:1。
 *
 * 用法（Node，任意版本，无依赖）：
 *   node validate_palette.js                       # 复验 palette.json 定稿组合
 *   node validate_palette.js "#3370ff,#0fb5ae,..." --mode light
 *   node validate_palette.js "#5c8dff,..." --mode dark --surface "#1f1f1f"
 *   node validate_palette.js "..." --pairs circular   # 饼/环图：相邻项含首尾闭环
 *   node validate_palette.js "..." --pairs all        # 散点/气泡/地图：任意两色都可能相邻
 *   node validate_palette.js "#d6e2ff,#94b4ff,..." --ordinal
 *   加 --json 输出机器可读结果。
 *
 * 用法（浏览器）：
 *   <body data-palette="#3370ff,#0fb5ae,..." data-mode="dark" data-surface="#1f1f1f">
 *   <script src="validate_palette.js"></script>
 *   → console.table 打印报告，FAIL 时 console.error。
 *   注意：script 的 src 相对于 HTML 文件位置解析，HTML 不在本目录时需先把
 *   校验器复制到 HTML 同目录（cp validate_palette.js <html所在目录>/）。
 *
 * 退出码：0 = 无硬性 FAIL（WARN 不算失败，但必须落实二级编码/救济手段）；
 *         1 = 存在 FAIL；2 = 用法/参数错误（含非法色值）。
 */

"use strict";

// ── 阈值 ────────────────────────────────────────────────────────────────────
var L_BAND = { light: [0.43, 0.77], dark: [0.48, 0.67] }; // OKLCH 亮度带
var C_MIN = 0.10;                    // OKLCH 饱和度下限
var DE_TARGET = 12, DE_FLOOR = 8;    // CVD ΔE 目标 / 地板
var CR_MIN = 3.0;                    // 标记 vs 底色最低对比度
var STEP_DL_MIN = 0.06;              // ordinal 相邻步最小 ΔL
var LIGHT_END_CR = 2.0;              // ordinal 最浅步对比度下限
var HUE_SPREAD_MAX = 40;             // ordinal 单一色相最大覆盖弧（度）
var SURFACE_DEFAULT = { light: "#ffffff", dark: "#1f1f1f" }; // 飞书文档明/暗底色

// Machado, Oliveira & Fernandes (2009) 色觉缺陷模拟矩阵（severity=1.0，线性 RGB 域）
var CVD_MATRIX = {
  protan: [[0.152286, 1.052583, -0.204868],
           [0.114503, 0.786281, 0.099216],
           [-0.003882, -0.048116, 1.051998]],
  deutan: [[0.367322, 0.860646, -0.227968],
           [0.280085, 0.672501, 0.047413],
           [-0.011820, 0.042940, 0.968881]],
  tritan: [[1.255528, -0.076749, -0.178779],
           [-0.078411, 0.930809, 0.147602],
           [0.004733, 0.691367, 0.303900]],
};

// ── 颜色数学（sRGB / OKLab / CIELAB，均为公开标准公式） ──────────────────────
function parseHex(hex) {
  var h = String(hex).trim().replace(/^#/, "");
  if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
  if (!/^[0-9a-fA-F]{6}$/.test(h)) throw new Error("非法颜色值: " + hex);
  return [parseInt(h.slice(0, 2), 16) / 255, parseInt(h.slice(2, 4), 16) / 255, parseInt(h.slice(4, 6), 16) / 255];
}

function srgbToLinear(c) { return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); }

function linearRgb(hex) { return parseHex(hex).map(srgbToLinear); }

function luminance(hex) {
  var rgb = linearRgb(hex);
  return 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2];
}

// WCAG 对比度
function contrastRatio(hexA, hexB) {
  var a = luminance(hexA), b = luminance(hexB);
  var hi = Math.max(a, b), lo = Math.min(a, b);
  return (hi + 0.05) / (lo + 0.05);
}

// 线性 RGB → OKLab（Björn Ottosson 公开转换）
function oklabFromLinear(rgb) {
  var r = rgb[0], g = rgb[1], b = rgb[2];
  var l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  var m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  var s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.7936177850 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.4285922050 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.8086757660 * s,
  ];
}

function oklch(hex) {
  var lab = oklabFromLinear(linearRgb(hex));
  var C = Math.hypot(lab[1], lab[2]);
  var H = Math.atan2(lab[2], lab[1]) * 180 / Math.PI;
  return { L: lab[0], C: C, H: (H % 360 + 360) % 360 };
}

// 线性 RGB → CIELAB（D65），用于 ΔE76
function cielabFromLinear(rgb) {
  var X = 0.4124564 * rgb[0] + 0.3575761 * rgb[1] + 0.1804375 * rgb[2];
  var Y = 0.2126729 * rgb[0] + 0.7151522 * rgb[1] + 0.0721750 * rgb[2];
  var Z = 0.0193339 * rgb[0] + 0.1191920 * rgb[1] + 0.9503041 * rgb[2];
  var f = function (t) { return t > 0.008856 ? Math.cbrt(t) : 7.787 * t + 16 / 116; };
  var fx = f(X / 0.95047), fy = f(Y / 1.0), fz = f(Z / 1.08883);
  return [116 * fy - 16, 500 * (fx - fy), 200 * (fy - fz)];
}

// 色觉缺陷模拟（线性 RGB 域施加矩阵后截断到 [0,1]）
function simulateCvd(rgb, kind) {
  var M = CVD_MATRIX[kind];
  var out = [];
  for (var i = 0; i < 3; i++) {
    var v = M[i][0] * rgb[0] + M[i][1] * rgb[1] + M[i][2] * rgb[2];
    out.push(Math.min(1, Math.max(0, v)));
  }
  return out;
}

// 两色在指定色觉条件下的 CIE76 ΔE（kind 为空 = 正常视觉）
function cvdDeltaE(hexA, hexB, kind) {
  var a = linearRgb(hexA), b = linearRgb(hexB);
  if (kind) { a = simulateCvd(a, kind); b = simulateCvd(b, kind); }
  var la = cielabFromLinear(a), lb = cielabFromLinear(b);
  return Math.hypot(la[0] - lb[0], la[1] - lb[1], la[2] - lb[2]);
}

// 一组色相角的最小覆盖弧（度）：排序后找最大间隙（含首尾环绕），360 减之。
// 对任意个数的色相都正确；简单的 max-min 折算在 ≥3 色跨 0° 时会算出假小值。
function hueSpread(hues) {
  if (hues.length < 2) return 0;
  var sorted = hues.slice().sort(function (a, b) { return a - b; });
  var maxGap = 360 - sorted[sorted.length - 1] + sorted[0];
  for (var i = 1; i < sorted.length; i++) {
    var g = sorted[i] - sorted[i - 1];
    if (g > maxGap) maxGap = g;
  }
  return 360 - maxGap;
}

// ── categorical 四检查 ──────────────────────────────────────────────────────
function checkCategorical(palette, opts) {
  opts = opts || {};
  var mode = opts.mode || "light";
  var surface = opts.surface || SURFACE_DEFAULT[mode];
  var pairs = opts.pairs || "adjacent";
  if (pairs !== "adjacent" && pairs !== "circular" && pairs !== "all") throw new Error("pairs 只接受 adjacent|circular|all（收到 " + pairs + "）");
  var band = L_BAND[mode];
  var checks = [];
  var failed = false;
  var cols = palette.map(oklch);

  // 0. 重复色永远不合法。即使重复项不相邻，它们也无法承载两个系列身份。
  var positions = {};
  palette.forEach(function (color, index) {
    var key = color.toLowerCase();
    if (!positions[key]) positions[key] = [];
    positions[key].push(index + 1);
  });
  var duplicates = Object.keys(positions).filter(function (key) { return positions[key].length > 1; });
  if (duplicates.length) failed = true;
  checks.push({
    name: "色值唯一", level: duplicates.length ? "fail" : "pass",
    detail: duplicates.length
      ? "重复: " + duplicates.map(function (key) { return key + " (slot " + positions[key].join("/") + ")"; }).join(", ")
      : "无重复色值",
  });

  // 1. 亮度带
  var offBand = [];
  palette.forEach(function (c, i) {
    if (cols[i].L < band[0] || cols[i].L > band[1]) offBand.push(c + " (L=" + cols[i].L.toFixed(3) + ")");
  });
  if (offBand.length) failed = true;
  checks.push({
    name: "亮度带", level: offBand.length ? "fail" : "pass",
    detail: offBand.length ? "越界: " + offBand.join(", ") : palette.length + " 色全部在 L " + band[0] + "–" + band[1] + " 内",
  });

  // 2. 饱和度下限
  var grayish = [];
  palette.forEach(function (c, i) {
    if (cols[i].C < C_MIN) grayish.push(c + " (C=" + cols[i].C.toFixed(3) + ")");
  });
  if (grayish.length) failed = true;
  checks.push({
    name: "饱和度下限", level: grayish.length ? "fail" : "pass",
    detail: grayish.length ? "低于 " + C_MIN + "（读作灰色）: " + grayish.join(", ") : "全部 C ≥ " + C_MIN,
  });

  // 3. 色盲区分度：普通序列查相邻，饼/环图还查首尾，散点/地图查任意两色。
  var idxPairs = [];
  var n = palette.length;
  if (pairs === "all") {
    for (var i = 0; i < n; i++) for (var j = i + 1; j < n; j++) idxPairs.push([i, j]);
  } else {
    for (var k = 0; k + 1 < n; k++) idxPairs.push([k, k + 1]);
    if (pairs === "circular" && n > 2) idxPairs.push([n - 1, 0]);
  }
  var worst = null;
  var worstTritan = null;
  idxPairs.forEach(function (p) {
    ["protan", "deutan"].forEach(function (kind) {
      var d = cvdDeltaE(palette[p[0]], palette[p[1]], kind);
      if (!worst || d < worst.dE) worst = { dE: d, kind: kind, a: palette[p[0]], b: palette[p[1]] };
    });
    var t = cvdDeltaE(palette[p[0]], palette[p[1]], "tritan");
    if (worstTritan === null || t < worstTritan) worstTritan = t;
  });
  var cvdLevel = !worst ? "pass" : worst.dE >= DE_TARGET ? "pass" : worst.dE >= DE_FLOOR ? "warn" : "fail";
  if (cvdLevel === "fail") failed = true;
  checks.push({
    name: "色盲区分度(" + (pairs === "all" ? "全对" : pairs === "circular" ? "相邻+首尾" : "相邻") + ")", level: cvdLevel,
    detail: worst
      ? "最差 " + worst.a + " ↔ " + worst.b + " ΔE=" + worst.dE.toFixed(1) + " (" + worst.kind + ")；目标≥" + DE_TARGET + "，地板 " + DE_FLOOR + "–" + DE_TARGET + " 须配二级编码 · tritan 最差 ΔE=" + worstTritan.toFixed(1) + "（参考值，不作门槛）"
      : "单色无需检查",
  });

  // 4. 表面对比度：不足 3:1 不算 FAIL，但救济手段（可见标签/表格视图）是义务不是可选项
  var lowCr = [];
  palette.forEach(function (c) {
    var cr = contrastRatio(c, surface);
    if (cr < CR_MIN) lowCr.push(c + " (" + cr.toFixed(2) + ":1)");
  });
  checks.push({
    name: "表面对比度", level: lowCr.length ? "warn" : "pass",
    detail: lowCr.length
      ? "低于 " + CR_MIN + ":1，必须配可见数值标签或表格视图: " + lowCr.join(", ")
      : "全部 ≥ " + CR_MIN + ":1（底色 " + surface + "）",
  });

  return { checks: checks, ok: !failed };
}

// ── ordinal（单色 ramp）检查 ────────────────────────────────────────────────
function checkOrdinal(palette, opts) {
  opts = opts || {};
  var mode = opts.mode || "light";
  var surface = opts.surface || SURFACE_DEFAULT[mode];
  var checks = [];
  var failed = false;
  var cols = palette.map(oklch);
  var Ls = cols.map(function (c) { return c.L; });

  // 亮度单调（允许整体正序或倒序）
  var asc = true, desc = true;
  for (var i = 1; i < Ls.length; i++) {
    if (Ls[i] <= Ls[i - 1]) asc = false;
    if (Ls[i] >= Ls[i - 1]) desc = false;
  }
  var mono = asc || desc;
  if (!mono) failed = true;
  checks.push({
    name: "亮度单调", level: mono ? "pass" : "fail",
    detail: mono ? "各步按浅→深单调排列" : "乱序，L 序列: " + Ls.map(function (l) { return l.toFixed(3); }).join(", "),
  });

  // 相邻步 ΔL
  var thin = [];
  for (var j = 1; j < Ls.length; j++) {
    var g = Math.abs(Ls[j] - Ls[j - 1]);
    if (g < STEP_DL_MIN) thin.push(palette[j - 1] + "↔" + palette[j] + " (ΔL=" + g.toFixed(3) + ")");
  }
  if (thin.length) failed = true;
  checks.push({
    name: "相邻步距", level: thin.length ? "fail" : "pass",
    detail: thin.length ? "步距过小: " + thin.join(", ") : "全部 ΔL ≥ " + STEP_DL_MIN,
  });

  // 最浅步对比度：取"贴近底色的一端"（light 底=最亮步，dark 底=最暗步），仍需读得出是一个标记
  var idx = 0;
  for (var m = 1; m < Ls.length; m++) {
    if (mode === "light" ? Ls[m] > Ls[idx] : Ls[m] < Ls[idx]) idx = m;
  }
  var nearest = palette[idx];
  var cr = contrastRatio(nearest, surface);
  if (cr < LIGHT_END_CR) failed = true;
  checks.push({
    name: "浅端对比度", level: cr >= LIGHT_END_CR ? "pass" : "fail",
    detail: nearest + " 对底色 " + cr.toFixed(2) + ":1" + (cr >= LIGHT_END_CR ? "" : "，低于 " + LIGHT_END_CR + ":1"),
  });

  // 单一色相（覆盖弧超 40° 就不是 ramp 而是 categorical 了）
  var spread = hueSpread(cols.map(function (c) { return c.H; }));
  var oneHue = spread <= HUE_SPREAD_MAX;
  if (!oneHue) failed = true;
  checks.push({
    name: "单一色相", level: oneHue ? "pass" : "fail",
    detail: "色相覆盖弧 " + spread.toFixed(0) + "°" + (oneHue ? "" : "，超过 " + HUE_SPREAD_MAX + "°，不是单色 ramp"),
  });

  return { checks: checks, ok: !failed };
}

// ── 输出 ────────────────────────────────────────────────────────────────────
var LEVEL_TAG = { pass: "PASS", warn: "WARN", fail: "FAIL" };

function printReport(result, ctx) {
  console.log("\n色板校验（" + ctx.mode + "，底色 " + ctx.surface + "，" + (ctx.ordinal ? "ordinal ramp" : "categorical") + "，" + ctx.count + " 色）");
  var hasWarn = false;
  result.checks.forEach(function (c) {
    if (c.level === "warn") hasWarn = true;
    console.log("  [" + LEVEL_TAG[c.level] + "] " + c.name + " — " + c.detail);
  });
  if (!result.ok) {
    console.log("\n  → 未通过，先修复 FAIL 项再使用该色板。");
  } else if (hasWarn) {
    console.log("\n  → 通过（带 WARN）。WARN 项对应的二级编码/救济手段是强制义务，不是建议。");
  } else {
    console.log("\n  → 全部通过。");
  }
}

// ── Node CLI 入口 ───────────────────────────────────────────────────────────
// 仅在被直接执行时运行；被 require/import 或在浏览器中加载时不进入。
var IS_NODE_CLI = typeof require !== "undefined" && typeof module !== "undefined" && require.main === module;

if (IS_NODE_CLI) {
  var args = process.argv.slice(2);
  var opt = { mode: "light", pairs: "adjacent", ordinal: false, json: false };
  var listArg = null;
  var usageExit = function (msg) {
    console.error(msg);
    console.error('用法: node validate_palette.js ["#hex,#hex,..." [--mode light|dark] [--surface #hex] [--pairs adjacent|circular|all] [--ordinal] [--json]]');
    process.exit(2);
  };
  for (var ai = 0; ai < args.length; ai++) {
    var a = args[ai];
    var flagName = null, flagVal;
    if (a === "--mode" || a === "--surface" || a === "--pairs") {
      flagName = a.slice(2); flagVal = args[++ai];
    } else if (/^--(mode|surface|pairs)=/.test(a)) {
      var eq = a.indexOf("="); flagName = a.slice(2, eq); flagVal = a.slice(eq + 1);
    } else if (a === "--ordinal") { opt.ordinal = true; continue; }
    else if (a === "--json") { opt.json = true; continue; }
    else if (a.charAt(0) === "-") { usageExit("未知参数: " + a); }
    else if (listArg === null) { listArg = a; continue; }
    else { usageExit("多余参数: " + a); }
    if (flagName) {
      // 缺值或误吞下一个 flag（--surface --mode ...）都按用法错误处理，绝不静默回退
      if (flagVal === undefined || flagVal.charAt(0) === "-") usageExit("--" + flagName + " 缺少取值");
      opt[flagName] = flagVal;
    }
  }
  if (opt.mode !== "light" && opt.mode !== "dark") usageExit("--mode 只接受 light|dark（收到 " + opt.mode + "）");
  if (opt.pairs !== "adjacent" && opt.pairs !== "circular" && opt.pairs !== "all") usageExit("--pairs 只接受 adjacent|circular|all（收到 " + opt.pairs + "）");
  var colors = (listArg || "").split(",").map(function (s) { return s.trim(); }).filter(Boolean);
  if (!colors.length && args.length) usageExit("缺少色板参数");
  if (!colors.length) {
    var preset = require("./palette.json");
    var presetCases = [
      { name: "categorical light", colors: preset.categorical.light, mode: "light", surface: SURFACE_DEFAULT.light, pairs: "adjacent" },
      { name: "categorical dark", colors: preset.categorical.dark, mode: "dark", surface: SURFACE_DEFAULT.dark, pairs: "adjacent" },
      { name: "circular light", colors: preset.categorical.light, mode: "light", surface: SURFACE_DEFAULT.light, pairs: "circular" },
      { name: "circular dark", colors: preset.categorical.dark, mode: "dark", surface: SURFACE_DEFAULT.dark, pairs: "circular" },
      { name: "all-pairs light", colors: preset.allPairsSafeSubset.light, mode: "light", surface: SURFACE_DEFAULT.light, pairs: "all" },
      { name: "all-pairs dark", colors: preset.allPairsSafeSubset.dark, mode: "dark", surface: SURFACE_DEFAULT.dark, pairs: "all" },
      { name: "HTMLBox dark canvas", colors: preset.categorical.dark, mode: "dark", surface: preset.surfaces.htmlboxCanvas, pairs: "adjacent" },
      { name: "ordinal light", colors: [3, 5, 7, 9].map(function (i) { return preset.sequentialBlue[i]; }), mode: "light", surface: SURFACE_DEFAULT.light, ordinal: true },
      { name: "ordinal dark", colors: [0, 3, 6, 10].map(function (i) { return preset.sequentialBlue[i]; }), mode: "dark", surface: SURFACE_DEFAULT.dark, ordinal: true },
    ];
    var presetFailed = false;
    presetCases.forEach(function (item) {
      var result = item.ordinal ? checkOrdinal(item.colors, item) : checkCategorical(item.colors, item);
      console.log("\n定稿组合: " + item.name);
      printReport(result, { mode: item.mode, surface: item.surface, ordinal: !!item.ordinal, count: item.colors.length });
      if (!result.ok) presetFailed = true;
    });
    process.exit(presetFailed ? 1 : 0);
  }
  var surface = opt.surface || SURFACE_DEFAULT[opt.mode];
  var res;
  try {
    parseHex(surface); // 提前校验底色合法性
    colors.forEach(parseHex); // 提前校验色板合法性；非法色值按用法错误（exit 2）而非 FAIL（exit 1）
    res = opt.ordinal
      ? checkOrdinal(colors, { mode: opt.mode, surface: surface })
      : checkCategorical(colors, { mode: opt.mode, surface: surface, pairs: opt.pairs });
  } catch (e) {
    usageExit("参数错误: " + e.message);
  }
  if (opt.json) {
    console.log(JSON.stringify({ mode: opt.mode, surface: surface, ordinal: opt.ordinal, ok: res.ok, checks: res.checks }, null, 2));
  } else {
    printReport(res, { mode: opt.mode, surface: surface, ordinal: opt.ordinal, count: colors.length });
  }
  process.exit(res.ok ? 0 : 1);
}

// ── 浏览器入口：<body data-palette="..."> 自动运行 ──────────────────────────
if (!IS_NODE_CLI && typeof document !== "undefined" && document && document.body && document.body.dataset && document.body.dataset.palette) {
  (function () {
    var ds = document.body.dataset;
    var colors = ds.palette.split(",").map(function (s) { return s.trim(); }).filter(Boolean);
    var mode = ds.mode || "light";
    if (mode !== "light" && mode !== "dark") {
      console.error("validate_palette: data-mode 只接受 light|dark（收到 " + mode + "）");
      return;
    }
    var surface = ds.surface || SURFACE_DEFAULT[mode];
    var res;
    try {
      res = ("ordinal" in ds)
        ? checkOrdinal(colors, { mode: mode, surface: surface })
        : checkCategorical(colors, { mode: mode, surface: surface, pairs: ds.pairs || "adjacent" });
    } catch (e) {
      console.error("validate_palette: 参数错误 — " + e.message);
      return;
    }
    console.table(res.checks.map(function (c) { return { 检查: c.name, 结果: LEVEL_TAG[c.level], 说明: c.detail }; }));
    if (!res.ok) console.error("validate_palette: 色板校验未通过（存在 FAIL）");
  })();
}

// 供 Node 复用：require 本文件后可调用这些函数（不会触发 CLI）。
if (typeof module !== "undefined" && module.exports) {
  module.exports = {
    oklch: oklch,
    contrastRatio: contrastRatio,
    cvdDeltaE: cvdDeltaE,
    hueSpread: hueSpread,
    checkCategorical: checkCategorical,
    checkOrdinal: checkOrdinal,
  };
}
