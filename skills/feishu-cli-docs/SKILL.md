---
name: feishu-cli-docs
description: >-
  飞书云文档正文：读取与总结 docx/wiki/sheet（含转成可直接发 IM 的 Markdown），从零创作文档（PRD、方案、报告、纪要等体裁模板，DocxXML 草稿与 doc script 预检），创建与编辑 docx（带内容新建并上传本地图片/附件/HTML 块/画板源文件、追加、覆盖、按章节或 block 精确替换/删除/移动、插入本地或剪贴板图片、设置文档封面、历史版本查看与回滚），Markdown 导入（Mermaid/PlantUML/SVG 转画板），导出 Markdown/PDF/Word/Excel 与下载或预览文档素材，云盘原生 .md 文件的上传、diff 与覆盖，以及读取和编辑已有思维笔记的节点。用户要阅读、总结、撰写、写入或改写飞书文档，把 Markdown 导入飞书，把文档导出到本地，设置文档封面，操作思维笔记节点，或把误改的文档回滚时使用。不用于：评论（即使请求提到文档）、协作者与权限、DOCX/XLSX 等二进制导入、上传文件的版本和云盘目录，使用 feishu-cli-storage；按单元格读写表格使用 feishu-cli-data；文档内 HTMLBox/ECharts 动态组件使用 feishu-cli-visual；获取会议的纪要、妙记与逐字稿使用 feishu-cli-meetings（撰写纪要类文档仍用本 Skill）。
compatibility: Requires feishu-cli v1.43.0+ and network access for Feishu API calls.
allowed-tools: Bash(feishu-cli:*) Bash(./feishu-cli:*) Bash(./bin/feishu-cli:*) Bash(jq:*) Bash(python3:*) Bash(sleep:*) Read Write
---

# 飞书文档

只读取当前任务需要的工作流；跨步骤任务可以按顺序读取多个。
加载后，将工作流中的 `references/`、`scripts/`、`templates/`、`examples/` 相对路径按该
`workflow.md` 所在目录解析；执行脚本时使用解析后的实际路径，不要依赖当前 shell 目录。

## 路由

| 意图 | 读取文件 |
|---|---|
| 阅读、总结、分析 docx/wiki/sheet，大文档按大纲/章节/关键词局部读取，获取块结构，不主动落盘 | `references/workflows/read/workflow.md` |
| 取文档内容转成可直接发 IM 的 Markdown（`doc read --doc-format im-markdown`，`--lang` 控制 @人显示语言） | `references/workflows/read/workflow.md` |
| 带 block id 读取（`doc read --with-ids` / `--engine docs_ai`），为精确修改做准备 | `references/workflows/read/workflow.md` |
| 从零创作文档（写 / 起草方案、PRD、周报、纪要、报告、教程等），选体裁、写 DocxXML 草稿并用 `doc script` 初始化与预检 | `references/workflows/author/workflow.md` |
| 创建 docx（含 `doc create --content` 带本地图片/附件、HTML 块、画板源文件建文档）、追加、覆盖、替换、删除或移动内容，按 block id 精确改写，插入本地或剪贴板图片/附件（`--from-clipboard`） | `references/workflows/write/workflow.md` |
| 设置、下载或删除文档封面（`doc resource update/download/delete --type cover`） | `references/workflows/write/workflow.md` |
| 查看历史版本、回滚文档（`doc history list/revert/revert-status`） | `references/workflows/write/workflow.md` |
| 把 Markdown 文件导入为飞书 docx（含 Mermaid/PlantUML/SVG 转画板） | `references/workflows/import/workflow.md` |
| 导出 docx/wiki/sheet 到本地 Markdown/PDF/Word/Excel，下载或预览文档内图片、附件、评论图片或画板缩略图（`doc media-download` / `doc media-preview`） | `references/workflows/export/workflow.md` |
| 上传、下载、覆盖、查找替换或比较云盘原生 `.md` | `references/workflows/markdown/workflow.md` |
| 读取已有思维笔记的节点，新增子节点或更新节点（`mindnote nodes list/create`） | `references/workflows/mindnote/workflow.md` |

## 关键边界

- “查看并总结”走 read；明确要求保存到路径才走 export。
- 从零写一篇文档走 author；只建空文档、原样写入用户给定的完整内容或改已有文档走 write。
- Markdown 转为可阅读 docx 走 import；把 `.md` 源文件原样存入云盘走 markdown。
- `mindnote nodes create` 只在已有思维笔记里新增/更新节点，不新建思维笔记；新建思维导图走 `feishu-cli-visual` 的画板。
- DOCX/XLSX 等二进制文件导入走 `feishu-cli-storage` 的 drive 工作流。
- `doc htmlbox` 属于 `feishu-cli-visual`；权限和转移所有权属于 `feishu-cli-storage`。

## 执行规则

1. `/wiki/` 链接或类型不明的 token 先 `drive inspect --url` 解析；Bitable 链接用 `bitable resolve`。wiki 链接解析后区分 node_token 与底层文档 obj_token；底层是 sheet/bitable/slides 时按类型交给对应技能。
2. 修改已有文档一律用 `doc content-update`，不要用 `doc import --document-id`（只会追加到文末）；改几个字用文本级替换，
   改章节或块先 `doc read --with-ids` 拿 block id。写入前确认目标、更新模式和影响范围；`content-update` 没有 dry-run，
   不确定时先在测试文档上验证。
3. 写命令默认 Bot 身份：Bot 新建的文档会自动给当前登录用户授予 `full_access`（输出 `permission_grant`）；编辑用户自己的
   文档而 Bot 无权限时，显式 `--user-access-token "$(feishu-cli auth token --as user)"`。读命令 User 优先、Bot 兜底。
4. 按退出码处理结果：2 = 参数或定位有歧义（选择器多处命中、方言占位无法写回、本地文件不存在），修正参数后重试；
   1 且有失败明细（`doc import` 的 `failures`、`content-update` 的 `partial_success`、本地资源 `failed`）时按明细补齐，
   不要整篇重导；10 = 危险操作缺 `--yes`（`doc delete`、`doc history revert`）。
5. 导入前读取 `references/workflows/import/references/doc-guide.md`。
6. 用户明确指定接收人时按其要求授权；其余按 write 工作流读取生效的 `owner_email` / `transfer_ownership`，仅在已配置 owner 时处理。未要求通知时在当前会话返回文档链接。

删除、覆盖类命令返回退出码 10 时，向用户确认目标与影响后追加全局 `--yes` 重跑，不要自行添加。身份或 scope 报错（如 99991663/99991668/99991672/99991679）读取 `../feishu-cli-platform/references/workflows/auth/references/identity.md`；判断成败、编写脚本或处理确认门禁读取 `../feishu-cli-platform/references/workflows/auth/references/agent-contract.md`。
