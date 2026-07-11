/**
 * 统一色板文档一致性检查 —— 防止各技能文档中的色值与色板漂移。
 *
 * 检查三件事：
 *   1. 色值来源：skills/ 下的 .md 或卡片模板 .json 中，每个 CSS hex
 *      （#RGB/#RGBA/#RRGGBB/#RRGGBBAA）必须来自 palette.json canonical 色，
 *      或命中 docHexExceptions 的逐文件精确例外。#RGB 展开后匹配 canonical；
 *      带 alpha 的 #RGBA/#RRGGBBAA 不丢弃 alpha，必须按原值登记例外。
 *      deprecated 色无条件报错（大小写不敏感）。
 *   2. 权威文档完整性：references/palette.md 必须包含全部 canonical 色值
 *      （categorical light/dark、派生对、sequential、status）。
 *   3. 例外配置合法：路径和值格式必须明确，禁止通配或全局放行。
 *
 * 用法：node check_docs.js [skills目录]   # 默认自动定位本文件所在的 skills/ 根
 * 退出码：0 = 全部通过；1 = 发现残留或缺失。
 */

"use strict";

var fs = require("fs");
var path = require("path");

var HERE = __dirname; // .../skills/feishu-cli-dataviz/scripts
var SKILLS_ROOT = process.argv[2] || path.resolve(HERE, "..", "..");
var PALETTE = JSON.parse(fs.readFileSync(path.join(HERE, "palette.json"), "utf8"));
var PALETTE_MD = path.resolve(HERE, "..", "references", "palette.md");

// 收集废弃色（去重、小写）
var deprecated = [];
Object.keys(PALETTE.deprecated).forEach(function (group) {
  if (group.charAt(0) === "$") return;
  PALETTE.deprecated[group].forEach(function (hex) {
    var h = hex.toLowerCase();
    if (deprecated.indexOf(h) < 0) deprecated.push(h);
  });
});

// 收集 palette.json 中除 deprecated/例外配置外的全部 canonical hex。
var canonical = [];
function collectCanonical(value, key) {
  if (key === "deprecated" || key === "docHexExceptions" || (key && key.charAt(0) === "$")) return;
  if (typeof value === "string" && /^#[0-9a-fA-F]{6}$/.test(value)) canonical.push(value.toLowerCase());
  else if (Array.isArray(value)) value.forEach(function (item) { collectCanonical(item); });
  else if (value && typeof value === "object") {
    Object.keys(value).forEach(function (childKey) { collectCanonical(value[childKey], childKey); });
  }
}
collectCanonical(PALETTE);
canonical = canonical.filter(function (h, i, a) { return a.indexOf(h) === i; });

var CSS_HEX = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;
var CSS_HEX_IN_TEXT = /#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{4}|[0-9a-fA-F]{3})(?![0-9a-fA-F])/g;

function normalizeOpaqueHex(hex) {
  var lower = hex.toLowerCase();
  if (lower.length === 4) {
    return "#" + lower.slice(1).split("").map(function (ch) { return ch + ch; }).join("");
  }
  return lower.length === 7 ? lower : null;
}

// 逐文件例外：键必须是 skills/ 下相对路径，值保留原 CSS hex 位数，按原值精确匹配。
var exceptions = {};
var exceptionErrors = [];
Object.keys(PALETTE.docHexExceptions || {}).forEach(function (file) {
  if (file.charAt(0) === "$") return;
  var values = PALETTE.docHexExceptions[file];
  if (path.isAbsolute(file) || file.indexOf("*") >= 0 || file.split("/").indexOf("..") >= 0) {
    exceptionErrors.push("非法例外路径: " + file);
    return;
  }
  if (!Array.isArray(values) || values.some(function (hex) { return !CSS_HEX.test(hex); })) {
    exceptionErrors.push("例外值必须是 3/4/6/8 位 CSS hex 数组: " + file);
    return;
  }
  exceptions[file] = values.map(function (hex) { return hex.toLowerCase(); });
});

// 遍历 skills/ 下的 .md 与 templates/*.json
function walk(dir, out) {
  fs.readdirSync(dir, { withFileTypes: true }).forEach(function (ent) {
    var p = path.join(dir, ent.name);
    if (ent.isDirectory()) walk(p, out);
    else if (/\.md$/.test(ent.name) || (/\.json$/.test(ent.name) && dir.indexOf("templates") >= 0)) out.push(p);
  });
  return out;
}

var files = walk(SKILLS_ROOT, []);
var deprecatedHits = [];
var unknownHits = [];
files.forEach(function (file) {
  var relative = path.relative(SKILLS_ROOT, file).split(path.sep).join("/");
  var allowed = exceptions[relative] || [];
  var lines = fs.readFileSync(file, "utf8").split("\n");
  lines.forEach(function (line, i) {
    var m = line.match(CSS_HEX_IN_TEXT);
    if (!m) return;
    m.forEach(function (hex) {
      var raw = hex.toLowerCase();
      var opaque = normalizeOpaqueHex(raw);
      if (opaque && deprecated.indexOf(opaque) >= 0) {
        deprecatedHits.push(relative + ":" + (i + 1) + "  " + hex + "  | " + line.trim().slice(0, 80));
      } else if ((!opaque || canonical.indexOf(opaque) < 0) && allowed.indexOf(raw) < 0) {
        unknownHits.push(relative + ":" + (i + 1) + "  " + hex + "  | " + line.trim().slice(0, 80));
      }
    });
  });
});

var mdText = fs.readFileSync(PALETTE_MD, "utf8").toLowerCase();
var missing = canonical.filter(function (h) { return mdText.indexOf(h) < 0; });

var failed = false;
if (deprecatedHits.length) {
  failed = true;
  console.error("✗ 发现废弃色残留 " + deprecatedHits.length + " 处：");
  deprecatedHits.forEach(function (h) { console.error("  " + h); });
} else {
  console.log("✓ 无废弃色残留（扫描 " + files.length + " 个文件，废弃清单 " + deprecated.length + " 色）");
}
if (unknownHits.length) {
  failed = true;
  console.error("✗ 发现非 canonical 且未登记例外的色值 " + unknownHits.length + " 处：");
  unknownHits.forEach(function (h) { console.error("  " + h); });
} else {
  console.log("✓ 所有 CSS hex（3/4/6/8 位）均来自 canonical 或逐文件精确例外");
}
if (exceptionErrors.length) {
  failed = true;
  console.error("✗ docHexExceptions 配置错误：");
  exceptionErrors.forEach(function (message) { console.error("  " + message); });
} else {
  console.log("✓ docHexExceptions 使用明确的逐文件 CSS hex 配置");
}
if (missing.length) {
  failed = true;
  console.error("✗ palette.md 缺失 canonical 色值：" + missing.join(", "));
} else {
  console.log("✓ palette.md 包含全部 " + canonical.length + " 个 canonical 色值");
}
process.exit(failed ? 1 : 0);
