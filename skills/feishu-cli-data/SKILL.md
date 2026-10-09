---
name: feishu-cli-data
description: >-
  飞书电子表格 Sheet 与多维表格 Bitable/Base：按范围读写单元格、类型保真整表读写、样式/合并/冻结/行列增删移动与隐藏、筛选与筛选视图、下拉框、原生单元格图片与浮动图片、Markdown 互转与 XLSX/CSV 导出；Bitable 表、字段、记录（结构化筛选、upsert、批量）、视图、链接解析、角色与成员、仪表盘、表单与分享、工作流和数据聚合，支持 --as bot 的 cron 无人值守。用户给出 /sheets/、/base/ 链接或要处理表格数据时使用。不用于：只需通读或总结整张表（feishu-cli-docs）、把本地 XLSX/CSV 文件导入成飞书表格和文档协作者权限（feishu-cli-storage）、图表可视化展示（feishu-cli-visual）、未封装 OpenAPI 透传（feishu-cli-platform）。
compatibility: Requires feishu-cli v1.43.0+ and network access for Feishu API calls.
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

1. `/wiki/` 链接或类型不明的 token 先 `drive inspect --url` 解析；Bitable 链接用 `bitable resolve`（`?table=` 不一定是数据表，
   `/record/`、表单分享链接也不能直接拆）。`/sheets/` 是 Sheet，`/base/` 是 Bitable。
2. 表格参数与 `--base-token` 都可直接传链接（含 `/spreadsheets/`、知识库 `/wiki/`）。
3. 身份：Sheet 不传 `--as` 时 User 优先、不可用时告警后回退 Bot；Bitable 默认 `--as auto`。cron / 无人值守显式 `--as bot`；
   `--as bot` 报 91403 是 Bot 不是协作者，按工作流把 Bot 加为协作者，不要反复换 token。
4. Sheet 范围前缀可写 sheetId 或子表名（`Sheet1!A1`）；多子表时必须带前缀或 `--sheet-id/--sheet-name`。行列命令
   `--range "3:5"` 是 1 起始两端包含，`--start/--end` 是 0 起始且不含 end；删除行列/子表前先 `--dry-run` 核对实际行号。
5. 长 ID、前导零编码写成 JSON 字符串（超过约 15 位的数字会被舍入）；`table-put` 数字列 dtype 用 `int64` / `float64`（`number` 不合法，会按文本写入）。
   `sheet find/replace` 必须带 `--range`。
6. Bitable 写入用 base/v3 结构：字段 JSON 顶层 `type`，不要包 `field` / `property`；`record batch-create` 用
   `create_records` 或 `fields`+`rows`，不是 v1 的 `records`。写前先用少量记录验证字段类型与选项。
7. 布尔 flag 写 `--flag` 或 `--flag=false`，不要写 `--flag false`（会被当成 true）。删除表/字段/记录/视图没有确认门禁、
   执行即生效；`form field delete` 默认连字段和整列数据一起删，只移除题目时用 `--keep-field`。
8. Sheet 单元格图片用 `sheet image write-image`（单张）/ `write-batch`（批量），不要用 `=IMAGE()` 公式或 Markdown 图片语法。

删除、覆盖类命令返回退出码 10 时，向用户确认目标与影响后追加全局 `--yes` 重跑，不要自行添加。身份或 scope 报错（如 99991663/99991668/99991672/99991679）读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md`；判断成败、编写脚本或处理确认门禁读取 `../feishu-cli-platform/references/workflows/auth/references/agent-contract.md`。
