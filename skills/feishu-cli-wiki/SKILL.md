---
name: feishu-cli-wiki
description: >-
  飞书知识库：获取知识库节点、列出空间、列出节点、导出节点内容，以及管理成员和节点移动。
  当用户明确在知识库里找文档或想处理 wiki 节点时使用。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书知识库技能

这个技能对齐官方 lark-wiki 的入口。

## 常用命令

```bash
feishu-cli wiki create --space-id <space_id> --title "新文档"
feishu-cli wiki update <node_token> --title "新标题"
feishu-cli wiki delete <node_token>
feishu-cli wiki move <node_token> --target-space <space_id>
feishu-cli wiki get <node_token>
feishu-cli wiki export <node_token> -o doc.md
feishu-cli wiki space-get <space_id>
feishu-cli wiki spaces
feishu-cli wiki nodes <space_id>
feishu-cli wiki member list <space_id>
feishu-cli wiki member add <space_id> --member-id <id>
feishu-cli wiki member remove <space_id> --member-id <id>
```

## 提示

- 如果用户给的是 `/wiki/...` 链接，先确认是不是知识库节点
- 如果后续要读文档内容，必要时切回文档相关技能
