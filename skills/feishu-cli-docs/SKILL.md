---
name: feishu-cli-docs
description: >-
  飞书文档统一入口，覆盖读取和分析 docx/wiki/sheet、创建与编辑文档、Markdown 导入、
  docx/wiki/sheet 导出 Markdown/PDF/Word/Excel，以及云盘原生 .md 文件 CRUD。用户要求阅读、
  总结、创建、追加、覆盖、替换或删除文档内容，把 Markdown 导入飞书并转换 Mermaid/PlantUML/SVG、
  下载图片或导出本地文件、
  比较和覆盖原生 Markdown 时必须使用本 Skill。
  本 Skill 只处理正文内容和文档/Markdown 文件转换。明确禁止用于文档评论、二进制文件导入、
  云盘目录和权限管理，这些使用 feishu-cli-storage；考勤等工作管理使用 feishu-cli-work；
  动态组件使用 feishu-cli-visual。
  只要意图是评论的 list/reply/resolve，即使请求中出现“文档”，也不要使用本 Skill。
argument-hint: <read|write|import|export|markdown> [args]
user-invocable: true
allowed-tools: Bash(feishu-cli:*), Bash(jq:*), Bash(python3:*), Bash(sleep:*), Read, Write
---

# 飞书文档

只读取当前任务需要的工作流；跨步骤任务可以按顺序读取多个。
加载后，将工作流中的 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 阅读、分析、获取块结构，不主动落盘 | `references/workflows/read/workflow.md` |
| 创建、追加、覆盖、替换或编辑 docx | `references/workflows/write/workflow.md` |
| 把 Markdown 导入为飞书 docx | `references/workflows/import/workflow.md` |
| 导出 docx/wiki/sheet 到本地文件 | `references/workflows/export/workflow.md` |
| 上传、下载、覆盖或比较云盘原生 `.md` | `references/workflows/markdown/workflow.md` |

## 关键边界

- “查看并总结”走 read；明确要求保存到路径才走 export。
- Markdown 转为可阅读 docx 走 import；把 `.md` 源文件原样存入云盘走 markdown。
- DOCX/XLSX 等二进制文件导入走 `feishu-cli-storage` 的 drive 工作流。
- `doc htmlbox` 属于 `feishu-cli-visual`；权限和转移所有权属于 `feishu-cli-storage`。

## 执行规则

1. 解析 URL 后区分普通文档 token 与 wiki node token。
2. 写入前确认目标、更新模式和影响范围；优先 dry-run 或测试文档。
3. 导入前读取 `references/workflows/import/references/doc-guide.md`。
4. 创建文档后按项目 owner_email 规则授权；不要把真实邮箱写进示例。
