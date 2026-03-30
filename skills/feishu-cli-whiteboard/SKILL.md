---
name: feishu-cli-whiteboard
description: >-
  飞书画板：导入 Mermaid / PlantUML 图表、创建画板节点、获取画板图像和节点内容。
  适用于白板、流程图、架构图和图表导入导出场景。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书画板技能

这个技能对齐官方 lark-whiteboard 的入口。

## 常用命令

```bash
feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid
feishu-cli board image <whiteboard_id> board.png
feishu-cli board nodes <whiteboard_id>
feishu-cli board create-notes <whiteboard_id> nodes.json
feishu-cli board update <whiteboard_id> nodes.json --overwrite
feishu-cli board delete <whiteboard_id> --node-ids node1,node2
```

## 重点

- Mermaid 推荐优先使用
- 图表复杂时要控制 participant 数和嵌套层数
- 画板编辑前先确认是不是已有白板内容
