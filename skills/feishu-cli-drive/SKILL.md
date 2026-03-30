---
name: feishu-cli-drive
description: >-
  飞书云空间文件管理：文件列表、文件夹、移动、复制、删除、创建快捷方式、
  上传、下载、版本管理、元数据和统计信息。也覆盖素材上传下载的底层文件操作。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书云空间技能

这个技能对齐官方 lark-drive 的“文件 / 云空间”入口。

## 常用命令

```bash
feishu-cli file list
feishu-cli file meta <file_token>
feishu-cli file stats <file_token>
feishu-cli file mkdir "新文件夹" --parent <folder_token>
feishu-cli file move <file_token> --target <folder_token> --type docx
feishu-cli file copy <file_token> --target <folder_token> --type docx
feishu-cli file delete <file_token> --type docx
feishu-cli file version list <file_token>
feishu-cli file upload /path/to/file.pdf --parent <folder_token>
feishu-cli file download <file_token> -o output.pdf
feishu-cli media upload image.png --parent-type doc_image --parent-node <document_id>
feishu-cli media download <media_token> --output image.png
```

## 何时使用

- 用户要“找文件”“列文件夹”“下载文件”“上传素材”
- 用户要查文件版本、元数据、统计
- 用户要处理文档附件，但不想进入文档编辑流程
