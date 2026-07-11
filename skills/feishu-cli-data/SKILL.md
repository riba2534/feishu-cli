---
name: feishu-cli-data
description: >-
  仅用于普通电子表格 Sheet 与多维表格 Bitable/Base，不是所有数据或 JSON 请求的通用入口。
  用户要求读写单元格、
  导入导出表格、设置样式/筛选视图/条件/下拉框/浮动图片，或操作多维表格的表、字段、记录、
  视图、角色、协作者、仪表盘、表单、工作流和数据聚合时使用；也覆盖 --as bot 的 cron/无人值守
  Bitable 场景时必须使用本 Skill。明确禁止用于文档权限/协作者、消息/事件订阅和未封装
  OpenAPI 通用透传；它们分别使用 feishu-cli-storage、feishu-cli-messaging 和
  feishu-cli-platform。文档内 Markdown 表格使用 feishu-cli-docs；数据图表展示使用
  feishu-cli-visual。
argument-hint: <sheet|bitable> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(jq:*), Read, Write
---

# 飞书表格与多维表格

加载工作流后，将其中 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| Sheet 创建、读写、样式、筛选视图、下拉、图片、导入导出 | `references/workflows/sheet/workflow.md` |
| Bitable/Base 表、字段、记录、视图、权限、表单、工作流 | `references/workflows/bitable/workflow.md` |

## 执行规则

1. 先识别 URL：`/sheets/` 是 Sheet，`/base/` 是 Bitable。
2. Sheet 使用 spreadsheet token + sheet ID；Bitable 统一使用 `--base-token`，身份通过 `--as bot|user|auto` 控制。
3. 写操作先用 `--dry-run`（命令支持时），批量操作先用少量记录验证字段类型。
4. 不要把 Bitable field create 的 body 额外包进 `field` 对象；直接使用命令帮助要求的字段 JSON。
