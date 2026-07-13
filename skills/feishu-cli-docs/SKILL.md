---
name: feishu-cli-docs
description: >-
  本地 Markdown → 飞书文档的专用导入管道，不是通用文档编辑器。
  核心能力：Markdown 双向无损转换（40+ 块类型）、Mermaid/PlantUML 自动转飞书画板矢量图、
  SVG/本地图片并发上传、大文档三阶段并发管道。
  **何时必须使用本 Skill：** 任务涉及「把本地 .md 文件导入/同步到飞书文档」、
  「Markdown 转飞书」、「含图/表/代码块的文档导入」、「wiki 下从 Markdown 建子页」、
  「Mermaid/PlantUML 图表转飞书」。
  也支持读取、创建、导出飞书文档，以及云盘原生 .md 文件 CRUD。
  **何时不用本 Skill：** 只是改飞书文档里某几个 block 的局部编辑 → 用 lark-doc；
  文档评论 → feishu-cli-storage；动态组件 → feishu-cli-visual。
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
