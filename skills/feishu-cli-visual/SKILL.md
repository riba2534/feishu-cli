---
name: feishu-cli-visual
description: >-
  飞书可视化，核心是妙笔BOX——larksuite 完全没有的能力缺口。
  独有能力：妙笔BOX（飞书文档内嵌 ECharts/Three.js 3D/地图飞线/词云/Canvas 粒子/CSS 动画）、
  妙搭/Miaoda HTML 应用发布、SVG→画板原生节点转换（svg_to_board.py）、
  统一数据可视化设计规范（调色板验证、图表形式启发式、反模式清单）。
  也覆盖画板（架构图/流程图/鱼骨/路线图/海报）、Slides/PPT 创建。
  **何时必须用本 Skill：** 任务涉及「文档里嵌会动的图表」「3D/地图/词云」「妙笔BOX」、
  「妙搭发布 HTML」「SVG 转飞书画板节点」。这些 lark-whiteboard 和 lark-slides 做不到。
  **何时用 lark-whiteboard/lark-slides 更合适：** 画板基础导出/更新、PPT 精细化排版 →
  lark 侧技能覆盖更全。消息卡片图表用 feishu-cli-messaging；Markdown 图表导入用
  feishu-cli-docs。
argument-hint: <dataviz|board|slides|htmlbox|apps> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(python3:*), Bash(node:*), Bash(npm:*), Read, Write
---

# 飞书可视化与展示

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

先读 dataviz 选择载体；用户已明确载体时直接读取对应工作流。

## 路由

| 意图 | 读取文件 |
|---|---|
| 未指定载体的数据图表、配色和形式选择 | `references/workflows/dataviz/workflow.md` |
| 静态画板、可编辑节点、SVG、Mermaid/PlantUML | `references/workflows/board/workflow.md` |
| 创建或修改 Slides 演示文稿 | `references/workflows/slides/workflow.md` |
| 文档内动态、交互、CSS/JS、ECharts、3D | `references/workflows/htmlbox/workflow.md` |
| 把 HTML 发布为妙搭应用 | `references/workflows/apps/workflow.md` |

## 载体边界

- 要动态或交互：htmlbox。
- 要静态且节点可编辑：board。
- 要演示文稿：slides。
- 要独立可分享 HTML 应用：apps。
- 要消息通知卡片：`feishu-cli-messaging` 的 card 工作流。

改动色板或底色后运行本 Skill 的 dataviz 校验脚本；原样使用已校验色板无需重复验证。
