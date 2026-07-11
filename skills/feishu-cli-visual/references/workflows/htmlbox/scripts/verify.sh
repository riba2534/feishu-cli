#!/usr/bin/env bash
# 妙笔BOX 落库前本地验证（SKILL 工作流第 2 步）：
#   打开 HTML → 等渲染就绪 → 抓 page error / console → 数渲染节点 → 截图。
# 自动判定「顶层 JS 错误」这一最高频白屏坑（飞书界面看不到、未捕获异常走 pageerror 不进 console），
# 但「画对没画对、在不在动」机器判不了，仍须肉眼看末尾打印的截图。
#
# 用法: scripts/verify.sh <html-file> [wait-seconds]
#   wait 默认 3s；地图 / echarts-gl / Three.js 等 CDN 重的传 4~5。
# 退出码: 0 通过 / 1 自动检查未过 / 2 环境或参数错误。
set -eu

HTML="${1:?用法: verify.sh <html-file> [wait-seconds]（地图/CDN 类传 4~5）}"
WAIT="${2:-3}"

case "$HTML" in /*) ABS="$HTML" ;; *) ABS="$PWD/$HTML" ;; esac
[ -f "$ABS" ] || { echo "❌ 文件不存在: $ABS"; exit 2; }
command -v agent-browser >/dev/null 2>&1 || { echo "❌ 需要 agent-browser（见 browser-router 技能）"; exit 2; }

SHOT="${ABS%.*}.png"

echo "▶ 打开 file://${ABS}（等待 ${WAIT}s 让 CDN / 动画就绪）"
# 用全新 session 打开：agent-browser 的 page error buffer 跨页累积、且 --clear 清不掉，
# 只有 close 重开能保证读到的 page error 只属于当前页（代价：会重置已有的 agent-browser 会话）。
agent-browser close --all >/dev/null 2>&1 || true
agent-browser open "file://${ABS}" >/dev/null 2>&1 || true
agent-browser wait "$((WAIT * 1000))" >/dev/null 2>&1 || true

# 渲染节点数：图表类应 > 0；纯 CSS / KPI 类天然为 0，故只作软提示。
NODES=$(agent-browser eval 'document.querySelectorAll("canvas,svg").length' 2>/dev/null | tr -dc '0-9' || true)
NODES="${NODES:-0}"
# 状态提示 #st：骨架渲染成功后会清空；仍停在「加载中…」或「…失败」说明卡住/CDN 挂了。
STATUS=$(agent-browser eval 'var e=document.querySelector("#st");e?e.textContent.trim():""' 2>/dev/null || true)
# page error 是硬失败信号（顶层 JS 异常走这里，不进 console）。
ERRS=$(agent-browser errors 2>/dev/null | tr -d '[:space:]' || true)
CONS=$(agent-browser console 2>/dev/null || true)

agent-browser screenshot "$SHOT" >/dev/null 2>&1 || true

echo "  canvas/svg 节点数 : $NODES"
echo "  状态提示(#st)     : ${STATUS:-（无 #st 或已清空）}"
echo "  截图              : $SHOT"

FAIL=0
if [ -n "$ERRS" ]; then
  echo "❌ 检测到 page error（顶层 JS 异常 → 飞书里会白屏且不报错）:"
  agent-browser errors 2>/dev/null | sed 's/^/    /' || true
  FAIL=1
fi
if printf '%s' "$STATUS" | grep -qiE '失败|error|加载中'; then
  echo "❌ 状态提示异常（CDN 没加载完 / 卡在加载中）: $STATUS"
  FAIL=1
fi
if echo "$CONS" | grep -qiE 'error|failed|失败'; then
  echo "⚠ console 有错误/告警（辅助信号，逐条核对是否致命）:"
  echo "$CONS" | grep -iE 'error|failed|失败' | sed 's/^/    /' || true
fi
if [ "$NODES" -eq 0 ]; then
  echo "⚠ 无 canvas/svg 节点：纯 CSS / KPI 类图属正常；ECharts / Three.js 类则疑似白屏，重点看截图。"
fi

echo "————————————————————————"
if [ "$FAIL" -eq 0 ]; then
  echo "✅ 自动检查通过（无 page error、状态正常）。"
  echo "   仍须肉眼看 $SHOT 确认「画对了、在动」——有节点 ≠ 画对。确认后再 doc htmlbox create。"
else
  echo "❌ 自动检查未过，别带病落库。排查见 references/pitfalls.md（白屏系统排查法）。"
fi
exit "$FAIL"
