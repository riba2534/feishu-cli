# 飞书导出

把飞书内容导出成本地文件。读文档到 Markdown 也可以用 `feishu-cli-docs`；本技能更偏“落盘/下载素材/导出文件格式”。

## 路由

| 输入 | 命令 |
|---|---|
| `/docx/<id>` 或 document_id | `feishu-cli doc export` |
| `/wiki/<token>` 或 node_token | `feishu-cli wiki export` |
| `/sheets/<token>` 或 spreadsheet_token | `feishu-cli sheet export --format markdown` |
| 需要 PDF/Word/Excel 文件 | `feishu-cli doc export-file` 或 `feishu-cli drive export` |

`sheet export` 支持直接传 `/sheets/<token>` URL。`wiki export` 会根据节点类型导出 docx 或 sheet。

## Markdown 导出

```bash
# 普通文档
feishu-cli doc export <document_id> --output /tmp/doc.md

# 知识库节点
feishu-cli wiki export <node_token_or_url> --output /tmp/wiki.md

# 普通电子表格
feishu-cli sheet export <spreadsheet_token_or_url> --format markdown -o /tmp/sheet.md
```

CLI 默认输出行为不同，skill 执行时建议显式传输出路径：

| 命令 | 未传输出路径时 |
|---|---|
| `doc export` | 打印到 stdout |
| `wiki export` | 保存到 `/tmp/<title>.md` |
| `sheet export` | 保存为 `<spreadsheet_token>.<format>` |

## doc export 专属参数

```bash
feishu-cli doc export <document_id> \
  --output /tmp/doc.md \
  --download-images \
  --assets-dir /tmp/assets \
  --front-matter \
  --highlight \
  --expand-mentions \
  --expand-sheets
```

| 参数 | 说明 |
|---|---|
| `--download-images` | 下载图片和画板缩略图并改写 Markdown 引用 |
| `--assets-dir` | 素材保存目录 |
| `--front-matter` | 添加 YAML front matter |
| `--highlight` | 保留文本颜色/背景色为 HTML span |
| `--expand-mentions` | 展开 @用户为友好名称 |
| `--expand-sheets` | 默认 true；把文档内嵌电子表格块展开成 Markdown 表格，false 时保留 `<sheet .../>` |

`--front-matter`、`--highlight` 仅 `doc export` 支持；`--expand-sheets`、`--expand-mentions` 同时支持 `doc export` 和 `wiki export`。所有读类导出命令 token 默认走"User 优先 + Tenant 兜底"——已 `auth login` 自动用 User Token，未登录尝试 App Token。

## Sheet Markdown

```bash
# 导出所有可见工作表
feishu-cli sheet export <token_or_url> --format markdown -o /tmp/sheet.md

# CSV 必须指定 sheet-id
feishu-cli sheet export <token_or_url> --format csv --sheet-id <sheet_id> -o /tmp/sheet.csv

# 超大表格 / 慢轮询：调高重试次数（每次间隔 1s）
feishu-cli sheet export <token_or_url> -o /tmp/big.xlsx --max-retries 60
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `-f, --format` | `xlsx` / `csv` / `markdown` | `xlsx` |
| `--sheet-id` | CSV 必填；Markdown 留空导出所有可见工作表 | 空 |
| `--max-retries` | xlsx/csv 异步任务的最大轮询次数（每次 1s） | `30`（≈ 30 秒）|
| `-o, --output` | 输出文件路径 | `<token>.<format>` |

Markdown 输出会按工作表生成标题和表格。大表格用于阅读场景；需要保留公式/样式请导出 xlsx；导出超大表格触发 timeout 时把 `--max-retries` 调到 60–120。

## 文件格式导出

```bash
feishu-cli doc export-file <doc_token> --type pdf -o /tmp/report.pdf
feishu-cli doc export-file <doc_token> --type docx -o /tmp/report.docx
feishu-cli doc export-file <sheet_token> --doc-type sheet --type xlsx -o /tmp/report.xlsx
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `--type` | `pdf` / `docx` / `xlsx` | 必填 |
| `--doc-type` | `docx` / `sheet` 等源文档类型 | `docx` |
| `-o, --output` | 输出路径 | `<doc_token>.<type>` |

长任务或需要 sub-id/resume 时改走下方"长任务/可恢复导出（drive export）"。

## 长任务/可恢复导出（drive export）

`drive export` 是 export_tasks 异步流程的封装：**创建任务 → 有界轮询（默认最多 10 次、每次 5s） → 下载**。超时未完成时返回 `next_command`，可用 `drive task-result` 或 `drive export-download` 接力，避免长任务把 CLI 挂死。

```bash
# 普通文档导出 Markdown（也可直接走 doc export 快捷路径）
feishu-cli drive export --token <docx_token> --doc-type docx --file-extension markdown --output-dir ./out

# 电子表格单 sheet 导出 CSV（sub-id 必填）
feishu-cli drive export --token <sheet_token> --doc-type sheet --file-extension csv --sub-id 0 --output-dir ./out --overwrite

# 多维表格单表导出 CSV
feishu-cli drive export --token <bitable_token> --doc-type bitable --file-extension csv --sub-id <table_id> --output-dir ./out
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `--token` | 源文档 token | 必填 |
| `--doc-type` | `doc` / `docx` / `sheet` / `bitable` | 必填 |
| `--file-extension` | `docx` / `pdf` / `xlsx` / `csv` / `markdown` | 必填 |
| `--sub-id` | sheet/bitable → csv 时必填的子表/工作表 ID | 空 |
| `--output-dir` | 输出目录 | `.` |
| `--overwrite` | 已存在时覆盖 | false |
| `-o, --output` | 设为 `json` 时返回结构化结果（含 next_command） | 文本 |
| `--user-access-token` | 覆盖登录态（必须 User Token） | 自动 |

**轮询超时接力**：JSON 输出中 `next_command` 形如 `feishu-cli drive task-result --scenario export --ticket <ticket> --file-token <doc_token>`；任务已 success 但下载失败则给 `drive export-download --file-token ...`。直接执行该命令即可续上。

**权限**：必须 User Token；scope 需要 `docs:document:export` + `drive:drive.metadata:readonly`。

## 本地文件导入提醒

`doc import-file` 属于“本地文件导入为云文档”，不属于导出；简单格式如下：

```bash
feishu-cli doc import-file report.docx --type docx --name "季度报告"
```

更推荐的异步导入、大小限制和 resume 能力见 `feishu-cli-storage`。

## 单素材下载（doc media-download）

按素材 token 单独下载文档内嵌图片、画板缩略图或附件文件，不依赖 `doc export --download-images` 的批处理流程。适合做手动补抓、画板缩略图导出、超大附件下载。

```bash
# 普通素材文件
feishu-cli doc media-download <token> -o image.png

# 文档内嵌图片（必须提供归属文档以构造 extra）
feishu-cli doc media-download <token> --doc-token <docx_token> --doc-type docx -o image.png

# 画板缩略图
feishu-cli doc media-download <board_id> --type whiteboard -o board   # 自动按实际格式补扩展名（通常 JPEG）

# 大文件，把超时拉到 30 分钟
feishu-cli doc media-download <token> -o large.bin --timeout 30m
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `--type` | `media` / `whiteboard` | `media` |
| `--doc-token` + `--doc-type` | 文档内嵌素材所需归属信息 | docx |
| `--extra` | 原始 extra JSON，优先级高于上两项 | 空 |
| `--timeout` | 下载超时（支持 `10m` / `30m` / `1h`） | `5m` |
| `-o, --output` | 输出路径 | token 同名 |

Token 已登录优先 User Token，未登录回落 App Token。

## 已知限制

- **sheet export markdown 复杂单元格可能丢内容**：电子表格部分单元格（块类型 32 / 富文本嵌套）在转 Markdown 时存在内容丢失风险（见仓库 CLAUDE.md "已知问题"）。**对账/留档场景请优先用 `--format xlsx`**，仅在阅读/diff 场景才用 markdown。
- **doc export 内嵌电子表格展开失败时保留占位**：`--expand-sheets`（默认 true）拉子表失败时输出 `<sheet token="..." id="..." rows=".." cols=".."/>` 占位标签而非报错中断，重新导出或排查权限/网络后再跑一次即可补齐。需要原样保留 token 引用时改用 `--expand-sheets=false`。

## 验证

1. 导出后检查文件存在且大小大于 0。
2. Markdown 场景读前 40 行确认标题、表格、图片路径是否合理；grep 是否残留 `<sheet .../>` 占位（如有需重导）。
3. 下载素材时确认 `assets-dir` 下有对应文件；wiki 批量导出时注意同名素材覆盖风险。
