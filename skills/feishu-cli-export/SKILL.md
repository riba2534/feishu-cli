---
name: feishu-cli-export
description: 将飞书文档或知识库文档导出为 Markdown 文件。当用户请求"导出文档"、"转换为 Markdown"、"保存为 md"时使用。Markdown 作为中间格式存储在 /tmp 目录。
argument-hint: <document_id|node_token|url> [output_path]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书文档导出技能

将飞书云文档或知识库文档导出为本地 Markdown 文件。

## 核心概念

**Markdown 作为中间态**：本地文档与飞书云文档之间通过 Markdown 格式进行转换，中间文件默认存储在 `/tmp` 目录中。

## 使用方法

```bash
# 导出普通文档
/feishu-export <document_id>
/feishu-export <document_id> ./output.md

# 导出知识库文档
/feishu-export <wiki_url>
```

## 执行流程

1. **解析参数**
   - 判断 URL 类型：
     - `/docx/` → 普通文档
     - `/wiki/` → 知识库文档
   - document_id：必需
   - output_path：可选，默认 `/tmp/<id>.md`

2. **执行导出**

   **普通文档**:
   ```bash
   feishu-cli doc export <document_id> --output <output_path>
   ```

   **知识库文档**:
   ```bash
   feishu-cli wiki export <node_token> --output <output_path>
   ```

3. **验证结果**
   - 读取导出的 Markdown 文件
   - 显示文件大小和内容预览

## 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| document_id/node_token | 文档 ID 或知识库节点 Token | 必需 |
| output_path | 输出文件路径 | `/tmp/<id>.md` |
| --download-images | 下载文档中的图片 | 否 |
| --assets-dir | 图片保存目录 | `./assets` |

## 支持的 URL 格式

| URL 格式 | 类型 | 命令 |
|---------|------|------|
| `https://xxx.feishu.cn/docx/<id>` | 普通文档 | `doc export` |
| `https://xxx.feishu.cn/wiki/<token>` | 知识库 | `wiki export` |
| `https://xxx.larkoffice.com/docx/<id>` | 普通文档 | `doc export` |
| `https://xxx.larkoffice.com/wiki/<token>` | 知识库 | `wiki export` |

## 输出格式

```
已导出文档！
  文件路径: /path/to/output.md
  文件大小: 2.5 KB

内容预览:
---
# 文档标题
...
```

## 示例

```bash
# 导出普通文档
/feishu-export <document_id>
/feishu-export <document_id> ~/Documents/doc.md

# 导出知识库文档
/feishu-export https://xxx.feishu.cn/wiki/<node_token>
/feishu-export <node_token> ./wiki_doc.md

# 导出并下载图片
/feishu-export <document_id> --download-images
```

## 图片处理（重要）

导出文档时务必下载图片，以便后续理解图片内容：

### 导出并下载图片

```bash
# 普通文档
feishu-cli doc export <document_id> \
  --output /tmp/doc.md \
  --download-images \
  --assets-dir /tmp/doc_assets

# 知识库文档
feishu-cli wiki export <node_token> \
  --output /tmp/wiki.md \
  --download-images \
  --assets-dir /tmp/wiki_assets
```

### 查看和理解图片

```bash
# 查看下载的图片列表
ls -la /tmp/doc_assets/

# 使用 Read 工具读取图片（Claude 支持多模态）
# Read /tmp/doc_assets/image_1.png
# Read /tmp/doc_assets/image_2.png
```

### 完整流程

1. **导出时添加图片参数**：`--download-images --assets-dir <dir>`
2. **检查图片文件**：`ls <assets_dir>/`
3. **读取图片内容**：使用 Read 工具逐个读取图片
4. **整合分析**：将图片描述与文档文本结合

## 已知问题

| 问题 | 说明 |
|------|------|
| 表格导出 | 表格内单元格内容可能显示为 `<!-- Unknown block type: 32 -->`，这是块类型 32（表格单元格）的已知转换问题 |

## 已验证功能

以下导出功能已通过测试验证：
- 普通文档导出 ✅
- 知识库文档导出 ✅
- 标题、段落、列表、代码块、引用、分割线 ✅
- 任务列表（Todo）✅
- **图片下载** ✅（使用 `--download-images`）
- 表格结构 ⚠️（内容可能丢失）
