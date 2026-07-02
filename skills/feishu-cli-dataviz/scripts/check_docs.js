/**
 * 统一色板文档一致性检查 —— 防止各技能文档中的色值与色板漂移。
 *
 * 检查两件事：
 *   1. 废弃色残留：palette.json 的 deprecated 清单中的任何 hex 出现在
 *      skills/ 下的 .md 或卡片模板 .json 里即报错（大小写不敏感）。
 *   2. 权威文档完整性：references/palette.md 必须包含全部 canonical 色值
 *      （categorical light/dark、派生对、sequential、status）。
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

// 收集 canonical 色（palette.md 必须包含）
var pairKeys = Object.keys(PALETTE.derivedPairs).filter(function (k) { return k.charAt(0) !== "$"; });
var canonical = []
  .concat(PALETTE.categorical.light, PALETTE.categorical.dark, PALETTE.sequentialBlue)
  .concat(Object.keys(PALETTE.status).map(function (k) { return PALETTE.status[k]; }))
  .concat(pairKeys.map(function (k) { return PALETTE.derivedPairs[k].fill; }))
  .concat(pairKeys.map(function (k) { return PALETTE.derivedPairs[k].stroke; }))
  .map(function (h) { return h.toLowerCase(); })
  .filter(function (h, i, a) { return a.indexOf(h) === i; });

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
var hits = [];
files.forEach(function (file) {
  var lines = fs.readFileSync(file, "utf8").split("\n");
  lines.forEach(function (line, i) {
    var m = line.match(/#[0-9a-fA-F]{6}\b/g);
    if (!m) return;
    m.forEach(function (hex) {
      if (deprecated.indexOf(hex.toLowerCase()) >= 0) {
        hits.push(path.relative(SKILLS_ROOT, file) + ":" + (i + 1) + "  " + hex + "  | " + line.trim().slice(0, 80));
      }
    });
  });
});

var mdText = fs.readFileSync(PALETTE_MD, "utf8").toLowerCase();
var missing = canonical.filter(function (h) { return mdText.indexOf(h) < 0; });

var failed = false;
if (hits.length) {
  failed = true;
  console.error("✗ 发现废弃色残留 " + hits.length + " 处：");
  hits.forEach(function (h) { console.error("  " + h); });
} else {
  console.log("✓ 无废弃色残留（扫描 " + files.length + " 个文件，废弃清单 " + deprecated.length + " 色）");
}
if (missing.length) {
  failed = true;
  console.error("✗ palette.md 缺失 canonical 色值：" + missing.join(", "));
} else {
  console.log("✓ palette.md 包含全部 " + canonical.length + " 个 canonical 色值");
}
process.exit(failed ? 1 : 0);
