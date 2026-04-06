---
name: feishu-cli-board
description: >-
  飞书画板全功能操作：创建画板、绘制架构图/流程图/看板（通过 create-notes API 精确控制节点位置和样式）、
  导入 Mermaid/PlantUML 图表、下载画板图片、获取/复制画板节点。
  当用户请求"画个图"、"画架构图"、"画流程图"、"画板"、"whiteboard"、"create-notes"、
  "在飞书里画图"、"画个看板"、"可视化"、"节点图"时使用。
  也适用于：用户给出一组实体和关系，期望在飞书文档中生成可视化图表的场景。
  与 Mermaid 导入的区别：Mermaid 由飞书服务端自动排版，create-notes 可精确控制坐标、颜色、连线，
  适合需要精排的架构图和看板。
argument-hint: "[whiteboard_id]"
user-invocable: true
allowed-tools: Bash, Read, Write
---

# 飞书画板全功能操作

## 前置条件

- **feishu-cli**：如尚未安装，前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取
- **认证**：环境变量 `FEISHU_APP_ID` + `FEISHU_APP_SECRET`，或 `~/.feishu-cli/config.yaml`
- **权限**：`board:whiteboard`（画板读写）+ `docx:document`（文档中添加画板）
- **验证**：`feishu-cli auth status` 确认认证正常

## 工作流程（3 步）

```
Step 1: 路由选择
  判断渲染路径：Mermaid 还是 OpenAPI JSON？

Step 2: 生成节点 JSON 或 Mermaid 代码
  - JSON 路径：参考 schema.md 构建节点数组 → layout.md 计算坐标 → style.md 上色
  - Mermaid 路径：直接生成 .mmd 内容

Step 3: 创建/更新并验证
  - 创建：board create-notes（先形状，再连线）
  - 验证：board image 截图检查
  - 有问题：调整坐标/样式后重新创建
```

### Step 1: 路由选择

| 图表类型 | 路径 | 理由 |
|----------|------|------|
| 思维导图 | **Mermaid** | 辐射结构自动布局 |
| 时序图 | **Mermaid** | 参与方+消息自动排列 |
| 类图 | **Mermaid** | 类关系自动布局 |
| 饼图 | **Mermaid** | Mermaid 原生支持 |
| 流程图 | **Mermaid** | 自动排版稳定 |
| 架构图 | **OpenAPI JSON** | 需要精确分层布局和配色 |
| 组织架构图 | **OpenAPI JSON** | 树形结构需精确坐标控制 |
| 对比图/矩阵 | **OpenAPI JSON** | 网格对齐需绝对定位 |
| 鱼骨图 | **OpenAPI JSON** | 自定义角度和分支 |
| 柱状图/折线图 | **OpenAPI JSON** | 需要坐标轴计算 |
| 其他自定义图表 | **OpenAPI JSON** | 精确控制样式和布局 |

**路由规则**：
1. 思维导图、时序图、类图、饼图、流程图 → 默认走 Mermaid（`board import`）
2. 用户输入包含 Mermaid 语法 → 走 Mermaid
3. 其他所有类型 → 走 OpenAPI JSON（`board create-notes`）

### Step 2: 生成节点 JSON / Mermaid

**JSON 路径**（大多数场景）：

1. 按 `references/content.md` 规划信息量和分组
2. 按 `references/layout.md` 选择布局模式，计算每个节点的 x/y 坐标
3. 按 `references/style.md` 上色（未指定时用经典色板）
4. 按 `references/schema.md` 语法输出完整 JSON
5. 连线参考 `references/connectors.md`，排版参考 `references/typography.md`

**Mermaid 路径**：

```bash
# 从内容导入
feishu-cli board import <whiteboard_id> --source-type content \
  -c "graph TD; A-->B-->C" --syntax mermaid

# 从文件导入
feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid
```

#### 限流处理

- 导入图表时遇到 HTTP 429 自动重试，采用指数退避 + 读取服务端 `x-ogw-ratelimit-reset` 响应头精确计算退避时间
- 大批量图表导入（如文档中包含 100+ 图表）时，CLI 自动控制并发和重试，默认最多重试 20 次
- verbose 模式下可观察重试日志，便于排查限流问题

### Step 3: 创建/更新并验证

```bash
# 1. 创建文档和画板
DOC_ID=$(feishu-cli doc create --title "架构图" -o json | python3 -c "import sys,json;print(json.load(sys.stdin)['document_id'])")
BOARD_ID=$(feishu-cli doc add-board $DOC_ID -o json | python3 -c "import sys,json;print(json.load(sys.stdin)['whiteboard_id'])")

# 2. 创建形状节点（先形状）
feishu-cli board create-notes $BOARD_ID shapes.json -o json
# → 返回 node_ids: ["o1:1", "o1:2", ...]

# 3. 创建连接线（引用上面的 ID）
feishu-cli board create-notes $BOARD_ID connectors.json -o json

# 4. 截图验证
feishu-cli board image $BOARD_ID output.png
```

**视觉审查**：检查信息完整、布局合理、配色协调、文字无截断、连线无交叉。有问题按症状表修复后重新创建。

## 命令速查

| 命令 | 说明 | 示例 |
|------|------|------|
| `board create-notes` | 批量创建节点（形状+连线） | `feishu-cli board create-notes <id> nodes.json -o json` |
| `board import` | 导入 Mermaid/PlantUML（429 自动重试） | `feishu-cli board import <id> diagram.mmd --syntax mermaid` |
| `board image` | 下载画板为 PNG | `feishu-cli board image <id> output.png` |
| `board nodes` | 获取画板所有节点 | `feishu-cli board nodes <id>` |
| `doc add-board` | 在文档中添加空画板 | `feishu-cli doc add-board <doc_id> -o json` |

**传参方式**：

```bash
# 从 JSON 文件（推荐）
feishu-cli board create-notes <id> nodes.json -o json

# 内联 JSON（简单场景）
feishu-cli board create-notes <id> '<json_array>' --source-type content -o json
```

## 关键约束速查表

1. **先形状后连线** -- 连接线通过 ID 引用形状节点，必须在形状创建后再创建连线
2. **最小字段集** -- 多余字段导致 `2890002 invalid arg`，只用 schema.md 列出的安全字段
3. **背景色块 fill_opacity <= 60** -- 否则完全遮挡上层节点
4. **z_index 分层** -- 背景 0-1、次级 2-3、常规节点 10、连线 50
5. **坐标系为绝对坐标** -- 没有自动布局引擎，每个节点必须手算 x/y/width/height
6. **节点文字保持简短** -- 标题 + 简短说明（12 字以内），不写长段落
7. **同组节点视觉一致** -- 同一分组内所有节点使用相同的 fillColor/borderColor

## 症状 → 修复表

| 看到的问题 | 改什么 |
|-----------|--------|
| 文字被截断/溢出 | 增大 width 或 height，或缩短文字 |
| 节点重叠粘连 | 增大节点间距（同层 >= 30px，有连线 >= 60px） |
| 背景色块遮挡节点 | 降低 fill_opacity（<= 25），确认 z_index 分层正确 |
| 连线穿过节点 | 调整 snap_to 方向或增大间距 |
| 布局整体偏左/偏右 | 调整 x 坐标使内容居中 |
| 大面积空白 | 缩小画布宽度，减小节点间距 |
| 文字和背景色太接近 | 调整 fill_color 或 text.font_color，确保对比度 |
| 分组看不出来 | 为每个分组使用不同颜色（见 style.md 色板） |
| 连线箭头挤在缝里 | 有连线的节点间距 >= 60px |
| 2890002 invalid arg | 检查是否包含多余字段（id/locked/children 等只读字段） |

## 渲染前自查

生成 JSON 后、创建节点前，快速检查：

- [ ] 不同分组用了不同颜色？同组节点样式完全一致？
- [ ] 外层浅色背景（低 opacity）、内层节点实心填充？
- [ ] 所有节点有边框（border_style=solid, border_width=medium）？
- [ ] 连线用灰色（#BBBFC4 或 #646A73），不用彩色？
- [ ] 背景色块 z_index=0-1 且 fill_opacity <= 25？
- [ ] 节点间距充足（同层 >= 30px，有连线 >= 60px）？
- [ ] JSON 中没有多余字段（id/locked/children 等）？

## board update — 更新画板内容

更新画板节点内容，支持从文件或 stdin 读取节点 JSON。

```bash
feishu-cli board update <whiteboard_id> [nodes_json_file] [--overwrite] [--dry-run] [--stdin]
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `<whiteboard_id>` | 画板 ID | 必填 |
| `[nodes_json_file]` | 节点 JSON 文件路径 | 与 `--stdin` 二选一 |
| `--stdin` | 从标准输入读取节点 JSON | 否 |
| `--overwrite` | 覆盖模式（先写后删，保证不会出现空画板） | 否 |
| `--dry-run` | 仅预览，输出将要删除的节点数量，不实际执行（需配合 `--overwrite`） | 否 |
| `-o json` | JSON 格式输出 | 否 |

### 使用示例

```bash
# 从文件更新（追加模式，保留旧节点）
feishu-cli board update BOARD_ID nodes.json

# 覆盖模式：先创建新节点，再删除旧节点
feishu-cli board update BOARD_ID nodes.json --overwrite

# 从 stdin 管道更新（覆盖模式）
cat nodes.json | feishu-cli board update BOARD_ID --stdin --overwrite

# 预览覆盖操作（不实际执行）
feishu-cli board update BOARD_ID nodes.json --overwrite --dry-run

# JSON 格式输出（返回新节点 ID 列表）
feishu-cli board update BOARD_ID nodes.json --overwrite -o json
```

### 覆盖模式流程

1. 获取当前画板所有旧节点 ID
2. 创建新节点
3. 删除不在新节点中的旧节点（先写后删，避免空画板）

## board delete — 删除画板节点

批量删除画板中的节点，支持指定节点 ID 或清空整个画板。

```bash
feishu-cli board delete <whiteboard_id> [--node-ids id1,id2] [--all]
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `<whiteboard_id>` | 画板 ID | 必填 |
| `--node-ids` | 要删除的节点 ID（逗号分隔） | 与 `--all` 二选一 |
| `--all` | 删除所有节点（清空画板） | 否 |
| `-o json` | JSON 格式输出 | 否 |

### 使用示例

```bash
# 删除指定节点
feishu-cli board delete BOARD_ID --node-ids o1:1,o1:2,o1:3

# 删除所有节点（清空画板）
feishu-cli board delete BOARD_ID --all

# JSON 格式输出
feishu-cli board delete BOARD_ID --all -o json
```

## 参考文档索引

| 文件 | 说明 | 何时读 |
|------|------|--------|
| `references/schema.md` | 节点类型、属性、JSON 格式 | JSON 路径必读 |
| `references/layout.md` | 布局策略、坐标计算、间距规则 | JSON 路径必读 |
| `references/style.md` | 5 套预设色板、上色步骤 | JSON 路径必读 |
| `references/connectors.md` | 连线策略、方向、形状选择 | 有连线时读 |
| `references/typography.md` | 字号层级、对齐规则 | 需要排版时读 |
| `references/content.md` | 信息量规划、分组规则 | 规划阶段读 |
| `references/node-api.md` | API 端点、错误码、Redraw 流程 | 排障/高级操作时读 |
