---
name: feishu-cli-visual
description: >-
  为飞书创建或编辑画板、Slides、文档内 HTMLBox 动态组件和妙搭 HTML 应用，选择图表与配色。适用于明确需要飞书载体的架构图、SVG/Mermaid、演示文稿、ECharts、地图或交互大屏。仅要求本地 SVG、PPTX 或 HTML 时不适用。消息卡片使用 feishu-cli-messaging；Markdown 图表导入使用 feishu-cli-docs。
compatibility: Requires feishu-cli v1.41.0+ and network access for Feishu API calls. SVG conversion needs whiteboard-cli; local checks need Python 3.10+, Node.js and agent-browser.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(python3:*) Bash(node:*) Bash(npm:*) Read Write
---

# 飞书可视化与展示

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

仅在目标是飞书产物时选择下列载体；用户只要本地文件时保留原交付方式。
飞书载体尚未确定时先读 dataviz；已明确载体时直接读取对应工作流。

## 路由

| 意图 | 读取文件 |
|---|---|
| 未指定载体的数据图表、配色和形式选择 | `references/workflows/dataviz/workflow.md` |
| 静态画板、可编辑节点、SVG、Mermaid/PlantUML | `references/workflows/board/workflow.md` |
| 创建或修改 Slides 演示文稿 | `references/workflows/slides/workflow.md` |
| 文档内动态、交互、CSS/JS、ECharts、3D | `references/workflows/htmlbox/workflow.md` |
| 把 HTML 发布为妙搭应用 | `references/workflows/apps/workflow.md` |

## 载体边界

- 飞书文档内动态或交互：htmlbox。
- 飞书静态画板且节点可编辑：board。
- 飞书演示文稿：slides。
- 用户要求发布为妙搭 HTML 应用：apps。
- 要消息通知卡片：`feishu-cli-messaging` 的 card 工作流。

改动色板或底色后运行本 Skill 的 dataviz 校验脚本；原样使用已校验色板无需重复验证。
主题和风格是默认选项，优先满足用户指定的品牌色、明暗主题和载体，并验证可读性。

遇到 Token、身份或 scope 报错（如 99991663/99991668/99991672/99991679）时，读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md` 确认应使用的身份与预检方式，排错表见 `../feishu-cli-platform/references/workflows/auth/workflow.md`。
