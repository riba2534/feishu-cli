---
name: feishu-cli-read
description: 读取飞书云文档或知识库内容。当用户请求查看、阅读、分析飞书文档或知识库时使用。支持通过文档 ID、知识库 Token 或 URL 读取。Markdown 作为中间格式存储在 /tmp 目录。
argument-hint: <document_id|node_token|url>
user-invocable: true
allowed-tools: Bash, Read, Grep
---

# 飞书文档阅读技能

从飞书云文档或知识库读取内容，转换为 Markdown 格式后进行分析和展示。

## 核心概念

**Markdown 作为中间态**：本地文档与飞书云文档之间通过 Markdown 格式进行转换，中间文件存储在 `/tmp` 目录中。

## 使用方法

```bash
/feishu-read <document_id>
/feishu-read <node_token>
/feishu-read <url>
```

## 执行流程

1. **解析参数**
   - 判断 URL 类型：
     - `/docx/` → 普通文档，使用 `doc export`
     - `/wiki/` → 知识库文档，使用 `wiki export`
   - 如果是 Token，根据格式判断类型

2. **导出为 Markdown（含图片下载）**

   **普通文档**:
   ```bash
   feishu-cli doc export <document_id> --output /tmp/feishu_doc.md --download-images --assets-dir /tmp/feishu_assets
   ```

   **知识库文档**:
   ```bash
   feishu-cli wiki export <node_token> --output /tmp/feishu_wiki.md --download-images --assets-dir /tmp/feishu_assets
   ```

   **重要**：务必使用 `--download-images` 参数下载文档中的图片到本地。

3. **读取文本内容**
   - 使用 Read 工具读取导出的 Markdown 文件
   - 分析文档结构和文本内容

4. **读取并理解图片内容**
   - 检查 `/tmp/feishu_assets` 目录是否有下载的图片
   - **使用 Read 工具逐个读取图片文件**，理解图片内容
   - 将图片内容整合到文档分析中

   ```bash
   # 列出下载的图片
   ls /tmp/feishu_assets/

   # 使用 Read 工具查看图片（Claude 支持多模态）
   # Read /tmp/feishu_assets/image_1.png
   ```

5. **报告结果**
   - 提供文档摘要（包含图片内容描述）
   - 保留 Markdown 文件和图片供用户进一步操作

## 输出格式

向用户报告：
- 文档标题
- 文档结构概要（标题层级）
- 内容摘要（关键信息）
- **图片内容描述**（如有图片）
- Markdown 文件路径（供后续使用）
- 图片文件路径（如有下载）

## 支持的 URL 格式

| URL 格式 | 类型 | 命令 |
|---------|------|------|
| `https://xxx.feishu.cn/docx/<id>` | 普通文档 | `doc export` |
| `https://xxx.feishu.cn/wiki/<token>` | 知识库 | `wiki export` |
| `https://xxx.larkoffice.com/docx/<id>` | 普通文档 | `doc export` |
| `https://xxx.larkoffice.com/wiki/<token>` | 知识库 | `wiki export` |

## 示例

```bash
# 读取普通文档
/feishu-read <document_id>
/feishu-read https://xxx.feishu.cn/docx/<document_id>

# 读取知识库文档
/feishu-read <node_token>
/feishu-read https://xxx.feishu.cn/wiki/<node_token>
```

## 图片处理流程（重要）

文档中的图片需要特别处理才能理解其内容：

### 步骤 1：导出时下载图片

```bash
# 知识库文档
feishu-cli wiki export <node_token> \
  --output /tmp/doc.md \
  --download-images \
  --assets-dir /tmp/doc_assets

# 普通文档
feishu-cli doc export <document_id> \
  --output /tmp/doc.md \
  --download-images \
  --assets-dir /tmp/doc_assets
```

### 步骤 2：检查下载的图片

```bash
ls -la /tmp/doc_assets/
# 输出示例：
# image_1.png  (403KB)
# image_2.png  (394KB)
```

### 步骤 3：使用 Read 工具查看图片

Claude 支持多模态，可以直接理解图片内容：

```
# 在 Claude 中使用 Read 工具读取图片
Read /tmp/doc_assets/image_1.png
Read /tmp/doc_assets/image_2.png
```

### 步骤 4：整合分析

将图片内容与文档文本结合，提供完整的文档分析。

## 完整示例

```bash
# 1. 导出文档和图片
feishu-cli wiki export <node_token> \
  -o /tmp/wiki_doc.md \
  --download-images \
  --assets-dir /tmp/wiki_assets

# 2. 查看图片列表
ls /tmp/wiki_assets/

# 3. 读取 Markdown 内容
# Read /tmp/wiki_doc.md

# 4. 读取每张图片理解内容
# Read /tmp/wiki_assets/image_1.png
# Read /tmp/wiki_assets/image_2.png

# 5. 综合分析后向用户报告
```

## 注意事项

1. **务必下载图片**：不下载图片只能看到 `feishu://media/<token>` 引用，无法理解图片内容
2. **逐个读取图片**：使用 Read 工具读取每张图片，Claude 会自动理解图片内容
3. **整合分析**：将图片描述与文档文本结合，提供完整的内容摘要
