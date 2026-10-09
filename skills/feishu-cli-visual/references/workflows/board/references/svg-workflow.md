# SVG → 飞书原生节点完整工作流

把一张 SVG 翻译成 N 个独立的飞书画板节点，让**每个矢量元素都可以单独点击编辑**。本文档说明服务端解析与本地管道怎么选、
本地 5 步管道做了什么、SVG 元素到飞书节点的映射规则，以及性能边界。

适用场景：飞轮 / 鱼骨 / 路线图 / Dashboard / 海报 / Mobile UI / 户型图 / 地铁图 / 插画 / 周期表 / 机芯 / 赛博朋克 / 任意 AI 设计图。

---

## 目录

- [服务端解析 vs 本地管道](#0-服务端-svg-解析-vs-本地管道ab-实测)
- [为什么不用单节点](#1-为什么不用-board-svg-import-单节点)
- [标准工作流](#2-标准工作流生成-svg--本地-5-步管道)
- [一键脚本](#3-一键脚本scriptssvg_to_boardpy)
- [翻译映射](#4-svg-元素--飞书节点翻译映射)
- [节点密度](#5-节点密度参考14-张实战图)
- [拆图判断](#6-何时该拆图)
- [验证清单](#7-验证清单)

## 0. 服务端 SVG 解析 vs 本地管道（A/B 实测）

拆成原生节点有两条路：

- **服务端解析**：`feishu-cli board import <board_id> drawing.svg --syntax svg`（`syntax_type=3`，一条命令）
- **本地管道**：`scripts/svg_to_board.py`（whiteboard-cli 翻译 → 修 z_index → 裁剪溢出 → 分批 create-notes）

同一批 SVG 在测试画板上的实测对比（2026-10，whiteboard-cli 0.2.13；两条路径的节点类型、坐标几乎一致，
服务端与 whiteboard-cli 走的是同一套翻译规则）：

| 维度 | 服务端解析 `--syntax svg` | 本地管道 svg_to_board.py |
|------|------------------------|-------------------------|
| rect / circle / text / line / group | round_rect、ellipse、text_shape、straight connector、group + 子节点 | 同左 |
| 曲线 path（`Q` 自环、`C` 三次贝塞尔） | curve connector，渲染与原图一致；三次贝塞尔的曲率更贴近原图 | curve connector，渲染正常，三次贝塞尔略平缓 |
| 弧线 `A`、不规则/三角 polygon | image 节点（服务端栅格化） | svg 节点 |
| 折线 polyline、`M L L` path | connector（`right_angled_polyline`），按原折点渲染 | 同左 |
| z_index | 按 SVG 顺序自动赋值 | Step 2 显式赋 `z_index = 序号` |
| 渐变 `fill=url(#id)`、自定义 `stroke-dasharray` | 渐变取首个色、虚线改为 `dash`，`-o json` 输出 `degraded_attributes` 逐条列出 | 不报告降级；渐变写入 `style.fill_gradient`，缩略图同样显示为单色 |
| viewBox 外的元素 | **原样保留**（画布外会多出一块） | Step 3 自动删除/截断 |
| 1500 个 rect（93 KB） | 1 次请求约 3 秒，1501 节点 | 未测（需分批上传） |
| 依赖 / 幂等 | 无本地依赖；不收 client_token，CLI 重试前回读去重 | 需 whiteboard-cli；每批带确定性 client_token，原样重跑不翻倍 |

结论：

1. 默认用服务端解析——可编辑性与本地管道相同，一条命令、无本地依赖，还能拿到降级清单；
2. SVG 有画布外元素需要裁掉、需要离线 `--dry-run` 预检节点、或要精确控制 z_index 时，用本地管道；
3. 单节点 `board svg-import` 仍只用于小图标（见下一节）。

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

其他情况都拆成原生节点：默认 `board import --syntax svg`，需要裁剪溢出时用 5 步管道（见第 0 节）。

---

## 2. 标准工作流（生成 SVG + 本地 5 步管道）

服务端解析只需要生成 SVG 后执行 `board import <board_id> drawing.svg --syntax svg -o json`；
下面的 Step 2–5 是本地管道 `scripts/svg_to_board.py` 内部做的事，手工排障时可对照。

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
- **曲线连接线**：2026-10 复测（whiteboard-cli 0.2.13 与服务端解析）`Q`/`C` 曲线 path 转为 curve connector，
  状态机自环、S 形曲线都按原形渲染；弧线 `A` 转为 svg/image 节点，形状正确但不再是连线。早期版本曾把自环渲染成
  悬空直线——落板后若曲线连线异常，改用多段 `<line>` / `<polyline>` 折线拼（转 connector，箭头三角用小 `<polygon>`）。

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
服务端解析会按 SVG 顺序自动赋值，不需要这一步。

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

规则（`svg_to_board.py` 的 `step3_trim_overflow`）：按完整 viewBox `min-x min-y width height` 判断边界（支持非零/负原点）——
完全在外的节点删除；部分越界的几何节点截断宽高；越界的 `svg` 节点直接删除（`svg_code` 与节点宽高绑定，截断会扭曲渲染）；
复合节点内部子坐标保持原样。服务端解析不做这一步，画布外元素需要先在 SVG 里删掉。

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

注意：手工分批时给每批带固定的 `--client-token`（≥10 字符）；输出无法解析或超时等结果不明时，先 `board nodes` 回读核对，
再用**同一个 token** 重跑该批，不要换 token 重传（详见 `pitfalls.md` 陷阱 3）。

---

## 3. 一键脚本：`scripts/svg_to_board.py`

5 步全自动管道，需要本地裁剪/预检时用它而不是手写：

```bash
# 基础用法（自动从 SVG 解析 viewBox）
python3 scripts/svg_to_board.py drawing.svg <board_id>

# 自定义 viewBox（如果 SVG 没标 viewBox 属性）
python3 scripts/svg_to_board.py drawing.svg <board_id> --viewbox 1600x900

# 自定义批次和节流
python3 scripts/svg_to_board.py drawing.svg <board_id> --batch 200 --interval 0.5

# CLI 不在 PATH 时指定路径
python3 scripts/svg_to_board.py drawing.svg <board_id> --feishu-cli ./bin/feishu-cli

# dry-run：跑 Step 1-3 但不上传（调试用）
python3 scripts/svg_to_board.py drawing.svg <board_id> --dry-run

# 不裁剪溢出节点（不建议，仅用于调试）
python3 scripts/svg_to_board.py drawing.svg <board_id> --keep-overflow
```

退出码：
- `0` 全部成功
- `1` whiteboard-cli 或 feishu-cli 不可用
- `2` SVG 解析失败 / viewBox 无法识别 / 修剪后无节点
- `3` 有批次失败、不完整或结果不确定，或回读验证未完成——先 `board nodes` 核对；补传时原样重跑同一命令
  （每批 client_token 由画板 ID + 本批内容决定，已落地的批次返回原节点，不会翻倍）

脚本输出示例（实跑）：

```
  viewBox = 0.0 0.0 800.0 500.0

=== Step 1: whiteboard-cli 翻译 SVG → 节点 JSON ===
  翻译成功：15 个节点
  类型分布：{'composite_shape': 8, 'text_shape': 2, 'connector': 2, 'svg': 2, 'group': 1}

=== Step 2: 修 z_index（画家算法） ===
  已为 15 个节点显式赋 z_index = 0..14

=== Step 3: 修剪 viewBox 溢出（原点 0.0,0.0，800.0x500.0） ===
  保留 14，删除 1 个完全溢出节点，截断 1 个边缘节点

=== Step 4: 分批上传（batch=300 interval=0.3s） ===
  ✓ 批 0-14 上传 14
  已确认创建：14/14（失败或未确认 0）

=== Step 5: 回读验证（节点总数下限检查） ===
  画板节点总数：14（含已有节点；已确认新增 14）
  类型分布：{...}
  节点总数下限检查通过；不能据此确认每个新增节点的内容或是否重复

========== 完成（2.7s）==========
```

---

## 4. SVG 元素 → 飞书节点翻译映射

whiteboard-cli 的翻译规则（2026-10 用 0.2.13 实测整理；服务端解析规则相同，只是把 svg 节点换成 image 节点）：

| SVG 元素 | 飞书节点 type | 子类型 / 关键字段 | 备注 |
|----------|--------------|------------------|------|
| `<rect rx=0>` | `composite_shape` | `composite_shape.type: rect` |  |
| `<rect rx>0>` | `composite_shape` | `composite_shape.type: round_rect` | 圆角自动识别 |
| `<circle>` | `composite_shape` | `composite_shape.type: ellipse` | 内部按 ellipse 统一 |
| `<ellipse>` | `composite_shape` | `composite_shape.type: ellipse` |  |
| `<text>` | `text_shape` | `text.text / font_size / font_weight / text_color / horizontal_align` | text-anchor 映射 |
| `<line>` | `connector` | `connector.shape: straight` | 直线 |
| `<path d="M ... L ... L ...">` 折线 | `connector` | `connector.shape: right_angled_polyline` + `turning_points` | 按原折点渲染 |
| `<path>` 含 `Q` / `C` 曲线 | `connector` | `connector.shape: curve` | 曲率保留 |
| `<path>` 含 `A` 弧线、闭合填充路径 | `svg` | `svg.svg_code` 保留 path 标签 | 服务端解析为 image 节点 |
| `<polygon>`（含三角形） | `svg` | `svg.svg_code` 保留 polygon | 不会识别为 triangle 形状；服务端为 image 节点 |
| `<polyline>` | `connector` | `connector.shape: right_angled_polyline` | |
| `<g>` | `group` | 子节点带 `parent_id` 指向 group | |
| `<linearGradient>` 被 `<rect>` 引用 | `composite_shape` | `style.fill_gradient` | 服务端解析取首个色并报 `degraded_attributes` |
| `stroke-dasharray` | 边框 | `style.border_style: dash` | 自定义虚线模式不保留 |

### 翻译边界情况

- `transform="rotate(...)"`：会展开到节点的 `angle` 字段（仅 `<text>` 和 `<g>` 支持）
- `transform="translate(...)"`：会被内联到 x/y 中
- `transform="scale(...)"`：可能丢失，不建议用
- `opacity`：转为 `style.fill_opacity` / `style.border_opacity`
- `font-family`：飞书强制用自家字体（Noto / 苹方），不保留自定义字体
- `<use>`：不支持（whiteboard-cli 可能展开，可能丢失）
- 曲线 `<path>`：`Q`/`C` 转 curve connector，`A` 弧转 svg/image 节点（见上表）；落板后曲线连线异常时改用多段 `<line>` / `<polyline>` 折线

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

> 口径说明：本节 2000 是实测渲染容量上限；`schema.md` 节点密度表的 600 是 `board lint` 的不扣分阈值（>600 触发 over_capacity 固定扣 0.2），两个数字不矛盾。

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
- [ ] **缩略图主要元素都在**：`board image <id> /tmp/check`（自动补实际扩展名，通常 .jpg）后看；写入后缩略图可能滞后
  10–20 秒，拿到旧图时稍后重下
- [ ] **lint 质量分 ≥ 0.85**：`board lint <id>`；节点 >600 时 over_capacity 固定扣 0.2 属预期，按 ≥ 0.65 评估

如其中任何一项不通过，回到 `pitfalls.md` 排障。

---

## 8. 一句话总结

| 需求 | 路径 |
|------|------|
| 让大图里的小元素可点击编辑 | 默认 `board import --syntax svg`；需裁剪溢出时用 5 步管道 + `scripts/svg_to_board.py` |
| 简单图标 / 印章 | `board svg-import`（单节点） |
| 200-2000 节点的复杂图 | 两条路径都可（服务端 1500 节点实测约 3 秒） |
| > 2000 节点 | 先考虑简化或拆图 |
