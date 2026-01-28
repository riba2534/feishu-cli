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

## 错误处理与边界情况

### 1. 常见错误

| 错误 | 原因 | 解决 |
|------|------|------|
| `code=131002, param err` | 参数错误 | 检查 token 格式 |
| `code=131001, node not found` | 节点不存在 | 检查 token 是否正确 |
| `code=131003, no permission` | 无权限访问 | 确认应用有 docx:document 或 wiki:wiki:readonly 权限 |
| `code=99991672, open api request rate limit` | API 限流 | 等待几秒后重试 |
| `write /tmp/xxx.md: permission denied` | 文件权限问题 | 检查输出目录权限，更换输出路径 |

### 2. 边界情况处理

**情况 1：目录节点导出**
- 知识库目录节点导出内容可能显示为 `[Wiki 目录...]`
- 这是正常行为，表示该节点是目录而非实际文档
- 使用 `wiki nodes <space_id> --parent <token>` 获取子节点

**情况 2：文档内容为空**
- 检查文档是否真的为空
- 检查是否有权限查看内容
- 检查是否是目录节点

**情况 3：图片下载失败**
- 检查 `--assets-dir` 目录是否存在且可写
- 检查网络连接
- 图片可能已被删除或权限不足

**情况 4：导出中断**
- 大型文档导出可能耗时较长
- 如果中断，可以重新执行命令
- 使用 `--output` 指定固定路径以便续传

### 3. 重试机制

如果遇到网络错误或 API 限流：

```bash
# 添加 --debug 查看详细错误信息
feishu-cli doc export <doc_id> --debug

# 等待几秒后重试
sleep 5 && feishu-cli doc export <doc_id>
```

## 已知问题

| 问题 | 说明 |
|------|------|
| 表格导出 | 表格内单元格内容可能显示为 `<!-- Unknown block type: 32 -->`，这是块类型 32（表格单元格）的已知转换问题 |
| 目录节点 | 知识库目录节点导出内容为 `[Wiki 目录...]`，需单独获取子节点 |

## 已验证功能

以下导出功能已通过测试验证（2026-01-27）：
- 普通文档导出 ✅
- 知识库文档导出 ✅
- 标题、段落、列表、代码块、引用、分割线 ✅
- 任务列表（Todo）✅
- **图片下载** ✅（使用 `--download-images`）
- 表格结构 ⚠️（内容可能丢失）
- 飞书画板 → 画板链接 ✅

## 双向转换说明

| 导入（Markdown → 飞书） | 导出（飞书 → Markdown） |
|------------------------|------------------------|
| Mermaid 代码块 → 飞书画板 | 飞书画板 → 画板链接 |
| 大表格 → 自动拆分为多个表格 | 多个表格 → 分开的表格 |

**注意**：Mermaid 图表导入后会转换为飞书画板，导出时生成的是画板链接而非 Mermaid 代码。
