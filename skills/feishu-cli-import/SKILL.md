---
name: feishu-cli-import
description: 从 Markdown 文件导入创建飞书文档。当用户请求"导入 Markdown"、"从 md 创建文档"、"上传 Markdown"时使用。Markdown 作为中间格式存储在 /tmp 目录。
argument-hint: <markdown_file> [--title "标题"]
user-invocable: true
allowed-tools: Bash, Read
---

# Markdown 导入技能

从本地 Markdown 文件创建或更新飞书云文档。

## 核心概念

**Markdown 作为中间态**：本地文档与飞书云文档之间通过 Markdown 格式进行转换，中间文件存储在 `/tmp` 目录中。

## 使用方法

```bash
# 创建新文档
/feishu-import ./document.md --title "文档标题"

# 更新已有文档
/feishu-import ./document.md --document-id <existing_doc_id>

# 上传本地图片
/feishu-import ./document.md --title "带图文档" --upload-images
```

## 执行流程

### 创建新文档

1. **验证文件**
   - 检查 Markdown 文件是否存在
   - 预览文件内容

2. **执行导入**
   ```bash
   feishu-cli doc import <file.md> --title "<title>" [--upload-images]
   ```

3. **添加权限**
   ```bash
   feishu-cli perm add <document_id> --doc-type docx --member-type email --member-id user@example.com --perm full_access
   ```

4. **发送通知**
   通知用户文档已创建

### 更新已有文档

1. **执行更新**
   ```bash
   feishu-cli doc import <file.md> --document-id <doc_id> [--upload-images]
   ```

2. **通知用户**

## 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| markdown_file | Markdown 文件路径 | 必需 |
| --title | 新文档标题 | 文件名 |
| --document-id | 更新已有文档 | 创建新文档 |
| --upload-images | 上传本地图片 | 否 |

## 支持的 Markdown 语法

- 标题（# ~ ######）
- 段落文本
- 无序/有序列表
- 任务列表（- [ ] / - [x]）
- 代码块（带语言标识）
- 引用块
- 分割线
- 表格
- 粗体、斜体、删除线、行内代码
- 链接

## 输出格式

```
已导入文档！
  文档 ID: <document_id>
  文档链接: https://feishu.cn/docx/<document_id>
  导入块数: 25
```

## 示例

```bash
# 创建新文档
/feishu-import ./meeting-notes.md --title "会议纪要"

# 更新现有文档
/feishu-import ./updated-spec.md --document-id <document_id>

# 带图片导入
/feishu-import ./blog-post.md --title "博客文章" --upload-images
```

## 已验证功能

以下导入功能已通过测试验证（2026-01-21）：

| Markdown 语法 | 导入状态 |
|--------------|---------|
| 标题（# ~ ######） | ✅ 正常 |
| 段落文本 | ✅ 正常 |
| 无序列表 | ✅ 正常 |
| 有序列表 | ✅ 正常 |
| 任务列表 | ✅ 正常 |
| 代码块 | ✅ 正常 |
| 引用块 | ✅ 正常 |
| 分割线 | ✅ 正常 |
| **粗体**/`*斜体*` | ✅ 正常 |
| 行内代码 | ✅ 正常 |
| 表格 | ✅ 结构正常 |

**测试命令**：
```bash
feishu-cli doc import /tmp/test.md --title "测试文档"
# 输出: 已创建文档: <document_id>，添加块数: 16
```
