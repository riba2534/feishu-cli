---
name: feishu-cli-media
description: 素材管理操作。当用户请求上传图片、下载素材、管理飞书文档中的媒体文件时使用。
argument-hint: <subcommand> <file|token> [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书素材管理技能

管理飞书云文档中的素材（图片、文件等），包括上传和下载操作。

## 使用方法

```bash
/feishu-media upload <file> --parent-node <doc_id>    # 上传素材
/feishu-media download <file_token> [--output path]    # 下载素材
```

## CLI 命令详解

### 1. 上传素材

将本地文件上传到飞书云空间，用于文档中的图片或附件。

```bash
# 上传图片到文档
feishu-cli media upload ./image.png --parent-type docx_image --parent-node <document_id>

# 上传文件到文档
feishu-cli media upload ./attachment.pdf --parent-type docx_file --parent-node <document_id>

# 指定文件名
feishu-cli media upload ./photo.jpg --parent-type docx_image --parent-node <doc_id> --name "封面图"

# JSON 格式输出
feishu-cli media upload ./image.png --parent-type docx_image --parent-node <doc_id> --output json
```

**参数说明**：
| 参数 | 说明 | 必需 | 示例 |
|------|------|------|------|
| `file` | 本地文件路径 | 是 | `./image.png` |
| `--parent-type` | 父类型 | 是 | `docx_image` |
| `--parent-node` | 父节点 Token | 是 | 文档 ID |
| `--name` | 自定义文件名 | 否 | `封面图` |
| `--output` | 输出格式 | 否 | `json` |

**parent-type 类型**：
| 值 | 说明 |
|-----|------|
| `docx_image` | 文档中的图片 |
| `docx_file` | 文档中的附件 |
| `sheet_image` | 表格中的图片 |
| `comment_image` | 评论中的图片 |

**输出示例**：
```
素材上传成功！
  文件 Token: boxcnAbCdEfGhIjKlMnOpQrSt
  文件名: image.png
  父节点: doccnXxx
```

**JSON 输出**：
```json
{
  "file_token": "boxcnAbCdEfGhIjKlMnOpQrSt",
  "file_name": "image.png",
  "parent_node": "doccnXxx"
}
```

### 2. 下载素材

从飞书云空间下载文件或图片。

```bash
# 下载到当前目录（使用原文件名）
feishu-cli media download <file_token>

# 下载到指定路径
feishu-cli media download <file_token> --output ./downloads/image.png

# 下载到指定目录（自动命名）
feishu-cli media download <file_token> --output ./downloads/
```

**参数说明**：
| 参数 | 说明 | 必需 | 默认值 |
|------|------|------|--------|
| `file_token` | 文件 Token | 是 | - |
| `--output, -o` | 输出路径 | 否 | 当前目录 |

**输出示例**：
```
已下载到 ./downloads/image.png
  文件大小: 256 KB
  文件类型: image/png
```

## 典型工作流

### 上传图片到文档

```bash
# 1. 上传图片
feishu-cli media upload ./diagram.png --parent-type docx_image --parent-node doccnXxx --output json
# 返回: {"file_token": "boxcnYyy"}

# 2. 在 Markdown 中引用（导入时自动处理）
# ![图片](boxcnYyy)
```

### 批量下载文档图片

```bash
# 1. 导出文档并下载图片
feishu-cli doc export doccnXxx -o doc.md --download-images --assets-dir ./images

# 或手动下载
feishu-cli media download boxcnToken1 -o ./images/
feishu-cli media download boxcnToken2 -o ./images/
```

### 迁移文档图片

```bash
# 1. 从源文档下载图片
feishu-cli media download <old_token> -o /tmp/img.png

# 2. 上传到新文档
feishu-cli media upload /tmp/img.png --parent-type docx_image --parent-node <new_doc_id>
```

## 支持的文件格式

### 图片
- PNG, JPG, JPEG, GIF, BMP, SVG, WEBP

### 文件
- PDF, DOC, DOCX, XLS, XLSX, PPT, PPTX
- ZIP, RAR, 7Z
- TXT, MD, CSV
- 其他常见格式

## 文件大小限制

| 类型 | 大小限制 |
|------|---------|
| 图片 | 20 MB |
| 文件 | 512 MB |

## 权限要求

- `drive:drive:readonly` - 下载文件
- `drive:drive` - 上传文件

## 注意事项

1. **Token 获取**：图片 Token 可从文档导出的 Markdown 中提取
2. **临时链接**：通过 API 获取的图片链接有时效性（通常 24 小时）
3. **批量操作**：建议使用 `doc export --download-images` 批量下载文档图片
4. **格式转换**：飞书可能对上传的图片进行格式转换和压缩
