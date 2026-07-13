---
name: feishu-cli-data
description: >-
  飞书电子表格与多维表格。核心独有能力：sheet import-md（Markdown GFM 表格 → 飞书电子表格，
  含列宽策略、批量填充优化 70s→3s）。也覆盖读写单元格、导入导出、样式/筛选视图/条件/下拉框/
  浮动图片，多维表格的表/字段/记录/视图/角色/仪表盘/表单/工作流。
  **何时必须用本 Skill：** 任务是把 Markdown 中的表格转成飞书电子表格（sheet import-md）。
  **何时用 lark-sheets/lark-base 更合适：** 图表/透视表/条件格式/公式翻译/Excel 迁移等
  精细化表格操作 → lark-sheets 和 lark-base 覆盖更全。日常表格操作两边都可，但 Markdown
  表格导入只有本 Skill 支持。
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
