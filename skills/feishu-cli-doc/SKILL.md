---
name: feishu-cli-doc
description: >-
  飞书文档：创建、导入、导出、读取和增量更新文档，插入图片/文件/画板，
  以及处理 Markdown 与飞书文档之间的转换。适用于文档创建、阅读、编辑、导出、
  追加、替换、删除章节等场景。
user-invocable: true
allowed-tools: Bash, Read, Write
---

# 飞书文档技能

这个技能对齐官方 lark-doc 的入口粒度，但命令全部使用 feishu-cli 的实际能力。

## 优先级

1. 只读：用 [feishu-cli-read](../feishu-cli-read/SKILL.md)
2. 写入与编辑：用 [feishu-cli-write](../feishu-cli-write/SKILL.md)
3. Markdown 导入：用 [feishu-cli-import](../feishu-cli-import/SKILL.md)
4. 导出与素材：用 [feishu-cli-export](../feishu-cli-export/SKILL.md)

## 常用命令

```bash
feishu-cli doc create --title "新文档"
feishu-cli doc export <document_id> -o output.md --download-images
feishu-cli doc import input.md --title "文档标题" --upload-images
feishu-cli doc add <document_id> -c '{"elements":[]}'
feishu-cli doc content-update <document_id> --mode append --markdown "## 新内容"
feishu-cli doc media-insert <document_id> --file image.png --type image
feishu-cli doc media-download <file_token> -o image.png
```

## 使用建议

- 读文档优先走 `feishu-cli-read`。
- 新建或重写文档优先走 `feishu-cli-write`。
- 将本地 Markdown 导入飞书优先走 `feishu-cli-import`。
- 导出为 Markdown / 下载素材优先走 `feishu-cli-export`。

