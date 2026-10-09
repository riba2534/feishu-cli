# 画板节点 API 详细参考

通过 `board create-notes` 命令或直接调用节点 API，在飞书画板上批量创建形状、文本、连接线等元素。

## API 概览

| 操作 | 端点 | 说明 |
|------|------|------|
| 创建节点 | POST `/open-apis/board/v1/whiteboards/{id}/nodes` | 批量创建，上限 3000 |
| 获取节点 | GET `/open-apis/board/v1/whiteboards/{id}/nodes` | 获取全部节点 |
| 删除节点 | DELETE `/open-apis/board/v1/whiteboards/{id}/nodes/{node_id}` | 单个删除 |
| 批量删除 | DELETE `/open-apis/board/v1/whiteboards/{id}/nodes/batch_delete` | 批量删除 |
| 修改节点 | -- | **无 PATCH**；用 create+delete，或 `board update --overwrite`（服务端 `overwrite: true` 原子覆盖） |

- 频率限制：50 req/s
- 请求体格式：`{"nodes": [...]}`

## CLI 命令

### board create-notes

批量创建节点（形状 + 连接线）。

```bash
# 从 JSON 文件（推荐，复杂图表）
feishu-cli board create-notes <whiteboard_id> nodes.json -o json

# 内联 JSON（简单场景）
feishu-cli board create-notes <whiteboard_id> '<json_array>' --source-type content -o json
```

返回：`{"count": 2, "node_ids": ["o1:1", "o1:2"], "whiteboard_id": "..."}`；带 `--client-token` 重放时返回首次创建的同一组 ID。

### board import

导入 Mermaid/PlantUML/SVG（服务端解析为 section 分组 + 原生节点；SVG 直接拆成原生节点）。

```bash
# Mermaid 内容导入
feishu-cli board import <whiteboard_id> "graph TD; A-->B-->C" \
  --source-type content --syntax mermaid --diagram-type flowchart

# Mermaid 文件导入
feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid

# PlantUML 文件导入
feishu-cli board import <whiteboard_id> diagram.puml --syntax plantuml

# 指定图表类型
feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid --diagram-type flowchart

# SVG 拆成原生节点
feishu-cli board import <whiteboard_id> drawing.svg --syntax svg -o json
```

### board nodes

获取画板所有节点（原样输出接口响应，节点在 `.data.nodes`；空画板为 `{"code":0,"data":{}}`）。

```bash
feishu-cli board nodes <whiteboard_id>
```

### board image

下载画板缩略图（服务端实际返回 JPEG，不保证 PNG；扩展名按实际格式自动补齐）。

```bash
feishu-cli board image <whiteboard_id> output   # 保存为 output.jpg 或 output.png，以实际格式为准
```

### doc add-board

在文档中添加空画板。

```bash
feishu-cli doc add-board <document_id> -o json
# 返回 whiteboard_id
```

## 节点类型详解

### composite_shape（形状）

最常用的节点类型。**最小格式**（推荐，多余字段导致 2890002 错误）：

```json
{
  "type": "composite_shape",
  "x": 100, "y": 100, "width": 200, "height": 50,
  "composite_shape": {"type": "round_rect"},
  "text": {
    "text": "节点文本",
    "font_size": 14,
    "font_weight": "regular",
    "horizontal_align": "center",
    "vertical_align": "mid"
  },
  "style": {
    "fill_color": "#611dc5",
    "fill_opacity": 100,
    "border_style": "solid",
    "border_color": "#1446C2",
    "border_width": "medium",
    "border_opacity": 100
  },
  "z_index": 10
}
```

**text 字段**：

| 字段 | 值 | 说明 |
|------|------|------|
| `font_size` | 12, 14, 16... | 字号 |
| `font_weight` | `regular`, `bold` | 字重 |
| `horizontal_align` | `left`, `center`, `right` | 水平对齐 |
| `vertical_align` | `top`, `mid`, `bottom` | 垂直对齐 |

**style 字段**：

| 字段 | 值 | 说明 |
|------|------|------|
| `fill_color` | `#rrggbb` | 填充颜色 |
| `fill_opacity` | 0-100 | 填充透明度 |
| `border_style` | `none`, `solid`, `dash`, `dot` | 边框样式 |
| `border_color` | `#rrggbb` | 边框颜色 |
| `border_width` | `narrow`, `medium`, `bold` | 边框宽度 |
| `border_opacity` | 0-100 | 边框透明度 |

### connector（连接线）

必须在形状节点创建后再创建（需引用节点 ID）。

```json
{
  "type": "connector",
  "width": 1, "height": 1,
  "z_index": 50,
  "connector": {
    "shape": "polyline",
    "start": {
      "arrow_style": "none",
      "attached_object": {
        "id": "<source_node_id>",
        "position": {"x": 1, "y": 0.5},
        "snap_to": "right"
      }
    },
    "end": {
      "arrow_style": "triangle_arrow",
      "attached_object": {
        "id": "<target_node_id>",
        "position": {"x": 0, "y": 0.5},
        "snap_to": "left"
      }
    }
  },
  "style": {
    "border_color": "#BBBFC4",
    "border_opacity": 100,
    "border_style": "solid",
    "border_width": "narrow"
  }
}
```

**connector 参数**：

| 字段 | 值 | 说明 |
|------|------|------|
| `shape` | `straight`, `polyline`, `curve`, `right_angled_polyline` | 连线形状 |
| `arrow_style` | `none`, `triangle_arrow` | 箭头样式 |
| `position` | `{"x": 0-1, "y": 0-1}` | 连接点位置（归一化坐标） |
| `snap_to` | `left`, `right`, `top`, `bottom` | 吸附方向 |

**注意**：GET 返回的 `start_object`/`end_object` 是只读字段，POST 时**不要**发送，使用 `start`/`end` 代替。

## z_index 与 fill_opacity（渲染层级）

z_index 分层规则与 fill_opacity 上限详见 **`schema.md` 的 "z_index 分层规则" 段**（避免本文档与 schema.md 重复）。

**两条关键规则提示**：

- 背景色块 `fill_opacity ≤ 60`（推荐 ≤ 25），否则完全遮挡上层元素
- whiteboard-cli 翻译 SVG 时**不输出 z_index 字段**，飞书 API 自动分配是无序的——通过 `scripts/svg_to_board.py` 5 步管道走 SVG 路径时已内置修复；手写节点 JSON 时务必显式指定 z_index（见 `pitfalls.md` 陷阱 1）

## 典型工作流

### 创建文档 + 画板 + 节点

```bash
# 步骤 1: 创建文档
feishu-cli doc create --title "架构图" -o json
# 返回 document_id

# 步骤 2: 添加画板
feishu-cli doc add-board <document_id> -o json
# 返回 whiteboard_id

# 步骤 3: 创建形状节点
feishu-cli board create-notes <whiteboard_id> shapes.json -o json
# 返回 node_ids

# 步骤 4: 创建连接线
feishu-cli board create-notes <whiteboard_id> connectors.json -o json

# 步骤 5: 截图验证（自动按实际格式补扩展名，通常得到 output.jpg）
feishu-cli board image <whiteboard_id> output
```

### 复制/修改画板

画板 API 不支持 PATCH：

- 整板复制用 `board clone <src> <dst>`：自动清洗只读字段、先建形状再建连线并重映射连线 ID（先 `--dry-run` 看节点数）
- 整板改写用 `board update <id> nodes.json --overwrite --snapshot old.json`（先把旧节点备份到本地再原子覆盖）
- 手工 redraw 时按下表清洗字段，新画板中先创建形状 → 映射旧 ID → 再创建连接线

### 需要清洗的字段（GET -> POST）

从 `board nodes` 获取的数据不能直接用于 `create-notes`，需移除以下字段：

| 层级 | 需移除的字段 | 原因 |
|------|------------|------|
| 顶层 | `id`, `locked`, `children`, `parent_id` | 只读/系统生成 |
| `text.*` | `text_color_type` | 未公开的内部字段 |
| `style.*` | `fill_color_type`, `border_color_type` | 未公开的内部字段 |
| `connector.*` | `start_object`, `end_object` | 只读，改用 `start`/`end` |

**composite_shape 必须保留完整子结构**：`composite_shape.type` + `text`（如有）。

**批量重建建议**：每批 10 个节点，间隔 3s，避免触发频率限制。

## Mermaid 导入参数

CLI 的 `--diagram-type` 取字符串（`auto` / `mindmap` / `sequence` / `activity` / `class` / `er` / `flowchart` / `state` / `component`），
默认 `auto` 按 Mermaid 首行声明识别即可。支持的声明：`flowchart TD`、`sequenceDiagram`、`classDiagram`、`stateDiagram-v2`、
`erDiagram`、`gantt`、`pie`、`mindmap`。

**Mermaid 注意**：
- 普通标签不要写字面花括号 `{text}`（会被识别为菱形节点）
- par、≥10 participant、多层 alt 早期会失败，2026-10 复测已可渲染；仍报 Parse error 时改 `--engine local`
- 时序图参与者建议 ≤ 8，主要是为了可读性
- `board import` 失败直接报错；降级为代码块只发生在 `doc import`

## 错误码

| 错误码 | 含义 | 常见原因 | 解决方案 |
|--------|------|---------|---------|
| 2890001 | invalid format | JSON 格式错误 | 检查 JSON 语法 |
| 2890002 | invalid arg | 包含未公开字段或格式不对 | 逐步删减字段定位问题，只用安全字段白名单 |
| 2890003 | record missing | whiteboard_id 不存在 | 确认 ID 来自 doc add-board 返回值 |
| 2890006 | rate limited | 超过 50 req/s | 降低请求频率，批量操作间隔 3s |
| 2890007 | whiteboard is not ready yet（HTTP 500） | `--overwrite` 刚执行完立即读画板（实测） | 等 1–2 秒重试 |

### 2890002 排障指引

JSON 中包含了 API 不支持的字段。只使用以下安全字段：

- `composite_shape` 节点：`type`, `x`, `y`, `width`, `height`, `composite_shape`, `text`, `style`, `z_index`
- `connector` 节点：`type`, `width`, `height`, `z_index`, `connector`, `style`
- `text` 对象：`text`, `font_size`, `font_weight`, `horizontal_align`, `vertical_align`
- `style` 对象：`fill_color`, `fill_opacity`, `border_style`, `border_color`, `border_width`, `border_opacity`

排查步骤：逐步删减 JSON 字段，定位导致错误的多余字段。常见陷阱包括 `id`、`locked`、`children`、`text_color_type` 等只读字段。
