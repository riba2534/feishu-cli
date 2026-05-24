---
name: feishu-cli-write
description: >-
  向飞书文档写入内容、创建新文档、新建空白文档。支持 doc import 从 Markdown 创建、
  doc content-update 精准追加/覆盖/替换/插入/删除，doc add/update/delete 低层块操作，
  以及图片/文件插入。当用户请求创建文档、写文档、更新文档、替换章节、插入章节、
  删除章节、追加内容、覆盖文档、插入图片或文件时使用。
argument-hint: <title|document_id> [content]
user-invocable: true
allowed-tools: Bash(feishu-cli doc:*), Bash(feishu-cli perm:*), Bash(feishu-cli msg:*), Bash(python3:*), Write, Read
---

# 飞书文档写入

本技能负责创建和编辑飞书 docx。Markdown 文件导入创建文档也可直接用 `feishu-cli-import`；只读/导出走 `feishu-cli-read` / `feishu-cli-export`。

## 新建文档

```bash
feishu-cli doc create --title "文档标题" --output json
```

创建后如需交付给用户，先解析 owner：

1. 优先读取 `FEISHU_OWNER_EMAIL`，其次读取 `~/.feishu-cli/config.yaml` 的 `owner_email`。
2. 解析到 owner 后授予 `full_access`：
   ```bash
   feishu-cli perm add <document_id> --doc-type docx --member-type email --member-id <owner_email> --perm full_access --notification
   ```
3. 仅当 `FEISHU_TRANSFER_OWNERSHIP=true` 或配置 `transfer_ownership: true` 时转移所有权：
   ```bash
   feishu-cli perm transfer-owner <document_id> --doc-type docx --member-type email --member-id <owner_email> --notification
   ```
4. 未配置 owner 时，不要使用占位邮箱，提示用户设置 `FEISHU_OWNER_EMAIL`。

## 用 Markdown 创建文档

```bash
feishu-cli doc import /tmp/doc.md --title "标题" --upload-images
```

写入临时 Markdown 后先做编码检查：

```bash
python3 -c "d=open('/tmp/doc.md','rb').read(); assert b'\xef\xbf\xbd' not in d; d.decode('utf-8')"
```

`doc import` 已在 CLI 内校验非法 UTF-8 和 U+FFFD，但生成阶段仍建议先检查，避免把乱码写入云文档。

## 编辑已有文档

不要把 `doc import --document-id` 当成更新命令；它会把 Markdown 转成新块追加到文档末尾。已有文档编辑优先用 `doc content-update`。

| 用户意图 | 推荐模式 |
|---|---|
| 在末尾新增内容 | `append` |
| 完全重写文档 | `overwrite` |
| 替换某个章节 | `replace_range` |
| 全文查找替换 | `replace_all` |
| 在某个章节前/后插入 | `insert_before` / `insert_after` |
| 删除某个章节 | `delete_range` |

常用示例：

```bash
# 按标题替换章节
feishu-cli doc content-update <document_id> --mode replace_range \
  --selection-by-title "## 旧章节" \
  --markdown-file /tmp/new-section.md

# 在章节后插入
feishu-cli doc content-update <document_id> --mode insert_after \
  --selection-by-title "## 目标章节" \
  --markdown "## 新增章节\n\n内容"

# 追加到末尾
feishu-cli doc content-update <document_id> --mode append \
  --markdown-file /tmp/append.md

# 完全覆盖
feishu-cli doc content-update <document_id> --mode overwrite \
  --markdown-file /tmp/full.md
```

关键规则：用户说“修改/替换/更新某段”时用 `replace_range` 或 `replace_all`，不要 append 导致重复。

## Markdown 图片

`doc import` 默认上传图片；`doc add` / `content-update` 使用 `--upload-images` 上传 Markdown 中的本地/网络图片并回填 Image Block。

```bash
feishu-cli doc content-update <document_id> --mode append \
  --markdown-file /tmp/with-image.md \
  --upload-images
```

单独插入图片或文件用 `doc media-insert`：

```bash
feishu-cli doc media-insert <document_id> --file /path/to/image.png --type image --align center --caption "说明"
feishu-cli doc media-insert <document_id> --file /path/to/report.pdf --type file
```

## 低层块操作

```bash
feishu-cli doc add <document_id> content.md --content-type markdown --upload-images
feishu-cli doc update <document_id> <block_id> --content-file update.json
feishu-cli doc delete <document_id> --start-index 3 --end-index 5
feishu-cli doc batch-update <document_id> updates.json --source-type file
```

低层 JSON 需要熟悉飞书 Block 结构；普通章节编辑优先用 `content-update`。

## 表格

Markdown 表格导入 docx 时：

- 行数 > 9：CLI 创建 9 行初始表，再用 `insert_table_row` 追加到同一 block。
- 列数 > 9：按列组拆分，保留首列作为标识。
- 行数极多（200+）时更适合用 Sheet：`feishu-cli sheet import-md report.md --title "报表"`。

文档内已有表格结构操作：

```bash
feishu-cli doc table insert-row DOC_ID TABLE_BLOCK_ID --index 1 --count 2
feishu-cli doc table delete-rows DOC_ID TABLE_BLOCK_ID --start 1 --end 3
feishu-cli doc table merge-cells DOC_ID TABLE_BLOCK_ID --row-start 0 --row-end 2 --col-start 0 --col-end 3
```

## 扩展语法

`doc import` / `content-update` 支持常见 Markdown，以及导出端生成的 HTML 扩展标签：

```html
<mention-user id="ou_xxx"/>
<mention-doc token="doc_token_xxx" type="docx">标题</mention-doc>
<callout type="NOTE">内容</callout>
<grid cols="2"><column>左</column><column>右</column></grid>
```

Mermaid / PlantUML 会在导入时转为飞书画板；语法限制参考 `../feishu-cli-import/references/doc-guide.md`。

## 验证

1. 创建/更新后确认返回 document_id 或成功状态。
2. 需要交付时确认 owner 权限已添加。
3. 图片/表格/图表较多时查看命令输出中的成功统计。
4. 重大覆盖操作前先确认用户明确要求 `overwrite`。
