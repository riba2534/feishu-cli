# 飞书导出

把飞书内容导出成本地文件。只是"读一下并总结"走 `../read/workflow.md`；本工作流偏"落盘 / 下载素材 / 导出文件格式"。

## 路由

| 输入 / 目标 | 命令 |
|---|---|
| `/docx/<id>`、document_id，或底层是 docx 的 `/wiki/<token>` → Markdown | `feishu-cli doc export` |
| `/wiki/<token>` 或 node_token（docx 或 sheet 节点）→ Markdown | `feishu-cli wiki export` |
| `/sheets/<token>` 或 spreadsheet_token → Markdown/CSV/XLSX | `feishu-cli sheet export` |
| 需要 PDF / Word / Excel 文件 | `feishu-cli doc export-file`（仅裸 token）或 `feishu-cli drive export`（支持 URL/wiki、可续跑） |
| 单个图片、附件或画板缩略图 | `feishu-cli doc media-download` |

## 身份

- `doc export`、`wiki export`、`doc export-file`、`doc media-download` 属于读类：User 优先、Bot（Tenant）兜底。已 `auth login` 时自动用 User Token；User Token 损坏或刷新失败时会在 stderr 告警后改用 Bot（stdout 不受影响），看到告警说明已不是用户身份。
- `sheet export` 另有 `--as bot|user|auto`；不传时同上（User 优先、告警后回退 Bot）。
- `drive export` 用 `--as bot|user|auto`（默认 auto：已配置 User 但解析/刷新失败时直接报错，不静默切 Bot）；cron 场景显式 `--as bot`。

## Markdown 导出

```bash
# 普通文档（也接受 /docx/ URL 与底层为 docx 的 /wiki/ URL）
feishu-cli doc export <document_id_or_url> --output /tmp/doc.md

# 知识库节点（docx 或 sheet）
feishu-cli wiki export <node_token_or_url> --output /tmp/wiki.md

# 普通电子表格（接受 /sheets/、/spreadsheets/、/wiki/ URL）
feishu-cli sheet export <spreadsheet_token_or_url> --format markdown -o /tmp/sheet.md
```

未传输出路径时各命令行为不同，执行时显式传输出路径：

| 命令 | 未传输出路径时 |
|---|---|
| `doc export` | 打印到 stdout |
| `wiki export` | 保存到 `/tmp/<节点标题>.md`（标题里的 `/` 等字符替换为 `_`） |
| `sheet export` | 当前目录 `<spreadsheet_token>.<xlsx\|csv\|md>` |
| `doc export-file` | 当前目录 `<doc_token>.<type>` |

输出路径（`-o`、`--assets-dir`、`--output-dir` 等）落在 `~/.ssh`、`~/.feishu-cli`、`/etc` 等敏感目录时，
在联网前以退出码 2 拒绝，不会先创建导出任务；换用普通目录即可（完整规则见 `feishu-cli-platform` 的 agent-contract）。

### doc export 本地引擎参数

```bash
feishu-cli doc export <document_id> \
  --output /tmp/doc.md \
  --download-images --assets-dir /tmp/assets \
  --front-matter --highlight
```

| 参数 | 说明 |
|---|---|
| `--download-images` | 下载图片、视频与画板到 `--assets-dir`（默认 `./assets`，相对当前目录）并改写引用；画板导出为图片（实测为 JPEG，如 `board_N.jpg`）。用 `--output` 写文件时引用路径相对输出 Markdown 所在目录，导出后可直接对该文件 `doc import`；输出到 stdout 时引用路径相对当前目录 |
| `--front-matter` | 顶部加 YAML front matter（title、document_id） |
| `--highlight` | 文字颜色/背景色输出为 `<span style>` |
| `--expand-mentions` | 默认 true；把 @用户 展开为名字（需 `contact:user.base:readonly`），`=false` 保留可回导的 `<mention-user/>` 标签 |
| `--expand-sheets` | 默认 true；内嵌电子表格展开为 Markdown 表格，`=false` 保留 `<sheet .../>` 引用 |

- `--front-matter`、`--highlight` 只有 `doc export` 有；`wiki export` 支持 `--download-images`、`--assets-dir`、`--expand-mentions`、`--expand-sheets`。
- 未加 `--download-images` 时，图片输出为 `<image token="…" …/>`，画板输出为 `<whiteboard token="…" type="blank"/>` 占位。
- 跨文档引用同步块会自动读取源文档并展开；循环引用、权限不足或 API 失败时保留带 `source_document_id` / `source_block_id` 的 `WARNING` 占位并在 stderr 诊断，不会静默丢内容。使用 `--download-images` 时同步块中的图片、视频和画板按源文档上下文下载。

### docs_ai 引擎（--engine docs_ai）

```bash
# 服务端 Markdown（docs_ai 方言：<title>、<callout emoji>、```mermaid 画板源码、合并单元格表格输出为 HTML <table>）
feishu-cli doc export <doc> --engine docs_ai -o doc.md
# 需要保留 callout 颜色、文字颜色、下划线等样式时用 XML + full（含 block id）
feishu-cli doc export <doc> --engine docs_ai --doc-format xml --detail full -o doc.xml
```

- 适合"导出 → 修改 → `doc content-update` 写回"：方言与写入接口一致；本地引擎导出的 `<whiteboard token … type="blank"/>` 等占位在写回时会被拒绝（见 `../write/workflow.md`）。
- Markdown 序列化会丢失 callout 颜色、文字颜色、下划线；`--detail with-ids|full` 只能配 `--doc-format xml`，否则以退出码 2 报错。
- 本地引擎专属参数（`--download-images`、`--assets-dir`、`--highlight`、`--expand-mentions`、`--expand-sheets`）不能与 docs_ai 同用（退出码 2）；`--front-matter` 两个引擎都可用。
- 画板在同一文档内可按 token 克隆；写回到其它文档常因 `degrade_code=2105` 失败（content-update 以非零退出报告 partial_success）。

## Sheet 导出

```bash
# 所有可见工作表 → Markdown
feishu-cli sheet export <token_or_url> --format markdown -o /tmp/sheet.md

# CSV 必须指定工作表（URL 带 ?sheet= 时可省略 --sheet-id）
feishu-cli sheet export <token_or_url> --format csv --sheet-id <sheet_id> -o /tmp/sheet.csv

# 超大表格 XLSX：调高轮询次数（每次间隔 1s，默认 30 次）
feishu-cli sheet export <token_or_url> -o /tmp/big.xlsx --max-retries 60
```

- `--format` 默认 `xlsx`；Markdown 在本地转换，不走导出任务。`sheet export` 没有 `--download-images`。
- 对账/留档、需要公式与样式时导出 xlsx；Markdown 用于阅读与 diff。
- 超时报"导出任务超时"时把 `--max-retries` 调到 60–120 再试。

## 文件格式导出（PDF / Word / Excel）

```bash
feishu-cli doc export-file <doc_token> --type pdf -o /tmp/report.pdf
feishu-cli doc export-file <doc_token> --type docx -o /tmp/report.docx
feishu-cli doc export-file <sheet_token> --doc-type sheet --type xlsx -o /tmp/report.xlsx
```

- `--type` 必填（`pdf` / `docx` / `xlsx`）；`--doc-type` 默认 `docx`，可选 `doc` / `docx` / `sheet` / `bitable`，组合须与源类型匹配（docx → pdf/docx，sheet/bitable → xlsx）。
- 文档参数接受 token 或 URL：`/docx/`、`/sheets/`、`/base/` URL 按路径推断 `--doc-type`（与显式 `--doc-type` 冲突时报错），`/wiki/` URL 自动解析为底层文档。
- 内部固定轮询约 60 秒；大文档或需要续跑时改走下方 `drive export`。

## 长任务 / 可恢复导出（drive export）

`drive export` 的 docx → Markdown 直接走 docs_ai 读取正文；其他格式使用 export_tasks：**创建任务 → 有界轮询（最多 10 次、每次 5s）→ 下载**。超时未完成时返回 `next_command` 接力。

```bash
# 文档 URL / wiki URL 直接导出（wiki 先解析为底层文档）
feishu-cli drive export --url https://xxx.feishu.cn/docx/<docx_token> --file-extension markdown --output-dir ./out

# 电子表格单 sheet 导出 CSV（--sub-id 必填）
feishu-cli drive export --token <sheet_token> --doc-type sheet --file-extension csv --sub-id <sheet_id> --output-dir ./out --overwrite

# Bot/cron 场景固定身份；dry-run 仅预览，不解析或刷新 Token
feishu-cli drive export --token <docx_token> --doc-type docx --file-extension pdf --as bot --dry-run
```

- 裸 `--token` 时 `--doc-type` 必填；`--file-extension` 须与源类型匹配（doc → docx/pdf；docx → docx/pdf/markdown；sheet → xlsx/csv；bitable → xlsx/csv/base；slides → pptx/pdf）。
- **轮询超时接力**：`-o json` 输出 `timed_out: true` 与 `next_command`（形如 `feishu-cli drive task-result --scenario export --ticket <ticket> --file-token <token> --as <身份>`）；直接执行它续查，**不要重新创建导出任务**，续跑保持同一身份。
- 格式矩阵、scope 与 `drive export-download` 等接力命令以 [Drive 工作流](../../../../feishu-cli-storage/references/workflows/drive/workflow.md) 为准。

## 本地文件导入提醒

`doc import-file` 属于"本地文件导入为云文档"，不属于导出：

```bash
feishu-cli doc import-file report.docx --type docx --name "季度报告"
```

更推荐的异步导入、大小限制和续跑能力见 `feishu-cli-storage` 的 drive 工作流。

## 单素材下载（doc media-download）

按素材 token 单独下载文档内嵌图片、附件或画板缩略图，适合补抓单个素材或导出画板缩略图。

```bash
# 文档内嵌图片：--doc-token 接受文档 token、/docx/ URL 或 wiki URL；-o 不带扩展名时按内容自动补（如 .png）
feishu-cli doc media-download <media_token> --doc-token <docx_token_or_url> -o ./image

# 画板缩略图（按服务端实际格式补扩展名，实测为 .jpg）
feishu-cli doc media-download <whiteboard_id> --type whiteboard -o ./board

# 已存在时覆盖
feishu-cli doc media-download <media_token> --doc-token <docx_token> -o ./image.png --overwrite
```

- 未传 `-o` 时以 token 为文件名（并自动补扩展名）。
- 目标文件已存在且未加 `--overwrite` 时**拒绝覆盖并以退出码 2 报错**；下载先写同目录临时文件，识别类型、检查覆盖后再原子改名，失败不会留下半截文件。
- `--timeout` 默认 5 分钟总时长，大文件可调到 `30m` / `1h`；单个素材超过 100MB 时报错。
- HTTP 403 时按提示补 `--doc-token`（文档内嵌素材按文档鉴权），并确认当前身份对文档有下载权限；`--extra` 可直接传原始 extra JSON，优先于 `--doc-token/--doc-type`。

## 已知限制

- **表格合并单元格**：本地引擎导出的 Markdown 表格无法表达合并关系——合并区域的内容只保留在左上单元格（单元格内多个块用 `<br>` 连接），其余位置为空。需要保留合并结构时用 `--engine docs_ai`（输出带 `rowspan/colspan` 的 HTML `<table>`）或 `doc export-file --type docx`。
- **内嵌电子表格展开失败**：`--expand-sheets`（默认 true）拉子表失败时输出 `<sheet token="…" …/>` 占位而不中断；排查权限/网络后重导，或用 `--expand-sheets=false` 明确保留引用。
- **跨文档同步块展开失败**：输出含 `source_document_id` / `source_block_id` 的 `WARNING`，查看 stderr 诊断并确认当前身份能读取源文档。

## 验证

1. 导出后检查文件存在且大小大于 0。
2. Markdown 场景读前 40 行确认标题、表格、图片路径；grep 是否残留 `<sheet …/>`、`WARNING` 占位（如有需排查后重导）。
3. 下载素材时确认 `--assets-dir` 下有对应文件；批量导出 wiki 时注意同名素材覆盖风险。
