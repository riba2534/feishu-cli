# 节点 Schema

feishu-cli 使用飞书 OpenAPI 原生 JSON 格式，所有节点通过绝对坐标定位。

## 目录

- [节点类型总览](#节点类型总览)
- [形状节点](#composite_shape形状节点)
- [连接线](#connector连接线)
- [SVG 节点](#svg矢量节点)
- [文本和背景](#纯文本节点)
- [尺寸和位置](#尺寸和位置最佳实践)
- [z_index](#z_index-分层规则)
- [最小示例](#最小示例2-个节点--1-条连线)

## 节点类型总览

| 类型 | type 值 | composite_shape.type | 用途 |
|------|---------|---------------------|------|
| 矩形 | `composite_shape` | `rect` | 通用节点 |
| 圆角矩形 | `composite_shape` | `round_rect` | 常用节点（推荐默认） |
| 椭圆 | `composite_shape` | `ellipse` | 起止节点、特殊标记 |
| 菱形 | `composite_shape` | `diamond` | 判断/决策节点 |
| 三角形 | `composite_shape` | `triangle` | 方向指示 |
| 圆柱体 | `composite_shape` | `cylinder` | 数据库/存储 |
| 平行四边形 | `composite_shape` | `parallelogram` | 输入/输出 |
| 纯文本 | `composite_shape` | 任意 + 无边框无填充 | 标签/标题 |
| 连接线 | `connector` | -- | 节点间关系 |
| 文本节点 | `text_shape` | -- | 独立文字（推荐用于纯标签） |
| **矢量图形** | **`svg`** | **--** | **任意 SVG 代码作为可编辑矢量节点（飞轮/鱼骨/路线图等 AI 自由作图首选）** |

---

## composite_shape（形状节点）

### 完整属性

```json
{
  "type": "composite_shape",
  "x": 100,
  "y": 100,
  "width": 200,
  "height": 50,
  "z_index": 10,
  "composite_shape": {
    "type": "round_rect"
  },
  "text": {
    "text": "节点文本",
    "font_size": 14,
    "font_weight": "regular",
    "horizontal_align": "center",
    "vertical_align": "mid"
  },
  "style": {
    "fill_color": "#FFFFFF",
    "fill_opacity": 100,
    "border_style": "solid",
    "border_color": "#1446C2",
    "border_width": "medium",
    "border_opacity": 100
  }
}
```

### 安全字段白名单

**只使用以下字段**，多余字段会导致 `2890002 invalid arg` 错误：

| 层级 | 允许的字段 |
|------|-----------|
| 顶层 | `type`, `x`, `y`, `width`, `height`, `z_index`, `composite_shape`, `text`, `style` |
| `composite_shape` | `type` |
| `text` | `text`, `font_size`, `font_weight`, `horizontal_align`, `vertical_align` |
| `style` | `fill_color`, `fill_opacity`, `border_style`, `border_color`, `border_width`, `border_opacity` |

### text 字段值

| 字段 | 可选值 | 说明 |
|------|--------|------|
| `font_size` | 12, 13, 14, 15, 16, 18, 20, 24, 28 | 字号 |
| `font_weight` | `regular`, `bold` | 字重 |
| `horizontal_align` | `left`, `center`, `right` | 水平对齐 |
| `vertical_align` | `top`, `mid`, `bottom` | 垂直对齐 |

### style 字段值

| 字段 | 可选值 | 说明 |
|------|--------|------|
| `fill_color` | `#rrggbb` | 填充颜色 |
| `fill_opacity` | 0-100 | 填充透明度（背景色块 <= 25，常规节点 100） |
| `border_style` | `none`, `solid`, `dash`, `dot` | 边框样式 |
| `border_color` | `#rrggbb` | 边框颜色 |
| `border_width` | `narrow`, `medium`, `bold` | 边框宽度（推荐 `medium`） |
| `border_opacity` | 0-100 | 边框透明度 |

---

## connector（连接线）

必须在形状节点创建后再创建，通过 ID 引用形状。

### 完整属性

```json
{
  "type": "connector",
  "width": 1,
  "height": 1,
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

### connector 安全字段白名单

| 层级 | 允许的字段 |
|------|-----------|
| 顶层 | `type`, `width`, `height`, `z_index`, `connector`, `style` |
| `connector` | `shape`, `start`, `end` |
| `start` / `end` | `arrow_style`, `attached_object` |
| `attached_object` | `id`, `position`, `snap_to` |
| `position` | `x`, `y`（归一化 0-1） |
| `style` | `border_color`, `border_opacity`, `border_style`, `border_width` |

### connector 参数说明

| 字段 | 值 | 说明 |
|------|------|------|
| `shape` | `straight`, `polyline`, `curve`, `right_angled_polyline` | 连线形状 |
| `arrow_style` | `none`, `triangle_arrow` | 箭头样式 |
| `position` | `{"x": 0-1, "y": 0-1}` | 节点边缘上的连接点（归一化坐标） |
| `snap_to` | `left`, `right`, `top`, `bottom` | 吸附方向 |

---

## svg（矢量节点）

整段 SVG 作为单个画板节点，飞书画板解析器会把内部元素渲染为可编辑矢量图形。**适合 AI 自由作图**（飞轮 / 鱼骨 / 路线图 / 仪表盘 / 户型图 / 海报 / 信息周期表等），完全绕开 Mermaid/PlantUML 的语法限制。

### 完整属性

```json
{
  "type": "svg",
  "x": 100,
  "y": 100,
  "width": 245,
  "height": 245,
  "angle": 0,
  "z_index": 10,
  "svg": {
    "key": "",
    "svg_code": "<svg viewBox=\"0 0 245 245\" xmlns=\"http://www.w3.org/2000/svg\"><path d=\"M 120 0 A 120 120 0 0 1 240 120 L 180 120 A 60 60 0 0 0 120 60 Z\" fill=\"#611DC5\" opacity=\"0.85\"/></svg>",
    "type": 0
  },
  "style": {
    "border_color": "#4e83fd",
    "border_color_type": 0,
    "border_opacity": 100,
    "border_style": "none",
    "border_width": "narrow",
    "fill_color_type": 0,
    "fill_opacity": 100,
    "theme_border_color_code": -1,
    "theme_fill_color_code": -1
  }
}
```

### 安全字段白名单

| 层级 | 允许的字段 |
|------|-----------|
| 顶层 | `type`, `x`, `y`, `width`, `height`, `angle`, `z_index`, `svg`, `style` |
| `svg` | `key`（空字符串占位）, `svg_code`（必填）, `type`（默认 0） |
| `style` | 同 composite_shape，但 `fill_color_type=0` + `fill_opacity=100` 表示走 SVG 自带填色 |

### 推荐 SVG 元素

| 元素 | 用途 | 支持度 |
|------|------|--------|
| `<rect>` | 矩形、卡片、背景 | ✓ |
| `<circle>` / `<ellipse>` | 圆环、节点 | ✓ |
| `<path>` | 任意路径（扇形 / 曲线 / 折线） | ✓ |
| `<polygon>` / `<polyline>` | 多边形、连线 | ✓ |
| `<text>` | 文字（可设 `font-size` `fill` `text-anchor`） | ✓ |
| `<line>` | 直线 | ✓ |
| `<g>` 嵌套 | 分组 | ✓ |
| `<foreignObject>` | HTML 嵌入 | ✗（不推荐） |
| `<image href>` | 外链图片 | ⚠ 需画板内已存的 token，建议改用 `board upload-image` |

### 极坐标布局（AI 自由作图常用）

均匀分布 N 个节点在圆周上：

```
for i in 0..N:
    θ = 2π * i / N
    x = cx + r * cos(θ)
    y = cy + r * sin(θ)
```

### 节点密度建议

| 节点数 | 状态 |
|--------|------|
| ≤ 100 | 流畅 |
| 100-300 | 正常 |
| 300-500 | 编辑略卡 |
| 500-600 | 可承受上限（参考元素周期表 659 节点实测） |
| > 700 | 不推荐 |

### 一键导入

```bash
# CLI 命令（自动解析 viewBox 推断宽高）
feishu-cli board svg-import <whiteboard_id> drawing.svg --x 100 --y 100

# 字符串直传
feishu-cli board svg-import <whiteboard_id> '<svg viewBox="0 0 200 200">...</svg>' --source-type content

# 预览不发请求
feishu-cli board svg-import <whiteboard_id> drawing.svg --dry-run
```

---

## 纯文本节点

用 composite_shape 模拟，设置无边框无填充：

```json
{
  "type": "composite_shape",
  "x": 100, "y": 50, "width": 300, "height": 40,
  "z_index": 10,
  "composite_shape": {"type": "round_rect"},
  "text": {
    "text": "图表标题",
    "font_size": 24,
    "font_weight": "bold",
    "horizontal_align": "center",
    "vertical_align": "mid"
  },
  "style": {
    "fill_opacity": 0,
    "border_style": "none"
  }
}
```

---

## 背景分区节点

用低透明度大矩形作为视觉分区，z_index 设为 0-1：

```json
{
  "type": "composite_shape",
  "x": 50, "y": 80, "width": 700, "height": 200,
  "z_index": 0,
  "composite_shape": {"type": "round_rect"},
  "text": {
    "text": "服务层",
    "font_size": 13,
    "font_weight": "regular",
    "horizontal_align": "left",
    "vertical_align": "top"
  },
  "style": {
    "fill_color": "#E6EFFF",
    "fill_opacity": 25,
    "border_style": "solid",
    "border_color": "#1446C2",
    "border_width": "narrow",
    "border_opacity": 40
  }
}
```

---

## 尺寸和位置最佳实践

feishu-cli 使用绝对坐标，没有自动布局引擎。所有节点必须手算 x/y/width/height。

### 尺寸参考

| 节点类型 | 推荐 width | 推荐 height | 说明 |
|----------|-----------|------------|------|
| 常规节点（短文本） | 120-200 | 40-50 | 1-2 行文字 |
| 卡片节点（标题+描述） | 160-240 | 60-80 | 2-3 行文字 |
| 数据库圆柱体 | 120-160 | 50-60 | cylinder 弧度固定 |
| 菱形判断 | 120-160 | 80-100 | 菱形需要更大空间放文字 |
| 纯文本标题 | 200-400 | 30-40 | H1 标题 |
| 背景分区 | 按内容区域 | 按内容区域 | 包裹所有子节点 + 内边距 |

### 位置计算

节点位置通过 `x`（左上角 x）和 `y`（左上角 y）确定。计算方法：

```
节点中心 x = x + width / 2
节点中心 y = y + height / 2
下一行 y = 当前行 y + 当前行最大 height + 行间距
同行下一个 x = 当前 x + 当前 width + 列间距
```

### 背景分区位置计算

背景分区需要包裹所有子节点，加上内边距：

```
分区 x = min(子节点 x) - padding
分区 y = min(子节点 y) - padding
分区 width = max(子节点 x + width) - min(子节点 x) + 2 * padding
分区 height = max(子节点 y + height) - min(子节点 y) + 2 * padding
```

推荐 padding: 20-30px。

---

## z_index 分层规则

| z_index | 用途 | fill_opacity |
|---------|------|-------------|
| 0-1 | 背景色带（分区底色） | <= 25 |
| 2-3 | 次级区域（表头区域） | <= 60 |
| 4-8 | 列容器（dash border） | -- |
| 9-16 | 文本标签（标题、图例） | -- |
| 10 | 常规形状节点 | 100（实心） |
| 50 | 连接线 | border_opacity=100 |

**关键规则**：背景色块 fill_opacity 必须 <= 60，推荐 <= 25，否则遮挡上层节点。

---

## 最小示例：2 个节点 + 1 条连线

```bash
# shapes.json
cat > /tmp/shapes.json << 'EOF'
[
  {"type":"composite_shape","x":100,"y":100,"width":160,"height":40,
   "composite_shape":{"type":"round_rect"},
   "text":{"text":"服务 A","font_size":14,"font_weight":"regular","horizontal_align":"center","vertical_align":"mid"},
   "style":{"fill_color":"#FFFFFF","fill_opacity":100,"border_style":"solid","border_color":"#1446C2","border_width":"medium","border_opacity":100},
   "z_index":10},
  {"type":"composite_shape","x":400,"y":100,"width":160,"height":40,
   "composite_shape":{"type":"round_rect"},
   "text":{"text":"服务 B","font_size":14,"font_weight":"regular","horizontal_align":"center","vertical_align":"mid"},
   "style":{"fill_color":"#FFFFFF","fill_opacity":100,"border_style":"solid","border_color":"#0C6800","border_width":"medium","border_opacity":100},
   "z_index":10}
]
EOF
feishu-cli board create-notes $BOARD_ID /tmp/shapes.json -o json
# → 返回 node_ids: ["o1:1", "o1:2"]

# connector.json
cat > /tmp/connector.json << 'EOF'
[
  {"type":"connector","width":1,"height":1,"z_index":50,
   "connector":{"shape":"polyline",
     "start":{"arrow_style":"none","attached_object":{"id":"o1:1","position":{"x":1,"y":0.5},"snap_to":"right"}},
     "end":{"arrow_style":"triangle_arrow","attached_object":{"id":"o1:2","position":{"x":0,"y":0.5},"snap_to":"left"}}},
   "style":{"border_color":"#BBBFC4","border_opacity":100,"border_style":"solid","border_width":"narrow"}}
]
EOF
feishu-cli board create-notes $BOARD_ID /tmp/connector.json -o json
```
