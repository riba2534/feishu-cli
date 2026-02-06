---
name: feishu-cli-board
description: 飞书画板操作：下载画板图片、导入 PlantUML/Mermaid 图表、创建画板节点。当用户需要操作飞书画板、导入图表到画板时使用。
argument-hint: <whiteboard_id> [command] [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书画板操作技能

使用 `feishu-cli` 操作飞书画板（白板），支持图表导入、图片下载、节点创建等功能。

## 功能概览

| 功能 | 命令 | 说明 |
|------|------|------|
| 下载图片 | `board image` | 将画板导出为图片 |
| 导入图表 | `board import` | 导入 PlantUML/Mermaid 图表 |
| 获取节点 | `board nodes` | 获取画板节点列表 |
| 创建节点 | `board create-notes` | 在画板上创建节点 |

## 下载画板图片

将飞书画板导出为 PNG 图片：

```bash
# 下载画板图片到指定文件
feishu-cli board image <whiteboard_id> output.png

# 下载到目录（使用画板 ID 作为文件名）
feishu-cli board image <whiteboard_id> ./images/

# JSON 格式输出
feishu-cli board image <whiteboard_id> board.png -o json
```

## 导入图表到画板

将 PlantUML 或 Mermaid 图表导入到飞书画板：

```bash
# 从文件导入 PlantUML 图表
feishu-cli board import <whiteboard_id> diagram.puml

# 导入 Mermaid 图表
feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid

# 直接导入图表代码
feishu-cli board import <whiteboard_id> "@startuml\nA -> B: hello\n@enduml" --source-type content

# 指定图表类型（自动检测失败时使用）
feishu-cli board import <whiteboard_id> diagram.puml --diagram-type sequence

# 使用经典样式
feishu-cli board import <whiteboard_id> diagram.puml --style classic
```

### 图表类型

| 类型 | 说明 |
|------|------|
| `auto` | 自动检测（默认） |
| `mindmap` | 思维导图 |
| `sequence` | 时序图 |
| `activity` | 活动图 |
| `class` | 类图 |
| `er` | ER 图 |
| `flowchart` | 流程图 |
| `state` | 状态图 |
| `component` | 组件图 |

### 已验证的 Mermaid 图表类型（8 种）

以下 8 种 Mermaid 图表类型已全部实测通过，可在飞书画板中正常渲染：

| Mermaid 声明 | diagram_type | 说明 |
|-------------|-------------|------|
| `flowchart TD` / `flowchart LR` | flowchart | 流程图，支持 subgraph |
| `sequenceDiagram` | sequence | 时序图，参与者建议 ≤ 8 |
| `classDiagram` | class | 类图 |
| `stateDiagram-v2` | auto | 状态图，必须用 v2 |
| `erDiagram` | er | ER 图 |
| `gantt` | auto | 甘特图 |
| `pie` | auto | 饼图 |
| `mindmap` | mindmap | 思维导图 |

### 样式类型

| 样式 | 说明 |
|------|------|
| `board` | 画板风格（默认） |
| `classic` | 经典风格 |

## 创建画板节点

在飞书画板上创建便签等节点：

```bash
# 从文件创建节点
feishu-cli board create-notes <whiteboard_id> nodes.json

# 直接传入 JSON
feishu-cli board create-notes <whiteboard_id> '[{"type":"sticky_note","x":100,"y":100,"content":"便签内容"}]' --source-type content

# 使用幂等 token
feishu-cli board create-notes <whiteboard_id> nodes.json --client-token abc123
```

### 节点 JSON 格式

```json
[
  {
    "type": "sticky_note",
    "x": 100,
    "y": 100,
    "content": "便签内容",
    "width": 200,
    "height": 150
  }
]
```

支持的节点类型：
- `sticky_note` - 便签
- `text` - 文本
- `shape` - 形状
- `line` - 线条
- `image` - 图片

## 在文档中添加画板

向飞书文档添加画板块：

```bash
# 在文档末尾添加画板
feishu-cli doc add-board <document_id>

# 在指定位置添加画板
feishu-cli doc add-board <document_id> --parent-id <block_id> --index 0

# JSON 格式输出
feishu-cli doc add-board <document_id> -o json
```

添加成功后返回：
- `block_id` - 画板块 ID
- `whiteboard_id` - 画板 ID（可用于后续操作）

## 权限要求

- `board:board` - 画板读写权限
- `docx:document` - 文档操作权限（用于 add-board）

## 获取画板节点

查看画板中的所有节点信息：

```bash
# 获取画板节点列表（JSON 格式）
feishu-cli board nodes <whiteboard_id>
```

## 最佳实践

1. **图表导入流程**：
   - 先用 `doc add-board` 在文档中创建画板块
   - 获取返回的 `whiteboard_id`
   - 使用 `board import` 导入图表到该画板

2. **Mermaid 图表**：推荐使用 Mermaid 语法，8 种图表类型全部已验证

3. **PlantUML 图表**：支持时序图、活动图、类图、用例图、组件图、ER 图、思维导图等

4. **导入后查看**：使用 `board nodes` 查看画板中导入的节点数据结构

## 示例

```bash
# 完整流程：在文档中添加画板并导入 Mermaid 图表
# 1. 添加画板到文档
feishu-cli doc add-board DOC_ID -o json
# 返回: {"whiteboard_id": "wb_xxx", "block_id": "blk_xxx"}

# 2. 导入 Mermaid 图表到画板
feishu-cli board import wb_xxx diagram.mmd --syntax mermaid

# 3. 查看画板节点
feishu-cli board nodes wb_xxx

# 4. 下载画板图片
feishu-cli board image wb_xxx output.png
```
