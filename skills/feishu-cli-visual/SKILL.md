---
name: feishu-cli-visual
description: >-
  飞书可视化与展示统一入口，负责选择合适载体，并覆盖画板、Slides、妙笔BOX 动态组件、
  妙搭 HTML 应用和统一数据可视化设计规范。用户要求画板/whiteboard、架构图、流程图、飞轮、
  鱼骨、路线图、海报、插画、SVG/Mermaid、数据图表或 dashboard，创建 Slides/PPT，嵌入会动的
  ECharts/地图/3D/window.magic 组件，或用妙搭/Miaoda/spark 发布 HTML 应用时必须使用本 Skill。
  消息卡片由 feishu-cli-messaging 构造和发送；Markdown 图表导入由 feishu-cli-docs 执行。
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
