# 飞书画板 · 5 路径画图指南

## 目录

- [前置条件](#前置条件)
- [选型决策树](#选型决策树)
- [服务端 Mermaid/PlantUML](#路径-amermaidplantuml-服务端)
- [Mermaid 本地引擎](#路径-bmermaid-本地引擎)
- [SVG 原生节点](#路径-csvg--原生节点-)
- [SVG 单节点](#路径-dsvg-单节点装饰)
- [精排架构图](#路径-e精排架构图)
- [命令速查](#全命令速查)
- [陷阱与验证](#三大致命陷阱速查)

## 前置条件

- **身份**：写命令（import / create-notes / update / svg-import / upload-image / delete / clone）默认 Bot；
  读命令（nodes / image / lint / export-code / svg-export）User 优先、未配置时回落 Bot。画板在用户本人的文档里
  （Bot 无权访问）时，写命令显式传 `--user-access-token` 或设置 `FEISHU_USER_ACCESS_TOKEN`
- **权限**：`board:whiteboard:node:read` / `board:whiteboard:node:create` / `board:whiteboard:node:delete`；
  `doc add-board` 另需文档写权限（`docx:document` 或 `docx:document:write_only`）
- **whiteboard-cli**（路径 B 与路径 C 的本地管道用）：`npm i -g @larksuite/whiteboard-cli`；
  本仓库实测版本 0.2.13，升级后先用 `--dry-run`（svg_to_board.py）或小图核对节点输出再批量使用

---

## 选型决策树

```
画什么？
│
├─ 标准图表（思维导图/时序图/类图/饼图/流程图/甘特图）
│  ├─ 默认 → 路径 A（服务端解析：一个 section 分组 + 原生子节点，可取回源码）
│  └─ 服务端报 Parse error / Invalid request parameter，或不要 section 分组
│     → 路径 B（Mermaid 本地引擎）
│
├─ AI 自由作图（飞轮/鱼骨/价值金字塔/转化漏斗/桑基/路线图/Dashboard/海报/
│              Mobile UI/户型图/地铁图/插画/周期表/机芯/赛博朋克城市等）
│  ⭐ → 路径 C（SVG → 原生节点，每个元素可单独点击编辑）
│     ├─ 默认：`board import --syntax svg`（服务端解析，一条命令，无需 whiteboard-cli）
│     └─ 需要裁剪 viewBox 溢出 / 离线预检 / 超大图分批：svg_to_board.py 本地管道
│
├─ 简单 SVG 装饰（图标/印章/小元素，< 2KB SVG）
│  └─ 路径 D（svg-import 单节点）
│
└─ 精排架构图 / 对比矩阵 / 组织树（需要绝对坐标 + 特定配色 + 连接线 ID 引用）
   └─ 路径 E（手写节点 JSON + create-notes）
```

**路径选择速查**：

| 用户描述 | 推荐路径 |
|---------|---------|
| "画个思维导图 / 时序图 / 类图" | A |
| "画个流程图" | A |
| "把这份 mermaid 落到画板" | A |
| "mermaid 服务端报 Parse error / Invalid request parameter" | B |
| "画个增长飞轮 / 鱼骨分析 / 价值金字塔" | **C** ⭐ |
| "画个 Dashboard / Mobile UI / 海报" | **C** ⭐ |
| "画个插画 / 户型图 / 地铁图" | **C** ⭐ |
| "AI 自由设计的图（用 Claude 直接吐 SVG）" | **C** ⭐ |
| "需要图里每个元素都能单独点击 / 改色 / 拖动" | **C** ⭐ |
| "上传一个小图标 / 印章到画板" | D |
| "画个 6 微服务架构图，需要精确坐标和连线" | E |

---

## 路径 A：Mermaid/PlantUML 服务端

适合：标准图表。服务端把整张图落为一个 `section` 分组节点（保存 `syntax.code` 源码），
内部是原生 `composite_shape` / `connector` / `life_line` / `mind_map` 等子节点。

### 快速开始

```bash
DOC_ID=$(feishu-cli doc create --title "示例" -o json | jq -r .document_id)
BOARD_ID=$(feishu-cli doc add-board $DOC_ID -o json | jq -r .whiteboard_id)

# 从文件导入
feishu-cli board import $BOARD_ID flowchart.mmd --syntax mermaid

# 从字符串导入
feishu-cli board import $BOARD_ID "graph TD; A-->B-->C" --source-type content --syntax mermaid --diagram-type flowchart

# PlantUML
feishu-cli board import $BOARD_ID diagram.puml --syntax plantuml

# 取回画板上图表的 Mermaid/PlantUML 源码（多个图表时加 --node-id）
feishu-cli board export-code $BOARD_ID --source
```

- `--syntax` 只接受 `plantuml` / `mermaid` / `svg`（`--style`、`--diagram-type` 同样白名单），未知取值直接报用法错误（exit 2），不会再被静默当成 PlantUML
- 服务端引擎接口不认 `client_token`（实测同一 token 重复请求会重复建图）：未带 `--overwrite` 时 CLI 在重试前回读画板顶层节点，上一次请求已落地就不再重复提交
- 同一画板多次 import 的图会叠放在同一原点：一张画板放一张图；`--overwrite` 会先清空**整张画板**（含其他图表）再写入，
  不能只替换其中一张
- `-o json` 输出 `ticket_id`（新 section 节点 ID）；`--syntax svg` 时另有 `degraded_attributes`

### 限制（详见 references/mermaid-engines.md）

- 布局由服务端决定，不能指定落点坐标
- CLI 只在规模明显超出实测范围（≥20 participant / ≥6 个 alt 块 / ≥50 行长标签）时在 stderr 提示，不阻断执行；2026-10 复测
  par、12 个 participant、3 层嵌套 alt 均能正常渲染。真正失败（Parse error / Invalid request parameter）时再改 `--engine local`
- `board import` 失败直接报错，不会降级为代码块（降级只发生在 `doc import`）

---

## 路径 B：Mermaid 本地引擎

适合：服务端解析失败（Parse error / Invalid request parameter）的 Mermaid，或不要 section 分组、要顶层独立节点。

### 快速开始

```bash
# 一次性装好本地引擎（仅首次）
npm i -g @larksuite/whiteboard-cli

# 走本地引擎
feishu-cli board import $BOARD_ID complex.mmd --syntax mermaid --engine local
```

### 工作原理

```
feishu-cli board import --engine local
    │
    ├─ whiteboard-cli 把 Mermaid 翻译为节点 JSON（在本地，不调飞书）
    └─ feishu-cli 把节点 JSON 上传为画板节点（可带 --client-token 幂等重跑）
```

本地引擎输出的节点不带 z_index、也不裁剪 viewBox 溢出；SVG 不要走 `--engine local`，用路径 C。
详细对比与陷阱见 `references/mermaid-engines.md`。

---

## 路径 C：SVG → 原生节点 ⭐

适合：所有 AI 自由设计图，每个元素都是独立可编辑的飞书节点。

### 快速开始 1：服务端 SVG 解析（默认）

```bash
DOC_ID=$(feishu-cli doc create --title "增长飞轮" -o json | jq -r .document_id)
BOARD_ID=$(feishu-cli doc add-board $DOC_ID -o json | jq -r .whiteboard_id)

feishu-cli board import $BOARD_ID flywheel.svg --syntax svg -o json
```

服务端（`syntax_type=3`）把 rect/circle/ellipse/text/line/折线与曲线 path 拆成原生可编辑节点（polygon、弧线等转为 image 节点），
按 SVG 顺序赋 z_index；
渐变 `fill=url(#id)`、自定义 `stroke-dasharray` 等不支持的属性会降级并在输出 `degraded_attributes` 中列出。
它**不裁剪** viewBox 外的元素——画布外有元素时先在 SVG 里删掉，或改用下面的本地管道。
A/B 实测结论见 `references/svg-workflow.md`「服务端 SVG 解析 vs 本地管道」。

### 快速开始 2：本地管道（一键脚本）

```bash
# Step 0: 定色板（生成 SVG 之前）—— 用户指定品牌/色板时沿用并校验；否则取统一色板
#   结构分组用浅底/深边对、数据系列用 categorical 原色，见 references/style.md。
#   原样按序取用统一色板时无需校验（已预校验，结论见 `../dataviz/references/palette.md`）；
#   仅当改色值/换底色时先跑校验（定位方式见 references/style.md"数据图表的系列色"一节）。

# Step 1: 准备 SVG（手写 / Python 生成 / AI 吐）
# 示例：用 Claude 生成一个增长飞轮 SVG，保存为 flywheel.svg

# Step 2: 创建文档 + 画板
DOC_ID=$(feishu-cli doc create --title "增长飞轮" -o json | jq -r .document_id)
BOARD_ID=$(feishu-cli doc add-board $DOC_ID -o json | jq -r .whiteboard_id)

# Step 3: 一键 SVG → 飞书画板（5 步管道自动执行）
python3 scripts/svg_to_board.py flywheel.svg $BOARD_ID
```

脚本会自动执行 5 步：

1. **whiteboard-cli 翻译**：SVG → 节点 JSON
2. **修 z_index**：按数组顺序显式赋值（修陷阱 1）
3. **修剪 viewBox 溢出**：按完整 `min-x min-y width height` 边界裁剪，支持非零/负原点；复合节点内部坐标保持原样（修陷阱 2）
4. **分批 create-notes**：每批 300，间隔 0.3s（防限流）；每批带确定性 `--client-token`，原样重跑不会翻倍
5. **验证**：拉真实节点数 / 类型分布对比

### 完整工作流详解

读 `references/svg-workflow.md`：包含 SVG 元素 → 飞书节点的翻译映射表、14 张实战图的节点密度参考、何时拆图的边界。

### SVG 设计参考

读 `references/examples-real.md`：14 张实战图的设计模式索引、每张图的 SVG 元素组合、关键技术点（极坐标 / 三角函数 / cubic-bezier 等）。

---

## 路径 D：SVG 单节点装饰

适合：图标 / 印章 / 小元素（< 2KB SVG），不需要拆开编辑。

### 快速开始

```bash
feishu-cli board svg-import $BOARD_ID icon.svg \
    --x 100 --y 100 --width 60 --height 60

# 自动从 viewBox 推断尺寸
feishu-cli board svg-import $BOARD_ID badge.svg --x 0 --y 0

# 直接传字符串
feishu-cli board svg-import $BOARD_ID '<svg viewBox="0 0 100 100">...</svg>' \
    --source-type content --x 50 --y 50

# 预览不发请求
feishu-cli board svg-import $BOARD_ID drawing.svg --dry-run
```

### 与路径 C 的核心差异

| 路径 | 节点数 | 可编辑性 | 适合复杂度 |
|------|--------|---------|----------|
| D（svg-import） | 1（整图 1 个 svg 节点） | ❌ 整图作为一个矢量贴图 | < 2 KB SVG |
| C（`import --syntax svg` / svg_to_board.py） | N（每个元素 1 个节点） | ✅ 每个 rect/text/path 都可单选 | 实测 1500 元素可用 |

---

## 路径 E：精排架构图

适合：架构图 / 对比矩阵 / 组织树，需要绝对坐标 + 特定配色 + 连接线 ID 引用。

### 快速开始

```bash
# 1. 创建文档 + 画板
DOC_ID=$(feishu-cli doc create --title "微服务架构" -o json | jq -r .document_id)
BOARD_ID=$(feishu-cli doc add-board $DOC_ID -o json | jq -r .whiteboard_id)

# 2. 写 shapes.json（先形状）
cat > /tmp/shapes.json << 'EOF'
[
  {"type":"composite_shape","x":100,"y":100,"width":160,"height":40,"z_index":10,
   "composite_shape":{"type":"round_rect"},
   "text":{"text":"服务 A","font_size":14,"font_weight":"regular","horizontal_align":"center","vertical_align":"mid"},
   "style":{"fill_color":"#FFFFFF","fill_opacity":100,"border_style":"solid","border_color":"#1446C2","border_width":"medium","border_opacity":100}},
  {"type":"composite_shape","x":400,"y":100,"width":160,"height":40,"z_index":10,
   "composite_shape":{"type":"round_rect"},
   "text":{"text":"服务 B","font_size":14,"font_weight":"regular","horizontal_align":"center","vertical_align":"mid"},
   "style":{"fill_color":"#FFFFFF","fill_opacity":100,"border_style":"solid","border_color":"#0C6800","border_width":"medium","border_opacity":100}}
]
EOF
feishu-cli board create-notes $BOARD_ID /tmp/shapes.json -o json
# → {"count":2,"node_ids":["o1:1","o1:2"],"whiteboard_id":"..."}

# 3. 写 connectors.json（再连线，引用上面的 node_ids）
cat > /tmp/connectors.json << 'EOF'
[
  {"type":"connector","width":1,"height":1,"z_index":50,
   "connector":{"shape":"polyline",
     "start":{"arrow_style":"none","attached_object":{"id":"o1:1","position":{"x":1,"y":0.5},"snap_to":"right"}},
     "end":{"arrow_style":"triangle_arrow","attached_object":{"id":"o1:2","position":{"x":0,"y":0.5},"snap_to":"left"}}},
   "style":{"border_color":"#BBBFC4","border_opacity":100,"border_style":"solid","border_width":"narrow"}}
]
EOF
feishu-cli board create-notes $BOARD_ID /tmp/connectors.json -o json
```

### 参考资料

- **节点 JSON Schema**：`references/schema.md`
- **布局策略**：`references/layout.md`（分层条带 / 行列对齐 / 岛屿式 / 树状）
- **配色系统**：`references/style.md`（统一色板取用方式 + 5 个主题变体 + 结构规则）
- **连线策略**：`references/connectors.md`（snap_to / shape / 间距）
- **排版规则**：`references/typography.md`（字号层级）
- **信息规划**：`references/content.md`（信息量参考）

---

## 全命令速查

| 命令 | 作用 | 关键参数 |
|------|------|---------|
| `feishu-cli doc add-board <doc_id>` | 在文档加画板块 | `--parent-id`（父块 ID，默认根级别）`--index`（插入位置，-1=末尾）`-o json` |
| `feishu-cli board nodes <board_id>` | 拉所有节点 | 无 |
| `feishu-cli board image <board_id> out` | 下载画板缩略图（自动按实际格式补扩展名，通常 JPEG） | 无 |
| `feishu-cli board create-notes <board_id> nodes.json` | 批量创建节点 | `--source-type` `--client-token` `--overwrite`（清空整板） |
| `feishu-cli board import <board_id> diagram.mmd --syntax mermaid` | 路径 A：服务端渲染；`--syntax svg` 为路径 C 服务端解析 | `--syntax [plantuml\|mermaid\|svg]` `--engine [server\|local]` `--diagram-type` `--style` `--overwrite`（清空整板）`--client-token`（仅 local）`--dry-run` |
| `feishu-cli board svg-import <board_id> drawing.svg` | 路径 D：单 svg 节点 | `--x` `--y` `--width` `--height` `--source-type` `--client-token` `--dry-run` |
| `python3 scripts/svg_to_board.py drawing.svg <board_id>` | 路径 C：5 步本地管道 | `--viewbox WxH`（覆盖为零原点视口，默认自动解析完整 viewBox）`--keep-overflow`（不裁剪溢出节点）`--batch`（默认 300）`--interval`（默认 0.3s）`--feishu-cli`（CLI 路径）`--dry-run` |
| `feishu-cli board update <board_id> nodes.json` | 写入节点（默认追加；`--overwrite` 原子清空整板后写入） | `--overwrite` `--snapshot` `--client-token` `--dry-run` `--stdin` |
| `feishu-cli board delete <board_id> --all` | 删节点，无确认立即执行（`--all` 清空整板；空画板提示"没有节点"并成功退出） | `--node-ids` |
| `feishu-cli board clone <src> <dst>` | 克隆画板（目标应为空画板；默认 Bot 读源画板） | `--batch-size`（默认 10）`--interval`（默认 1s）`--filter-types` `--dry-run` |
| `feishu-cli board upload-image <board_id> photo.png` | 图片转 image 节点（支持 jpeg/png/gif/webp/bmp/tiff，只读文件头取尺寸；EXIF Orientation 5-8 旋转的 JPEG——手机竖拍照片最常见——无法自动取尺寸，会报错要求显式 `--width/--height`） | `--x` `--y` `--width` `--height` `--dry-run` |
| `feishu-cli board lint <board_id>` | 几何质检 | 无 |
| `feishu-cli board export-code <board_id>` | 反向导出 SVG；`--source` 取回 Mermaid/PlantUML 源码 | `--output-path` `--merge` `--source` `--node-id` `--overwrite`（输出文件已存在时默认报错） |
| `feishu-cli board svg-export <board_id> --output-path board.svg` | 服务端整板渲染 SVG 快照 | `--output-path` `--overwrite`（目标已存在时必须） |

---

## 三大致命陷阱（速查）

实战中**最容易踩**的三个坑，全部在 `references/pitfalls.md` 有详细排障：

### 陷阱 1: z_index 错乱 ⭐⭐⭐

- **现象**：大背景遮挡前景，画板视觉混乱
- **根因**：whiteboard-cli 输出节点不带 z_index，飞书自动分配是乱序
- **修复**：上传前按数组 index 显式赋 `z_index = i`
- **一键修复**：`scripts/svg_to_board.py` Step 2 内置

### 陷阱 2: viewBox 溢出 ⭐⭐

- **现象**：右下角"半截楼"诡异图形
- **根因**：节点 `x + width > viewBox_w`
- **修复**：上传前过滤 / 截断溢出节点
- **一键修复**：`scripts/svg_to_board.py` Step 3 内置

### 陷阱 3: 节点翻倍 ⭐⭐

- **现象**：清空重传后节点数 ×2
- **根因**：脚本把"输出解析失败 / 超时"当成失败重传，但 API 其实已成功
- **修复**：结果不明时先 `board nodes` 回读核对，不要直接重传；已翻倍则 `board delete --all` 后重传
- **一键修复**：`scripts/svg_to_board.py` 每批带确定性 `--client-token`，同一 SVG、同一画板原样重跑不会翻倍
- **预防**：`board update` / `svg-import` / `create-notes` / `import --engine local` 带同一个 `--client-token`（≥10 字符）重跑，
  服务端直接返回首次写入的节点 ID，不会翻倍（`/nodes` 端点实测幂等，10 分钟后重放仍返回原节点）

---

## 关键约束速查表

1. **先形状后连线**：connector 通过 ID 引用形状节点，必须先创建形状才能创建连线
2. **最小字段集**：多余字段触发 `2890002 invalid arg`，只用 `references/schema.md` 列出的安全字段
3. **背景色块 fill_opacity ≤ 25**：否则完全遮挡上层节点
4. **z_index 分层**：背景 0-1、次级 2-3、常规节点 10、连线 50
5. **坐标系为绝对坐标**：手写节点必须手算 x/y/width/height（路径 E）
6. **节点文字简短**：标题 + 简短说明（< 12 字），不写长段落
7. **同组节点视觉一致**：同分组用相同 `fill_color / border_color`
8. **节点数上限**：单画板 > 2000 节点时编辑器开始卡顿，考虑拆图或简化
9. **默认色板**：没有用户指定色板时，分组/系列色按 `references/style.md` 按序取用、不循环；采用品牌色或换底色时需跑 dataviz 校验器 `../dataviz/scripts/validate_palette.js`（默认色板已预校验）

---

## 症状 → 修复对照表

| 看到的问题 | 改什么 | 详见 |
|-----------|--------|------|
| 文字被截断 / 溢出 | 增大 width 或 height，或缩短文字 | `references/typography.md` |
| 节点重叠粘连 | 增大节点间距（同层 ≥ 30px，有连线 ≥ 60px） | `references/layout.md` |
| 背景色块遮挡节点 | 降低 fill_opacity（≤ 25），确认 z_index 分层 | `references/schema.md` |
| 连线穿过节点 | 调整 snap_to 方向或增大间距 | `references/connectors.md` |
| 大背景反而盖住前景 | z_index 错乱 → 显式赋值 | `references/pitfalls.md` ⭐ |
| 右下角"半截楼" | viewBox 溢出 → 修剪 | `references/pitfalls.md` ⭐ |
| 节点数翻倍 / 颜色加深 | 重传导致翻倍 → `board delete --all` 后重传 | `references/pitfalls.md` ⭐ |
| 文字和背景色太接近 | 调 fill_color 或 text.text_color，确保对比度 | `references/style.md` |
| 分组看不出来 | 同分组用同色，跨组换色 | `references/style.md` |
| 数据系列颜色难分辨（色盲/投影） | 系列色改按统一色板顺序取用并跑校验器 | `../dataviz/workflow.md` |
| 不确定该画柱状/折线/饼 | 未指定形式时按数据任务选择；用户已指定时先遵循，必要时说明适用性 | `../dataviz/references/choosing-a-form.md` |
| `2890002 invalid arg` | 含多余字段（id/locked/children 等只读字段） | `references/schema.md` |
| Mermaid 服务端报错 | 切 `--engine local` 或改 SVG | `references/mermaid-engines.md` |

---

## 端到端验证清单

落板后逐项检查：

- [ ] 缩略图主元素都在：`feishu-cli board image <id> /tmp/check`（自动补实际扩展名；缩略图在写入后可能滞后
  10–20 秒，拿到旧图时稍后重下）
- [ ] 节点数对：`feishu-cli board nodes <id> | jq '.data.nodes | length'`
- [ ] z_index 最小是大背景：见 `references/pitfalls.md` 通用诊断 Step 2
- [ ] viewBox 无溢出：`min_x ≤ x`、`x+w ≤ min_x+width`，y 方向同理
- [ ] lint 质量分 ≥ 0.85：`feishu-cli board lint <id>`；节点 >600 时 over_capacity 固定扣 0.2 属预期，按 ≥ 0.65 评估

任何一项不通过，回 `references/pitfalls.md` 排障。

---

## 参考文档索引

| 文件 | 何时读 |
|------|-------|
| `references/svg-workflow.md` | 走路径 C 时必读（5 步管道详解 + 翻译映射表） |
| `references/mermaid-engines.md` | 走路径 A/B 时必读（服务端 vs 本地引擎选型） |
| `references/plantuml-safe-subset.md` | 走路径 A 用 PlantUML 时必读（服务端可稳定解析的安全语法子集） |
| `references/pitfalls.md` | ⭐ 实战必读（z_index / viewBox / 翻倍三大陷阱排障） |
| `references/examples-real.md` | 设计参考（14 张实战图的模式 + SVG 元素 + 节点密度） |
| `references/schema.md` | 走路径 E 时必读（节点 JSON 权威参考） |
| `references/layout.md` | 走路径 E 时必读（5 种布局策略 + 间距规则） |
| `references/style.md` | 路径 C/E 配色参考（统一色板取用方式 + 5 个主题变体） |
| `references/connectors.md` | 用连线时参考（snap_to / shape 选择） |
| `references/typography.md` | 文字排版参考（字号层级） |
| `references/content.md` | 信息量规划（避免过载） |
| `references/node-api.md` | API 端点详解、错误码排障、典型工作流 |
| `references/basic-commands.md` | 需要单条 board 命令的完整参数与示例时查（image 下载 / import 变体等基础操作详解） |

---

## 一句话总结

| 用户描述 | 命令 |
|---------|------|
| "画个增长飞轮 / 鱼骨 / Dashboard" | `feishu-cli board import $BOARD drawing.svg --syntax svg`（需裁剪溢出时用 `python3 scripts/svg_to_board.py drawing.svg $BOARD`） |
| "把这份 mermaid 落到画板" | `feishu-cli board import $BOARD diagram.mmd --syntax mermaid` |
| "mermaid 服务端失败 / 太复杂" | 加 `--engine local` |
| "上传个小图标 / 印章" | `feishu-cli board svg-import $BOARD icon.svg` |
| "精排架构图，每个节点要手摆位置" | 手写 nodes.json + `feishu-cli board create-notes` |
| "克隆这张画板" | `feishu-cli board clone <src> <dst>` |
| "把这张图片放到画板" | `feishu-cli board upload-image $BOARD photo.png` |
| "检查画板质量" | `feishu-cli board lint $BOARD` |
| "把画板里的 SVG 拉回本地" | `feishu-cli board export-code $BOARD --output-path design.svg --merge` |
| "取回画板里 Mermaid/PlantUML 的源码" | `feishu-cli board export-code $BOARD --source --output-path diagram` |
