# 画板节点 API 详细参考

通过 `board create-notes` 命令或直接调用节点 API，在飞书画板上批量创建形状、文本、连接线等元素。

## API 概览

| 操作 | 端点 | 说明 |
|------|------|------|
| 创建节点 | POST `/open-apis/board/v1/whiteboards/{id}/nodes` | 批量创建，上限 3000 |
| 获取节点 | GET `/open-apis/board/v1/whiteboards/{id}/nodes` | 获取全部节点 |
| 修改/删除 | — | **不支持**，需 redraw（重建画板） |

- 频率限制：50 req/s
- 请求体格式：`{"nodes": [...]}`

## 节点类型

### composite_shape（形状）

最常用的节点类型，支持矩形、圆角矩形等。

**最小格式**（推荐，多余字段可能导致 2890002 错误）：

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
    "border_style": "none"
  },
  "z_index": 10
}
```

**text 字段说明**：

| 字段 | 值 | 说明 |
|------|------|------|
| `font_size` | 12, 14, 16... | 字号 |
| `font_weight` | `regular`, `bold` | 字重 |
| `horizontal_align` | `left`, `center`, `right` | 水平对齐 |
| `vertical_align` | `top`, `mid`, `bottom` | 垂直对齐 |

**style 字段说明**：

| 字段 | 值 | 说明 |
|------|------|------|
| `fill_color` | `#rrggbb` | 填充颜色 |
| `fill_opacity` | 0-100 | 填充透明度 |
| `border_style` | `none`, `solid`, `dash`, `dot` | 边框样式 |
| `border_color` | `#rrggbb` | 边框颜色 |
| `border_width` | `narrow`, `medium`, `bold` | 边框宽度 |
| `border_opacity` | 0-100 | 边框透明度 |

### connector（连接线）

连接两个已创建的节点。**必须在形状节点创建后再创建连接线**（需要引用节点 ID）。

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
    "border_color": "#646a73",
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

正确的层级和透明度设置对画板渲染至关重要：

**关键规则**：背景色块 `fill_opacity` 必须 ≤60%，否则会完全遮挡上层元素。

| z_index 范围 | 用途 | fill_opacity |
|-------------|------|-------------|
| 0-1 | 外层容器、背景色带 | ≤50% |
| 2-3 | 次级色带、表头区域 | ≤60% |
| 4-8 | 列容器（dash border） | — |
| 9-16 | 文本标签（表头、图例） | — |
| 17+ | 内容卡片/条目 | 100%（实心） |
| 50 | 连接线 | border_opacity=100 |

## 典型工作流

### 1. 创建文档 + 画板 + 节点

```bash
# 步骤 1: 创建文档
feishu-cli doc create --title "架构图" -o json
# 返回 document_id

# 步骤 2: 添加画板
feishu-cli doc add-board <document_id> -o json
# 返回 whiteboard_id

# 步骤 3: 创建节点
feishu-cli board create-notes <whiteboard_id> nodes.json -o json
# 返回 node_ids

# 步骤 4: 截图验证
feishu-cli board image <whiteboard_id> output.png
```

### 2. 复制/修改画板（Redraw 模式）

画板 API 不支持 PATCH/DELETE，修改已有画板需要 redraw：

```bash
# 步骤 1: 导出原始节点
feishu-cli board nodes <original_whiteboard_id> > original.json

# 步骤 2: 清洗节点数据（移除只读字段）
# 需移除: id, locked, children, text.text_color_type,
#         style.fill_color_type, style.border_color_type

# 步骤 3: 分离形状和连接线
# 先创建形状 → 获取新 ID → 映射旧 ID → 再创建连接线

# 步骤 4: 创建新画板并写入
feishu-cli doc create --title "修改版" -o json
feishu-cli doc add-board <new_doc_id> -o json
feishu-cli board create-notes <new_whiteboard_id> cleaned_shapes.json -o json
feishu-cli board create-notes <new_whiteboard_id> remapped_connectors.json -o json
```

### 3. 需要清洗的字段（GET → POST）

从 `board nodes` 获取的数据不能直接用于 `create-notes`，需要移除以下字段：

| 层级 | 需移除的字段 | 原因 |
|------|------------|------|
| 顶层 | `id`, `locked`, `children`, `parent_id` | 只读/系统生成。**`parent_id` 必须移除**，否则 2890002 |
| `text.*` | `text_color_type` | 未公开的内部字段 |
| `style.*` | `fill_color_type`, `border_color_type` | 未公开的内部字段 |
| `connector.*` | `start_object`, `end_object` | 只读，改用 `start`/`end` |

### image（图片节点）

画板图片节点需要特殊处理，有多个关键注意事项：

**创建格式**（token 必须嵌套在 `image` 对象内）：

```json
{
  "type": "image",
  "x": 100, "y": 100, "width": 86, "height": 86,
  "z_index": 110,
  "image": {"token": "<file_token>"}
}
```

**图片上传**：画板图片必须使用 `parent_type=whiteboard`，`parent_node=画板ID`：

```bash
feishu-cli media upload image.png --parent-type whiteboard --parent-node <whiteboard_id> -o json
# 返回 {"file_token": "xxx"}
```

**⚠️ 关键规则**：

| 规则 | 说明 |
|------|------|
| **上传 parent_type** | 必须是 `whiteboard`，用 `docx_image` 上传的 token 在画板中无法渲染（显示棋盘格） |
| **上传 parent_node** | 必须是目标画板 ID（whiteboard_id），不是文档 ID |
| **token 嵌套格式** | 必须 `{"image":{"token":"xxx"}}`，不能放顶层 `{"token":"xxx"}`（顶层会被忽略） |
| **每个节点独占 token** | 同一张图片用于多个节点时，**必须分别上传获得不同 token**，不可复用 |
| **圆形裁切** | API 不支持 `clip`/`mask`/`border_radius`/`crop_rect` 等裁切属性。需**预处理图片**为圆形后上传 |

**圆形图片预处理**（Python PIL）：

```python
from PIL import Image, ImageDraw

img = Image.open("avatar.png").convert("RGBA")
mask = Image.new("L", img.size, 0)
ImageDraw.Draw(mask).ellipse((0, 0, img.size[0]-1, img.size[1]-1), fill=255)
output = Image.new("RGBA", img.size, (0, 0, 0, 0))
output.paste(img, (0, 0), mask)
output.save("avatar_circle.png")
```

**完整克隆图片流程**：

```bash
# 1. 从原始画板获取节点，提取 image 节点的 token 列表
feishu-cli board nodes <original_board_id> > original.json

# 2. 下载原始图片
feishu-cli media download <old_token> -o /tmp/images/<old_token>.png

# 3. 预处理为圆形（如果原图是圆形头像）

# 4. 逐个上传到新画板（每个节点一个独立 token）
feishu-cli media upload image.png --parent-type whiteboard --parent-node <new_board_id> -o json

# 5. 创建图片节点（注意限频，建议每个节点间隔 2s）
feishu-cli board create-notes <new_board_id> '[{"type":"image",...,"image":{"token":"<new_token>"}}]' --source-type content
```

## 批量创建的限频与重试

画板节点创建 API 有频率限制（实测约 10-20 req/5s），批量创建时需注意：

- **批量模式**：每批 10 个节点，批间间隔 3 秒
- **失败重试**：批量失败时改为逐个创建，每个间隔 2 秒
- **新建画板后**：需等待 5-10 秒画板初始化完成后再创建节点

## 错误码

| 错误码 | 含义 | 常见原因 |
|--------|------|---------|
| 2890001 | invalid format | JSON 格式错误 |
| 2890002 | invalid arg | 包含 `parent_id` 等未公开字段、连接线格式错误、节点缺少必需子结构（如 composite_shape 缺少 `composite_shape.type` 和 `text` 字段） |
| 2890003 | record missing | whiteboard_id 不存在 |
| 2890006 | rate limited | 超过频率限制 |

## 常用配色参考

| 用途 | 颜色 | 示例 |
|------|------|------|
| 紫色强调 | `#611dc5` | 分类标题 |
| 浅紫背景 | `#efecff` | 次级区域 |
| 绿色标注 | `#0c6800` | 流程节点 |
| 绿色域名 | `#e0f6dd` | 领域标签 |
| 浅蓝容器 | `#e6efff` | 嵌套分组 |
| 灰色背景 | `#f2f3f5` | 子条目 |
| 灰色边框 | `#bbbfc4` | 虚线容器 |
| 连接线灰 | `#646a73` | 连接线 |
| 蓝色成熟 | `#3370ff` | 状态-成熟 |
| 浅蓝进展 | `#e6efff` | 状态-进展中 |
| 橙色成熟 | `#ed6d0c` | 二级状态-成熟 |
| 浅橙进展 | `#ffeadf` | 二级状态-进展中 |
