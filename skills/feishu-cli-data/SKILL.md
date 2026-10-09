---
name: feishu-cli-data
description: >-
  仅用于普通电子表格 Sheet 与多维表格 Bitable/Base，不是所有数据或 JSON 请求的通用入口。
  用户要求读写单元格、
  导入导出表格、设置样式/筛选视图/条件/下拉框/原生单元格图片/浮动图片，或操作多维表格的表、字段、记录、
  视图、角色、协作者、仪表盘、表单、工作流和数据聚合时使用；也覆盖 --as bot 的 cron/无人值守
  Bitable 场景时必须使用本 Skill。明确禁止用于文档权限/协作者、消息/事件订阅和未封装
  OpenAPI 通用透传；它们分别使用 feishu-cli-storage、feishu-cli-messaging 和
  feishu-cli-platform。文档内 Markdown 表格使用 feishu-cli-docs；数据图表展示使用
  feishu-cli-visual。
compatibility: Requires feishu-cli v1.42.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(jq:*) Read Write
---

# 飞书表格与多维表格

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| Sheet 筛选视图与条件、下拉菜单、浮动图片 / 单元格图片、批量样式，以及全部 sheet 命令的决策要点 | `references/workflows/sheet/workflow.md` |
| Sheet 创建与元信息、读写追加、`table-get` / `table-put` 类型保真、行列与子表管理、查找替换、筛选、保护、Markdown 互转 | `references/workflows/sheet/workflow.md`，再读 `references/workflows/sheet/references/basic-commands.md` |
| Bitable/Base 链接解析（resolve、block）、表、字段、记录（含附件、批量、分页）、视图与视图配置、data-query | `references/workflows/bitable/workflow.md` |
| Bitable 角色与协作者、高级权限、工作流、仪表盘与图表块、表单与分享 | `references/workflows/bitable/workflow.md` |

## 执行规则

1. 先识别 URL：`/sheets/` 是 Sheet，`/base/` 是 Bitable。
2. 表格参数与 `--base-token` 都可直接传链接（含 `/spreadsheets/`、知识库 `/wiki/`）。Bitable 链接里的 `?table=` 不一定是数据表，
   `/record/`、表单分享链接也不能直接拆，先 `bitable resolve --url` 判型。
3. 身份：Sheet 不传 `--as` 时 User 优先、不可用时告警后回退 Bot；Bitable 默认 `--as auto`。cron / 无人值守显式 `--as bot`；
   `--as bot` 报 91403 是 Bot 不是协作者，按工作流把 Bot 加为协作者，不要反复换 token。
4. Sheet 范围前缀可写 sheetId 或子表名（`Sheet1!A1`）；多子表时必须带前缀或 `--sheet-id/--sheet-name`。行列命令
   `--range "3:5"` 是 1 起始两端包含，`--start/--end` 是 0 起始且不含 end；删除行列/子表前先 `--dry-run` 核对实际行号。
5. 数据类型：长 ID、前导零编码写成 JSON 字符串（JSON 数字超过约 15 位会被舍入）；`table-put` 的数字列 dtype 写
   `int64` / `float64`，`number` 不是合法 dtype 会按文本写入。`sheet find/replace` 必须带 `--range`。
6. Bitable 写入用 base/v3 结构：字段 JSON 顶层 `type`，不要包 `field` / `property`；`record batch-create` 用
   `create_records` 或 `fields`+`rows`，不是 v1 的 `records`。写前先用少量记录验证字段类型与选项。
7. 布尔 flag 写 `--flag` 或 `--flag=false`，不要写 `--flag false`（会被当成 true）。删除表/字段/记录/视图没有确认门禁、
   执行即生效；`form field delete` 默认连字段和整列数据一起删，只移除题目时用 `--keep-field`。
8. Sheet 单元格图片用 `sheet image write-image`（单张）/ `write-batch`（批量），不要用 `=IMAGE()` 公式或 Markdown 图片语法。

遇到 Token、身份或 scope 报错（如 99991663/99991668/99991672/99991679）时，读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md` 确认应使用的身份与预检方式，排错表见 `../feishu-cli-platform/references/workflows/auth/workflow.md`。
