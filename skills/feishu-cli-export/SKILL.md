---
name: feishu-cli-export
description: >-
  将飞书文档、知识库文档或电子表格导出到本地。支持 docx/wiki/sheet 导出 Markdown，
  doc export 内嵌电子表格自动展开，图片/画板素材下载，以及 doc export-file 异步导出
  PDF/Word/Excel。当用户请求导出文档、保存为 Markdown、导出 PDF/Word/Excel、下载文档图片、
  导出表格或表格转 Markdown 时使用。本地导入请用 feishu-cli-import 或 feishu-cli-drive。
argument-hint: <document_id|node_token|spreadsheet_token|url> [output_path]
user-invocable: true
allowed-tools: Bash(feishu-cli doc:*), Bash(feishu-cli wiki:*), Bash(feishu-cli sheet:*), Bash(feishu-cli drive:*), Read
---

# 飞书导出

把飞书内容导出成本地文件。读文档到 Markdown 也可以用 `feishu-cli-read`；本技能更偏“落盘/下载素材/导出文件格式”。

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
```

Markdown 输出会按工作表生成标题和表格。大表格用于阅读场景；需要保留公式/样式请导出 xlsx。

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

长任务或需要 sub-id/resume 时使用 `feishu-cli-drive` 的 `drive export`。

## 本地文件导入提醒

`doc import-file` 属于“本地文件导入为云文档”，不属于导出；简单格式如下：

```bash
feishu-cli doc import-file report.docx --type docx --name "季度报告"
```

更推荐的异步导入、大小限制和 resume 能力见 `feishu-cli-drive`。

## 验证

1. 导出后检查文件存在且大小大于 0。
2. Markdown 场景读前 40 行确认标题、表格、图片路径是否合理。
3. 下载素材时确认 `assets-dir` 下有对应文件；wiki 批量导出时注意同名素材覆盖风险。
