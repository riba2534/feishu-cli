# 画板操作详细参考

## 下载画板图片

将画板导出为 PNG 图片：

```bash
feishu-cli board image <whiteboard_id> output   # 自动按实际格式补扩展名（服务端实际返回 JPEG）
```

## 导入图表到画板

### 从文件导入

```bash
# PlantUML 文件（默认）
feishu-cli board import <whiteboard_id> diagram.puml

# Mermaid 文件
feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid

# 指定图表类型
feishu-cli board import <whiteboard_id> diagram.puml --diagram-type sequence

# SVG：服务端拆成可编辑原生节点（syntax_type=3），不支持的属性列在 degraded_attributes
feishu-cli board import <whiteboard_id> drawing.svg --syntax svg -o json
```

### 从内容直接导入

```bash
feishu-cli board import <whiteboard_id> "graph TD; A-->B" \
  --source-type content \
  --syntax mermaid \
  --diagram-type flowchart
```

### 导入参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--syntax` | `plantuml` / `mermaid` / `svg`，未知取值报用法错误（exit 2） | `plantuml` |
| `--diagram-type` | 图表类型字符串：auto/mindmap/sequence/activity/class/er/flowchart/state/component（只作用于 PlantUML/Mermaid） | `auto` |
| `--style` | `board` 或 `classic`（只作用于 PlantUML/Mermaid） | `board` |
| `--client-token` | 幂等键（≥10 字符），仅 `--engine local` 生效 | 空 |
| `--source-type` | `file` 或 `content` | `file` |
| `<source>` | source-type=content 时直接传图表源码；source-type=file 时传文件路径 | 必填 |

### diagram-type 映射

| CLI 值 | 说明 |
|--------|------|
| auto | 自动检测 |
| mindmap | 思维导图 |
| sequence | 时序图 |
| activity | 活动图 |
| class | 类图 |
| er | ER 图 |
| flowchart | 流程图 |
| state | 状态图 |
| component | 组件图 |

## 取回图表源码

服务端导入的 Mermaid / PlantUML 图表在 section 节点上保留 `syntax.code`，可原样取回再编辑：

```bash
feishu-cli board export-code <whiteboard_id> --source                          # 只有一个图表时直接打印
feishu-cli board export-code <whiteboard_id> --source --node-id t1:2           # 多个图表时指定节点
feishu-cli board export-code <whiteboard_id> --source --node-id t1:2 --output-path diagram   # 自动补 .mmd/.puml
```

改完用 `board import <whiteboard_id> diagram.mmd --syntax mermaid --overwrite` 写回。

## 写入幂等（client_token）

`/nodes` 端点（`board update`、`board svg-import`、`board create-notes`、`board import --engine local`）
对同一 `client_token` 只写一次，重复请求直接返回首次的节点 ID。网络超时等结果未知时，带同一个值重跑：

```bash
feishu-cli board update <whiteboard_id> nodes.json --client-token fp-nodes-20260101-001
```

`/nodes/plantuml`（`board import` 服务端引擎）实测**不认** client_token，CLI 改为重试前回读顶层节点去重；
文档导入（`doc import`）的图表重试使用 `overwrite` 覆盖本次新建的空画板，同样不会叠图。

## 获取画板节点

```bash
feishu-cli board nodes <whiteboard_id>
```

## 在文档中添加画板

```bash
# 在文档末尾添加空白画板
feishu-cli doc add-board <document_id>

# 在指定位置添加
feishu-cli doc add-board <document_id> --parent-id <block_id> --index 0
```

## 支持的 Mermaid 图表类型

以下 8 种类型全部经过实际验证：

| 类型 | diagram_type | 验证状态 |
|------|-------------|---------|
| flowchart | 6 | 通过（支持 subgraph） |
| sequenceDiagram | 2 | 通过 |
| classDiagram | 4 | 通过 |
| stateDiagram-v2 | 0（auto） | 通过 |
| erDiagram | 5 | 通过 |
| gantt | 0（auto） | 通过 |
| pie | 0（auto） | 通过 |
| mindmap | 1 | 通过 |

## 画板 API 技术说明

- API 端点：`/open-apis/board/v1/whiteboards/{id}/nodes/plantuml`
- `syntax_type=1` 表示 PlantUML，`syntax_type=2` 表示 Mermaid，`syntax_type=3` 表示 SVG
- 空画板 `GET /nodes` 返回 `{"code":0,"data":{}}`（没有 nodes 字段），按"无节点"处理
- 使用通用 HTTP 请求方式（client.Get/Post），非专用 SDK 方法

## 权限要求

| 权限 | 说明 |
|------|------|
| `board:board` | 画板操作 |
| `docx:document` | 文档中添加画板 |

## 创建画板节点

通过 JSON 批量创建画板节点（形状、连接线等）：

```bash
# 从文件创建节点
feishu-cli board create-notes <whiteboard_id> nodes.json

# 直接传入 JSON
feishu-cli board create-notes <whiteboard_id> '[{"type":"composite_shape","x":100,"y":100,"width":200,"height":50,"composite_shape":{"type":"round_rect"},"text":{"text":"Hello"},"style":{"fill_color":"#611dc5","border_style":"none","fill_opacity":100}}]' --source-type content

# JSON 输出（返回节点 ID 列表）
feishu-cli board create-notes <whiteboard_id> nodes.json -o json
```

详细的节点格式和高级用法请参考 `references/node-api.md`。

## 画板图片节点

画板中插入图片需要特殊的上传和创建流程：

```bash
# 1. 上传图片（必须用 whiteboard 类型 + 画板 ID）
feishu-cli media upload image.png --parent-type whiteboard --parent-node <whiteboard_id> -o json
# 返回 {"file_token": "xxx"}

# 2. 创建图片节点（token 必须嵌套在 image 对象内）
feishu-cli board create-notes <whiteboard_id> \
  '[{"type":"image","x":100,"y":100,"width":86,"height":86,"image":{"token":"<file_token>"},"z_index":100}]' \
  --source-type content
```

**关键注意事项**：
- `parent_type` 必须是 `whiteboard`（不是 `docx_image`），否则图片在画板中显示为棋盘格
- `parent_node` 必须是画板 ID（不是文档 ID）
- token 格式：`{"image":{"token":"xxx"}}`（嵌套），不能放顶层
- 每个图片节点需要独立的 token，同一张图片用于多个节点时必须分别上传
- 圆形头像：API 不支持 `clip`/`mask`/`border_radius`，需预处理图片为圆形后上传

详见 `references/node-api.md` 的 image 节点章节。

## 已知限制

| 限制 | 说明 |
|------|------|
| Mermaid 花括号 | `{text}` 被识别为菱形节点，需避免 |
| Mermaid par 语法 | `par...and...end` 飞书不支持 |
| 画板无 PATCH API；已有 DELETE | 修改节点用 create+delete，或 `board update --overwrite`（服务端 `overwrite: true` 原子清空并写入新节点） |
| 画板图片裁切 | API 不支持 `clip`/`mask`/`crop_rect`/`border_radius` 等属性，需预处理图片 |
| 画板图片 token | 每个节点必须独占 token，不可多节点复用同一 token |
