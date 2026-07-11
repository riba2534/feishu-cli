# 连线系统

## 连线策略

| 连线数 | 策略 |
|--------|------|
| <= 8 | 逐条画 |
| 9-15 | 选代表性连线（每层选 1-2 个节点连到下一层） |
| > 15 | 精简分组，层到层连线，或回退减少节点数 |

一个节点有 3+ 条连线时：入线从 top，出线从 bottom，同侧多条线用不同 position 分散。

---

## 连接方向速查

| 方向 | start position | start snap_to | end position | end snap_to |
|------|---------------|---------------|-------------|-------------|
| -> 左到右 | `{x:1, y:0.5}` | `right` | `{x:0, y:0.5}` | `left` |
| v 上到下 | `{x:0.5, y:1}` | `bottom` | `{x:0.5, y:0}` | `top` |
| <- 右到左 | `{x:0, y:0.5}` | `left` | `{x:1, y:0.5}` | `right` |
| ^ 下到上 | `{x:0.5, y:0}` | `top` | `{x:0.5, y:1}` | `bottom` |

position 是归一化坐标（0-1），表示节点边缘上的连接点位置。

---

## 扇出/扇入

一个节点连多条线时，调整 position 避免连线重叠：

**扇出（1 对 3，从 bottom 出发）**：

```
连线1: start position {x:0.25, y:1} snap_to=bottom → 左侧子节点 top
连线2: start position {x:0.5,  y:1} snap_to=bottom → 中间子节点 top
连线3: start position {x:0.75, y:1} snap_to=bottom → 右侧子节点 top
```

**扇入（3 对 1，汇聚到 top）**：

```
连线1: 左侧节点 bottom → end position {x:0.25, y:0} snap_to=top
连线2: 中间节点 bottom → end position {x:0.5,  y:0} snap_to=top
连线3: 右侧节点 bottom → end position {x:0.75, y:0} snap_to=top
```

---

## shape 选择

| shape | 效果 | 适用场景 |
|-------|------|---------|
| `polyline` | 圆角折线（默认首选） | 流程图、架构图、大多数场景 |
| `straight` | 直线 | 坐标轴、数轴、几何图形边框 |
| `right_angled_polyline` | 直角折线 | 组织架构树、总线规约 |
| `curve` | 曲线 | 优雅的跨层连线、脑图分支、注解箭头 |

---

## 箭头样式

| arrow_style | 效果 |
|-------------|------|
| `none` | 无箭头 |
| `triangle_arrow` | 三角箭头 |

常见组合：
- 单向流：`start.arrow_style = "none"`, `end.arrow_style = "triangle_arrow"`
- 双向流：两端都用 `"triangle_arrow"`
- 无方向关联：两端都用 `"none"`

---

## 连线样式

连线的 style 通过 border 属性控制：

```json
"style": {
  "border_color": "#BBBFC4",
  "border_opacity": 100,
  "border_style": "solid",
  "border_width": "narrow"
}
```

| 样式 | border_style | 用途 |
|------|-------------|------|
| 实线 | `solid` | 主流程、强关系 |
| 虚线 | `dash` | 可选路径、弱关系、回调 |
| 点线 | `dot` | 注解、说明性连线 |

**连线颜色规则**：统一使用主题定义的连线灰（默认主题 `#BBBFC4`，科技暗色主题 `#4A4A4A`，见 style.md 主题变体表），不要和节点颜色混用。

---

## 间距要求

有连线的节点间距 >= 60px，否则箭头挤在缝里看不清。

```
错误: 节点 A (x=100, w=160) → 节点 B (x=280, w=160)  间距=20px
正确: 节点 A (x=100, w=160) → 节点 B (x=320, w=160)  间距=60px
```
