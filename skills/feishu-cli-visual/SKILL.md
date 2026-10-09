---
name: feishu-cli-visual
description: >-
  飞书可视化载体：画板（架构图、流程图、SVG/Mermaid/PlantUML 转可编辑节点、取回 Mermaid 源码、克隆、质检与导出）、飞书 Slides 演示文稿（从 XML 创建、按页增删改、截图）、文档内妙笔 HTMLBox 动态组件（ECharts、地图、3D、动画、交互大屏）、妙搭 HTML 应用发布与访问范围，以及数据图表形式与配色规范。用户要在飞书里画图、做幻灯片/PPT、嵌入会动或可交互的图表、发布 HTML 应用时使用。不用于：只产出本地 SVG/PPTX/HTML 文件；消息卡片（feishu-cli-messaging）；HTML 邮件（feishu-cli-mail）；随 Markdown 导入的图表（feishu-cli-docs）；多维表格仪表盘（feishu-cli-data）。
compatibility: Requires feishu-cli v1.43.0+ and network access for Feishu API calls. SVG conversion needs whiteboard-cli; local checks need Python 3.10+, Node.js and agent-browser.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(python3:*) Bash(node:*) Bash(jq:*) Bash(sleep:*) Bash(whiteboard-cli:*) Read Write
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
| 静态画板：SVG/Mermaid/PlantUML 导入、手写节点、取回源码、克隆、质检、导出 | `references/workflows/board/workflow.md` |
| Slides 幻灯片/PPT：创建、读 XML、按页增删改、截图预览、上传图片 | `references/workflows/slides/workflow.md` |
| 文档内动态、交互、CSS/JS、ECharts、3D、window.magic 小程序 | `references/workflows/htmlbox/workflow.md` |
| 妙搭 HTML 应用：创建、发布、查发布状态、访问范围 | `references/workflows/apps/workflow.md` |

## 载体边界

- 飞书文档内动态或交互：htmlbox。
- 飞书静态画板且节点可编辑：board。
- 飞书演示文稿：slides。
- 用户要求发布为妙搭 HTML 应用：apps。
- 要消息通知卡片：`feishu-cli-messaging` 的 card 工作流。

## 执行规则

1. **身份**：board、`doc htmlbox`、slides 的写命令默认 Bot；操作用户已有的文档、画板或演示文稿时显式传
   `--user-access-token`（或 `FEISHU_USER_ACCESS_TOKEN`）。Slides 新建优先用 User：应用未开通 tenant 的
   `slides:presentation:create` 时 Bot 创建报 99991672（exit 3）。`apps` 全部命令必须 User Token + spark scope。
2. **破坏性操作先问用户**：`board delete`、`slides delete-slide`、把妙搭应用设为 `--scope public` 前先向用户确认。
   `board import/update/create-notes --overwrite` 会清空整张画板（含其他图表），`board update --overwrite` 先加
   `--snapshot old.json` 留备份；`board delete` 没有确认门禁、立即执行。删除类命令以退出码 10 返回时是在等确认，
   不要自行补 `--yes`，先取得用户同意。
3. **不重复写入**：画板写入结果不明时，`board update/svg-import/create-notes/import --engine local` 用同一个
   `--client-token` 重跑；服务端 `board import` 不接受 client token（CLI 自动重试前会回读去重），
   命令失败后要重跑先用 `board nodes` 确认上次没有落地。
4. **妙搭发布需用户明确同意**：先 `apps html-publish --dry-run`，确认 `would_block=false`；`--wait` 或
   `apps release get` 看到 `finished` 才算发布完成，`online_url` 默认仅创建者可见。
5. **落地后自检**：画板用 `board image` 看缩略图（写入后可能滞后 10–20 秒）、Slides 用 `slides screenshot`、
   HTMLBox 落库前跑 `verify.sh`；改动色板或底色后运行 dataviz 校验脚本，原样使用已校验色板无需重复验证。

主题和风格是默认选项，优先满足用户指定的品牌色、明暗主题和载体，并验证可读性。

删除、覆盖类命令返回退出码 10 时，向用户确认目标与影响后追加全局 `--yes` 重跑，不要自行添加。身份或 scope 报错（如 99991663/99991668/99991672/99991679）读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md`；判断成败、编写脚本或处理确认门禁读取 `../feishu-cli-platform/references/workflows/auth/references/agent-contract.md`。
