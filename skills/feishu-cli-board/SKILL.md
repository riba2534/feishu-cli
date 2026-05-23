---
name: feishu-cli-board
description: >-
  飞书画板全能操作 · 5 种画图路径任选其一：
  (A) Mermaid/PlantUML 服务端渲染（思维导图/时序图/类图/饼图/流程图/甘特图，整图作为一个节点，飞书自动排版）
  (B) Mermaid 本地引擎 whiteboard-cli（绕开 par/参与方数等服务端限制，每个节点可点击编辑）
  (C) SVG 自由作图（任意视觉：飞轮/鱼骨/Dashboard/海报/插画/架构图等，每个 SVG 元素 → 1 个原生节点可单独编辑）
      → 使用 scripts/svg_to_board.py 把 SVG 翻译为飞书原生节点
  (D) 简单 SVG 单节点装饰（图标/印章/小元素，2KB 以内 SVG）
  (E) 精排架构图（手写节点 JSON，绝对坐标 + 配色 + 连线 ID 引用）
  当用户请求"画图/画板/whiteboard/画架构图/画流程图/画飞轮/画鱼骨/画路线图/画 Dashboard/画插画/画海报/
  AI 自由作图/SVG 落画板/克隆画板/上传图片到画板/可视化/节点图/精排"时使用。
  特别地，当用户反馈"右下角半截楼""z_index 错乱""节点翻倍""复杂图渲染不全""mermaid 服务端失败"
  务必读 references/pitfalls.md 排障。
  写类（add-board/create-notes/update/delete/clone/svg-import/upload-image/import）默认 App Token（Bot 身份），无需登录；
  读类（image/nodes/export-code/lint）登录后自动 User Token，未登录回落 App Token。
argument-hint: "[whiteboard_id]"
user-invocable: true
allowed-tools: Bash(feishu-cli board:*), Bash(feishu-cli doc:*), Bash(feishu-cli media:*), Bash(python3:*), Bash(whiteboard-cli:*), Read, Write
---

# 飞书画板 · 5 路径画图指南

## 前置条件

- **feishu-cli**：未装就到 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 装
- **认证**：环境变量 `FEISHU_APP_ID` + `FEISHU_APP_SECRET` 或 `~/.feishu-cli/config.yaml`
- **权限**：`board:whiteboard`（画板读写）+ `docx:document`（文档加画板）
- **whiteboard-cli**（路径 B/C 用）：`npm i -g @larksuite/whiteboard-cli`

---

## 选型决策树

```
画什么？
│
├─ 标准图表（思维导图/时序图/类图/饼图/流程图/甘特图）
│  ├─ 图整图展示即可，不需要单独编辑节点 → 路径 A（Mermaid 服务端）
│  └─ 需要单独编辑每个节点 / Mermaid 含 par / 10+ participant / 30+ 长标签
│     → 路径 B（Mermaid 本地引擎）
│
├─ AI 自由作图（飞轮/鱼骨/价值金字塔/转化漏斗/桑基/路线图/Dashboard/海报/
│              Mobile UI/户型图/地铁图/插画/周期表/机芯/赛博朋克城市等）
│  ⭐ → 路径 C（SVG → 原生节点，每个元素可单独点击编辑）
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
| "mermaid 服务端报错 / 不支持 par / 超复杂" | B |
| "画个增长飞轮 / 鱼骨分析 / 价值金字塔" | **C** ⭐ |
| "画个 Dashboard / Mobile UI / 海报" | **C** ⭐ |
| "画个插画 / 户型图 / 地铁图" | **C** ⭐ |
| "AI 自由设计的图（用 Claude 直接吐 SVG）" | **C** ⭐ |
| "需要图里每个元素都能单独点击 / 改色 / 拖动" | **C** ⭐ |
| "上传一个小图标 / 印章到画板" | D |
| "画个 6 微服务架构图，需要精确坐标和连线" | E |

---

## 路径 A：Mermaid/PlantUML 服务端

适合：标准图表，整图作为一个节点展示。

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
```

### 限制（详见 references/mermaid-engines.md）

- 整张图作为一个节点，无法拆开编辑
- 不支持 `par` 语法
- ≥10 participant / ≥3 层 alt / ≥30 长标签可能失败
- CLI 自动诊断复杂度并向 stderr 警告

---

## 路径 B：Mermaid 本地引擎

适合：复杂 Mermaid（服务端会失败），或需要每个节点单独编辑。

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
    └─ feishu-cli 把节点 JSON 用 create-notes 分批上传
```

详细对比与陷阱见 `references/mermaid-engines.md`。

---

## 路径 C：SVG → 原生节点 ⭐

适合：所有 AI 自由设计图，每个元素都是独立可编辑的飞书节点。

### 快速开始（一键脚本）

```bash
# Step 1: 准备 SVG（手写 / Python 生成 / AI 吐）
# 示例：用 Claude 生成一个增长飞轮 SVG，保存为 flywheel.svg

# Step 2: 创建文档 + 画板
DOC_ID=$(feishu-cli doc create --title "增长飞轮" -o json | jq -r .document_id)
BOARD_ID=$(feishu-cli doc add-board $DOC_ID -o json | jq -r .whiteboard_id)

# Step 3: 一键 SVG → 飞书画板（5 步管道自动执行）
python3 ./skills/feishu-cli-board/scripts/svg_to_board.py flywheel.svg $BOARD_ID
```

脚本会自动执行 5 步：

1. **whiteboard-cli 翻译**：SVG → 节点 JSON
2. **修 z_index**：按数组顺序显式赋值（修陷阱 1）
3. **修剪 viewBox 溢出**：删超出节点（修陷阱 2）
4. **分批 create-notes**：每批 300，间隔 0.3s（防限流）
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
| C（svg_to_board.py） | N（每个元素 1 个节点） | ✅ 每个 rect/text/path 都可单选 | 不限 |

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
   "style":{"fill_color":"#FFFFFF","fill_opacity":100,"border_style":"solid","border_color":"#5178C6","border_width":"medium","border_opacity":100}},
  {"type":"composite_shape","x":400,"y":100,"width":160,"height":40,"z_index":10,
   "composite_shape":{"type":"round_rect"},
   "text":{"text":"服务 B","font_size":14,"font_weight":"regular","horizontal_align":"center","vertical_align":"mid"},
   "style":{"fill_color":"#FFFFFF","fill_opacity":100,"border_style":"solid","border_color":"#509863","border_width":"medium","border_opacity":100}}
]
EOF
feishu-cli board create-notes $BOARD_ID /tmp/shapes.json -o json
# → {"node_ids":["o1:1","o1:2"]}

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
- **配色系统**：`references/style.md`（5 套色板 + 结构规则）
- **连线策略**：`references/connectors.md`（snap_to / shape / 间距）
- **排版规则**：`references/typography.md`（字号层级）
- **信息规划**：`references/content.md`（信息量参考）

---

## 全命令速查

| 命令 | 作用 | 关键参数 |
|------|------|---------|
| `feishu-cli doc add-board <doc_id>` | 在文档加画板块 | `--parent-id` `--index` |
| `feishu-cli board nodes <board_id>` | 拉所有节点 | 无 |
| `feishu-cli board image <board_id> out.png` | 下载画板缩略图 | 无 |
| `feishu-cli board create-notes <board_id> nodes.json` | 批量创建节点 | `--source-type` `--client-token` |
| `feishu-cli board import <board_id> diagram.mmd --syntax mermaid` | 路径 A：服务端渲染 | `--engine [server\|local]` `--diagram-type` `--style` `--dry-run` |
| `feishu-cli board svg-import <board_id> drawing.svg` | 路径 D：单 svg 节点 | `--x` `--y` `--width` `--height` `--source-type` `--dry-run` |
| `python3 svg_to_board.py drawing.svg <board_id>` | 路径 C：5 步管道 | `--viewbox WxH` `--batch` `--interval` `--keep-overflow` `--dry-run` |
| `feishu-cli board update <board_id> nodes.json` | 更新画板（覆盖模式） | `--overwrite` `--snapshot` `--dry-run` `--stdin` |
| `feishu-cli board delete <board_id> --all` | 删全部节点 | `--node-ids` |
| `feishu-cli board clone <src> <dst>` | 克隆画板 | `--batch-size` `--interval` `--filter-types` `--dry-run` |
| `feishu-cli board upload-image <board_id> photo.png` | 图片转 image 节点 | `--x` `--y` `--width` `--height` `--dry-run` |
| `feishu-cli board lint <board_id>` | 几何质检 | 无 |
| `feishu-cli board export-code <board_id>` | 反向导出 SVG | `--output-path` `--merge` |

详细的每个命令的内部行为见对应代码：`cmd/board_*.go`。

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
- **根因**：脚本 stdout 解析失败但 API 已成功 → 重传导致翻倍
- **修复**：rc=0 一律按成功，已翻倍则 `board delete --all` 后重传
- **一键修复**：`scripts/svg_to_board.py` Step 4 容错解析内置

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
| `2890002 invalid arg` | 含多余字段（id/locked/children 等只读字段） | `references/schema.md` |
| Mermaid 服务端报错 | 切 `--engine local` 或改 SVG | `references/mermaid-engines.md` |

---

## 端到端验证清单

落板后逐项检查：

- [ ] 缩略图主元素都在：`feishu-cli board image <id> /tmp/check.png`
- [ ] 节点数对：`feishu-cli board nodes <id> | jq '.data.nodes | length'`
- [ ] z_index 最小是大背景：见 `references/pitfalls.md` 通用诊断 Step 2
- [ ] viewBox 无溢出：`max(x+w) ≤ viewBox_w`
- [ ] lint 质量分 ≥ 0.85：`feishu-cli board lint <id>`

任何一项不通过，回 `references/pitfalls.md` 排障。

---

## 参考文档索引

| 文件 | 何时读 |
|------|-------|
| `references/svg-workflow.md` | 走路径 C 时必读（5 步管道详解 + 翻译映射表） |
| `references/mermaid-engines.md` | 走路径 A/B 时必读（服务端 vs 本地引擎选型） |
| `references/pitfalls.md` | ⭐ 实战必读（z_index / viewBox / 翻倍三大陷阱排障） |
| `references/examples-real.md` | 设计参考（14 张实战图的模式 + SVG 元素 + 节点密度） |
| `references/schema.md` | 走路径 E 时必读（节点 JSON 权威参考） |
| `references/layout.md` | 走路径 E 时必读（5 种布局策略 + 间距规则） |
| `references/style.md` | 路径 C/E 配色参考（5 套色板） |
| `references/connectors.md` | 用连线时参考（snap_to / shape 选择） |
| `references/typography.md` | 文字排版参考（字号层级） |
| `references/content.md` | 信息量规划（避免过载） |
| `references/node-api.md` | API 端点详解、错误码排障、典型工作流 |

---

## 一句话总结

| 用户描述 | 命令 |
|---------|------|
| "画个增长飞轮 / 鱼骨 / Dashboard" | `python3 svg_to_board.py drawing.svg $BOARD` |
| "把这份 mermaid 落到画板" | `feishu-cli board import $BOARD diagram.mmd --syntax mermaid` |
| "mermaid 服务端失败 / 太复杂" | 加 `--engine local` |
| "上传个小图标 / 印章" | `feishu-cli board svg-import $BOARD icon.svg` |
| "精排架构图，每个节点要手摆位置" | 手写 nodes.json + `feishu-cli board create-notes` |
| "克隆这张画板" | `feishu-cli board clone <src> <dst>` |
| "把这张图片放到画板" | `feishu-cli board upload-image $BOARD photo.png` |
| "检查画板质量" | `feishu-cli board lint $BOARD` |
| "把画板里的 SVG 拉回本地" | `feishu-cli board export-code $BOARD --output-path design.svg --merge` |
