# SVG → 飞书原生节点完整工作流

把一张 SVG 翻译成 N 个独立的飞书画板节点，让**每个矢量元素都可以单独点击编辑**。本文档解释为什么这是首选路径、5 步标准流程是什么、SVG 元素到飞书节点的映射规则，以及性能边界。

适用场景：飞轮 / 鱼骨 / 路线图 / Dashboard / 海报 / Mobile UI / 户型图 / 地铁图 / 插画 / 周期表 / 机芯 / 赛博朋克 / 任意 AI 设计图。

---

## 1. 为什么不用 `board svg-import` 单节点？

`board svg-import` 把整段 SVG 塞进一个 type=svg 节点（svg_code 字段保留完整 SVG 字符串），听起来很方便，但有两个致命限制：

### 限制 1: 飞书对单 svg 节点的渲染复杂度有上限

实测：

- 一份 150 KB 的赛博朋克 SVG（含上千 `<rect>` 模拟窗户）落为单 svg 节点 → **只渲染了左边 3-4 栋楼**，右半区大片空白
- 一份 63 KB 的周期表 SVG（118 元素 × 4 信息项 = 472 + 矩形）落为单 svg 节点 → **缺失大量元素**

飞书内部把 svg_code 渲染成栅格图存到画板上，对单节点的元素数量 / 体积有内部上限。**复杂场景下必须拆**。

### 限制 2: 整图作为一个矢量贴图，不可拆分编辑

用户最直接的反馈：「大图里的小元素都是可以点击的，而这个看起来画板里面只有一个整体的元素」。

- 单 svg 节点：选中只能整张选，不能改单个元素的颜色 / 文字
- 原生节点拆分：每个 `<rect>` / `<circle>` / `<text>` 都是独立飞书节点，可单独选中、改色、移动、删除

### 何时仍用 `board svg-import`？

只有 3 种场景适合单节点：

1. **简单图标 / 印章 / 小装饰**（< 2 KB SVG）
2. **不要求编辑的展示元素**
3. **viewBox 极小（< 200×200）的小元素**

其他情况一律用 5 步管道。

---

## 2. 5 步标准工作流

### Step 1: 生成 SVG

#### 三种方式

| 方式 | 适合场景 | 工具 |
|------|---------|------|
| 手写 | 简单图（< 100 元素） | 任意文本编辑器 |
| Python 程序化 | 重复结构（周期表 / 赛博朋克城市 / 路线图） | Python `math` + `xml.etree` |
| AI 生成 | 创意设计（飞轮 / 鱼骨 / 插画） | Claude 直接吐 SVG 代码 |

#### 设计原则

- **viewBox 设大点**：常用 1400×900 或 1600×900，避免局促
- **推荐元素**：`<rect>` / `<circle>` / `<ellipse>` / `<path>` / `<polygon>` / `<polyline>` / `<text>` / `<line>` / `<g>`
- **避免**：
  - `<foreignObject>`（飞书不支持 HTML 嵌入）
  - 外链 `<image href="https://...">`（改用 `board upload-image`）
  - 复杂动画 `<animate>` `<animateTransform>`（飞书静态画板不支持）
  - **用曲线 `<path>`（贝塞尔 `Q`/`C`、弧 `A`）画连接线 / 自环 / 弧线箭头**——whiteboard-cli 转节点时曲率会丢失，退化成直线甚至悬空在节点旁（实测：状态机 `healthy` 自环用 `M..Q..` 画，渲染成一条飘在节点上方、不连回节点的孤立直线）。**画自环 / 弧线 / 曲线连接一律用多段 `<line>` 或 `<polyline>` 折线拼**（直线 → connector，渲染稳定，箭头三角用小 `<polygon>`）。纯装饰性的曲线填充图形（如插画里的山脊、花瓣，非连接语义）可保留，会转 svg 节点。

#### 极坐标 / 三角函数布局（飞轮 / 雷达图）

```python
import math
N = 4
cx, cy, R = 500, 500, 300
for i in range(N):
    θ = -math.pi/2 + i * 2*math.pi/N    # 12 点位置开始顺时针
    x = cx + R * math.cos(θ)
    y = cy + R * math.sin(θ)
    # 在 (x, y) 放标签 / 卡片
```

### Step 2: SVG → 节点 JSON 翻译

```bash
whiteboard-cli -i drawing.svg -f svg -t openapi -o nodes.json
```

参数：
- `-i drawing.svg`：输入 SVG 文件
- `-f svg`：强制识别为 SVG（避免 .json/.mmd 误识别）
- `-t openapi`：输出飞书 OpenAPI 节点 JSON 格式（不是 PNG）
- `-o nodes.json`：输出路径

输出结构：

```json
{
  "nodes": [
    {"type": "composite_shape", "x": 0, "y": 0, "width": 1600, "height": 900, "composite_shape": {"type": "rect"}, "style": {...}},
    {"type": "text_shape", "x": 100, "y": 50, "width": 200, "height": 30, "text": {"text": "标题", "font_size": 24, ...}},
    ...
  ]
}
```

### Step 3: 修复 z_index ⭐⭐⭐ 关键

**为什么必须做**：whiteboard-cli 输出节点不带 z_index，飞书 API 自动分配是无序的（详见 `pitfalls.md` 陷阱 1）。

```python
import json
data = json.load(open("nodes.json"))
nodes = data["nodes"]
for i, node in enumerate(nodes):
    node["z_index"] = i        # 0..N-1，前小后大（画家算法）
json.dump(data, open("nodes.json", "w"))
```

不做这一步：背景大矩形可能被分到 z=1684，反而盖住前景的所有小节点，渲染一塌糊涂。

### Step 4: 修剪 viewBox 溢出 ⭐⭐ 关键

**为什么必须做**：超出 viewBox 的节点会显示为"半截楼"诡异图形（详见 `pitfalls.md` 陷阱 2）。

```python
def trim_overflow(nodes, vw, vh):
    kept = []
    for n in nodes:
        x = float(n.get("x", 0))
        y = float(n.get("y", 0))
        w = float(n.get("width", 0))
        h = float(n.get("height", 0))
        if x >= vw or y >= vh or (x + w) <= 0 or (y + h) <= 0:
            continue
        if (x + w > vw or y + h > vh) and n.get("type") == "svg":
            continue   # svg 节点截断会扭曲渲染，直接删
        if x + w > vw:
            n["width"] = vw - x
        if y + h > vh:
            n["height"] = vh - y
        kept.append(n)
    return kept
```

### Step 5: 分批 create-notes 上传

**为什么分批**：单次 POST 节点过多会触发 API 限制（实测 > 500 节点失败率上升），且容易 HTTP body 过大。

**为什么节流**：连续 POST 会触发飞书速率限制（HTTP 429）。

```bash
# 每批 300 节点 + 间隔 0.3s 是实测稳定值
feishu-cli board create-notes <board_id> nodes_batch_0.json
sleep 0.3
feishu-cli board create-notes <board_id> nodes_batch_1.json
sleep 0.3
...
```

注意：rc=0 一律按成功，**不要因 stdout 解析失败而重试**（详见 `pitfalls.md` 陷阱 3）。

---

## 3. 一键脚本：`scripts/svg_to_board.py`

5 步全自动管道。强烈推荐用这个而不是手写：

```bash
# 基础用法（自动从 SVG 解析 viewBox）
python3 scripts/svg_to_board.py drawing.svg <board_id>

# 自定义 viewBox（如果 SVG 没标 viewBox 属性）
python3 scripts/svg_to_board.py drawing.svg <board_id> --viewbox 1600x900

# 自定义批次和节流
python3 scripts/svg_to_board.py drawing.svg <board_id> --batch 200 --interval 0.5

# dry-run：跑 Step 1-3 但不上传（调试用）
python3 scripts/svg_to_board.py drawing.svg <board_id> --dry-run

# 不裁剪溢出节点（不建议，仅用于调试）
python3 scripts/svg_to_board.py drawing.svg <board_id> --keep-overflow
```

退出码：
- `0` 全部成功
- `1` whiteboard-cli 或 feishu-cli 不可用
- `2` SVG 解析失败 / viewBox 无法识别
- `3` 部分批次上传失败

脚本输出示例：

```
  viewBox = 1600.0x900.0

=== Step 1: whiteboard-cli 翻译 SVG → 节点 JSON ===
  翻译成功：1984 个节点
  类型分布：{'composite_shape': 1919, 'connector': 48, 'svg': 9, 'text_shape': 8}

=== Step 2: 修 z_index（画家算法） ===
  已为 1984 个节点显式赋 z_index = 0..1983

=== Step 3: 修剪 viewBox 溢出（1600.0x900.0） ===
  保留 1977，删除 7 个完全溢出节点，截断 0 个边缘节点

=== Step 4: 分批上传（batch=300 interval=0.3s） ===
  ✓ 批 0-300 上传 300
  ✓ 批 300-600 上传 300
  ...

=== Step 5: 验证 ===
  画板节点数：1977（期望 ≈ 1977）
  类型分布：{...}

========== 完成（87.3s）==========
```

---

## 4. SVG 元素 → 飞书节点翻译映射

whiteboard-cli 的翻译规则（实测整理）：

| SVG 元素 | 飞书节点 type | 子类型 / 关键字段 | 备注 |
|----------|--------------|------------------|------|
| `<rect rx=0>` | `composite_shape` | `composite_shape.type: rect` |  |
| `<rect rx>0>` | `composite_shape` | `composite_shape.type: round_rect` | 圆角自动识别 |
| `<circle>` | `composite_shape` | `composite_shape.type: ellipse` | 内部按 ellipse 统一 |
| `<ellipse>` | `composite_shape` | `composite_shape.type: ellipse` |  |
| `<text>` | `text_shape` | `text.text / font_size / font_weight / text_color / horizontal_align` | text-anchor 映射 |
| `<line>` | `connector` | `connector.start.position / end.position` | 直线 |
| `<path d="M ... L ... L ...">` 简单 | `connector` 或 `svg` | 视情况 | 折线可能转 connector |
| `<path>` 复杂 | `svg` | `svg.svg_code` 保留 path 标签 | 含弧线 / 曲线 |
| `<polygon>` 三角形 | `composite_shape` | `composite_shape.type: triangle` | 识别为三角形 |
| `<polygon>` 不规则 | `svg` | `svg.svg_code` 保留 polygon | 多边形 |
| `<polyline>` | `connector` 或 `svg` | 视点数 | |
| `<g>` | `group` | 嵌套节点保留为 children | |
| `<defs>` / `<linearGradient>` | 不直接转节点 | 但被 `<path>` / `<rect>` 内嵌引用时随 svg 节点保留 | 渐变只在 svg 节点内有效 |

### 翻译边界情况

- `transform="rotate(...)"`：会展开到节点的 `angle` 字段（仅 `<text>` 和 `<g>` 支持）
- `transform="translate(...)"`：会被内联到 x/y 中
- `transform="scale(...)"`：可能丢失，不建议用
- `opacity`：转为 `style.fill_opacity` / `style.border_opacity`
- `font-family`：飞书强制用自家字体（Noto / 苹方），不保留自定义字体
- `<use>`：不支持（whiteboard-cli 可能展开，可能丢失）
- 曲线 `<path>`（`Q`/`C` 贝塞尔、`A` 弧）：作**连接线 / 自环**用时曲率易丢、退化为直线或悬空 → 改用多段 `<line>` / `<polyline>` 折线；作**填充形状**用时转 svg 节点尚可保留

---

## 5. 节点密度参考（14 张实战图）

| 图 | viewBox | 节点数 | 主要类型分布 | 上传耗时 | 编辑器流畅度 |
|----|---------|--------|-------------|---------|-------------|
| 飞轮 | 1000×1000 | 30 | text:24 / svg:4 / shape:2 | <2s | 流畅 |
| 金字塔 | 1000×700 | 28 | text:21 / svg:5 / connector:1 / shape:1 | <2s | 流畅 |
| 鱼骨 | 1400×700 | 67 | text:30 / connector:28 / shape:8 / svg:1 | 1-3s | 流畅 |
| Mobile UI | 430×900 | 67 | text:41 / shape:24 / connector:2 | 1-3s | 流畅 |
| 桑基 | 1400×820 | 85 | svg:35 / text:33 / shape:17 | 2-5s | 流畅 |
| 路线图 | 1600×700 | 93 | text:41 / shape:37 / connector:14 / svg:1 | 2-5s | 流畅 |
| 插画 | 1400×800 | 101 | connector:45 / shape:40 / svg:15 / text:1 | 2-5s | 流畅 |
| 地铁 | 1400×900 | 117 | text:60 / shape:47 / connector:9 / svg:1 | 3-6s | 流畅 |
| 平面图 | 1200×800 | 119 | shape:44 / text:52 / connector:17 / svg:6 | 3-6s | 流畅 |
| 海报 | 900×1300 | 124 | shape:79 / text:43 / svg:2 | 3-6s | 流畅 |
| Dashboard | 1400×900 | 171 | text:94 / shape:58 / connector:15 / svg:4 | 5-10s | 流畅 |
| 机芯 | 1200×1200 | 316 | shape:259 / connector:43 / text:13 / svg:1 | 10-20s | 流畅 |
| 周期表 | 1900×1100 | 647 | text:516 / shape:131 | 30-60s | 流畅 |
| 赛博朋克 | 1600×900 | 1984 | shape:1919 / connector:48 / text:16 / svg:8 | 60-90s | 略卡 |

实战边界：

- **≤ 200 节点**：所有操作流畅
- **200-500 节点**：流畅，上传 ~10-20s
- **500-1000 节点**：编辑器仍流畅，上传 ~30-60s
- **1000-2000 节点**：编辑器开始略卡，上传 ~60-120s（赛博朋克级）
- **> 2000 节点**：编辑器明显卡顿，建议拆图或简化

---

## 6. 何时该拆图？

如果单张图节点数 > 1500-2000，考虑：

### 方案 A：简化 SVG

- 合并冗余装饰元素（80 颗星星 → 20 颗）
- 删除过密的小色块（地面反光的 40 个反光小条 → 渐变 rect）
- 用更少的 `<rect>` 模拟密集窗户（每栋楼 100 窗 → 30 窗，颜色对比拉满弥补密度损失）

### 方案 B：分多张画板

把视觉上可独立的区域拆开：
- 一张「全景图」（远景剪影 + 月亮 + 星空）
- 一张「中景特写」（中景建筑 + 霓虹招牌）
- 一张「近景细节」（前排建筑 + 飞行器）

在文档里用 3 个 `<whiteboard>` 块串联。

### 方案 C：用矢量贴图 + 局部互动

主背景用一张图片（PNG / `board upload-image` 上传），关键互动元素用独立节点叠加。性能最好但失去全局可编辑性。

---

## 7. 验证清单

落板后逐项核对：

- [ ] **节点数对**：`board nodes <id> | jq '.data.nodes | length'` 接近 nodes.json
- [ ] **z_index 最小是大背景**：参考 `pitfalls.md` Step 2
- [ ] **无 viewBox 溢出**：`max(x+w) ≤ viewBox_w`
- [ ] **缩略图主要元素都在**：`board image <id> /tmp/check.png` 后看
- [ ] **lint 质量分 ≥ 0.85**：`board lint <id>`

如其中任何一项不通过，回到 `pitfalls.md` 排障。

---

## 8. 一句话总结

| 需求 | 路径 |
|------|------|
| 让大图里的小元素可点击编辑 | 用 5 步管道 + `scripts/svg_to_board.py` |
| 简单图标 / 印章 | `board svg-import`（单节点） |
| 200-2000 节点的复杂图 | 5 步管道，没问题 |
| > 2000 节点 | 先考虑简化或拆图 |
