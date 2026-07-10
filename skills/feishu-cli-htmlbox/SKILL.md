---
name: feishu-cli-htmlbox
description: >-
  在飞书云文档里画**会动的图 / 可交互图表 / 数据大屏**——妙笔BOX 是飞书文档里唯一能真实跑 CSS/JS 的载体
  （iframe 沙箱）。能画：ECharts 全家桶（折线/柱/饼/雷达/散点/热力/桑基/漏斗/仪表/K线/箱线/平行坐标/旭日/treemap/
  力导向关系图/时序/甘特）、真实地图与经纬度飞线、echarts-gl 3D（map3D/3D柱/3D散点/3D曲面）、Three.js 真 3D 场景、
  词云、水球、纯 CSS 动画、Canvas 粒子、SVG 矢量动画、KPI 滚动大屏。
  当用户要"在飞书文档里画图/做动画/能动的图/可交互图表/数据大屏/Dashboard/折线图/柱状图/地图/飞线图/3D图/
  关系图/流程动画/ECharts/可视化"，或要做**能调 AI / 读写多维表 / 持久化状态 / 拿用户身份的交互式文档小程序**，
  或提到"妙笔BOX/HTML 小组件/让飞书文档里的图动起来/嵌入网页到飞书文档/window.magic"时，
  **必须用本技能**。注意：要"动"只能用妙笔BOX；画板（feishu-cli-board）的 SVG 节点会被服务端栅格化成静态图、不会动。
argument-hint: <document_id> [block_id]
user-invocable: true
allowed-tools: Bash(feishu-cli doc:*), Bash(feishu-cli perm:*), Bash(feishu-cli msg:*), Bash(agent-browser:*), Bash(playwright-cli:*), Read, Write
---

# 在飞书文档里画会动的图（妙笔BOX）

你拿到「在飞书文档里画一个 X 图 / 做个能动的可视化 / 做数据大屏」这类任务时用本技能。
妙笔BOX 是飞书文档里**唯一能真实执行 CSS/JS 的载体**（iframe 沙箱），所以一切动画、ECharts、Three.js、Canvas
都能动。流程就三件事：**照配方写一页自包含 HTML → 本地浏览器验证它真渲染 → 用 `doc htmlbox` 落库**。

> 要"动"只能用妙笔BOX。画板（`feishu-cli-board`）的 SVG 节点会被飞书服务端**栅格化成静态图、永远不动**；
> 代价是妙笔BOX 是整体 iframe，内部元素不能像画板节点那样在飞书里单独点选编辑（鱼与熊掌不可兼得）。

## 第一步永远是选型：你要画什么 → 用什么 → 配方在哪

先在下表对号入座，再去对应 reference 抄骨架 + 配方。**不要从零手搓 ECharts option**——配方都验证过、能直接 `create`。

| 你要画的图 | 引擎 / 方案 | 配方位置 |
|---|---|---|
| 折线 / 面积 / 柱状 / 堆叠柱 / 柱状竞赛 BarRace / 玫瑰饼 / 雷达 | ECharts（换 `series.type`） | `references/gallery.md` › 数据对比 |
| 散点气泡 / 热力 / 日历热力 / 箱线 / K线蜡烛 / 平行坐标 | ECharts | `references/gallery.md` › 分布统计 |
| 饼 / 漏斗 / 桑基 Sankey / 主题河流 / 仪表盘 gauge / 水球 liquidFill | ECharts(+扩展) | `references/gallery.md` › 构成流向 |
| 力导向关系图（可拖拽）/ 组织树 tree / 旭日 sunburst / 矩形树图 treemap | ECharts | `references/gallery.md` › 关系层级 |
| 时序 / 状态机 / 甘特 / CI流水线 / 看板流动 | ECharts custom / CSS | `references/gallery.md` › 流程时序 |
| 拓扑 / Agent 编排动画 / 多 Agent 协作回放 | `scripts/animate_diagram.py`（结构化 JSON → 自包含 SVG 动画 HTML） | `references/animated-flowchart.md` |
| 词云 wordCloud | echarts-wordcloud | `references/gallery.md` › 构成流向 |
| 纯 CSS 动画（旋转/脉动/进度条/打字机/变色） | CSS `@keyframes`（最稳，不依赖外网） | `references/gallery.md` › 创意动画 |
| 粒子流 / 星空 / 自绘动画 | Canvas + `requestAnimationFrame` | `references/gallery.md` › 创意动画 |
| 矢量路径自绘 / morphing / 维恩图 | 内联 `<svg>` + SMIL/CSS | `references/gallery.md` › 创意动画 |
| KPI 数据大屏（数字滚动 + 迷你趋势） | HTML/CSS/JS | `references/gallery.md` › 创意动画 |
| **真实地图着色 choropleth / 经纬度 geo 飞线** | ECharts geo + `registerMap` | `references/geo-3d.md` |
| **3D 立体地图 map3D / 3D 柱 / 3D 散点 / 3D 曲面** | echarts-gl | `references/geo-3d.md` |
| **真 3D 场景（旋转星球 / GPU 粒子 / 几何体）** | Three.js（WebGL） | `references/geo-3d.md` |

不确定用哪个？**默认 ECharts**（覆盖绝大多数统计/关系图）；只有要真实地理、可旋转 3D、或 ECharts 给不出的自由视觉，才上 geo-3d / Canvas / SVG。

**不确定画什么形式**（柱还是线？要不要干脆一个大数字？）→ 先看 `feishu-cli-dataviz` 技能
（任务→形式启发式 + 反模式清单）。**系列配色**统一取该技能色板 dark 列（约定见
`references/gallery.md` 开头），不自创色值。

## 不止画图：window.magic 文档小程序运行时

妙笔BOX 的 iframe 里，飞书注入了一个 `window.magic` 运行时——**OpenAPI 建的块也有**（已实测，注入认 `component_type_id` 不认来源）。所以它不只是画图，还能做**会读数据、能交互、有状态的文档小程序**。当任务需要下面任一项，看 `references/window-magic.md`：

| 需求 | 能力 |
|---|---|
| 当前用户身份（名字 / open_id，用于个性化或写人员字段） | `window.magic.user` |
| 读当前文档全文 / 元信息（喂 AI、统计、生成目录） | `getDocAsMarkdown` / `getPageMeta` |
| 持久化状态（计数器 / 抽奖 / 祝福墙 / 已读列表） | `window.magic.redis` / `store`（**禁 localStorage**，见 pitfalls） |
| 图表接真实数据源（活数据 Dashboard） | `base_records_search` 拉多维表 → ECharts |
| 文档内调 AI（摘要 / 问答 / 文案，无需自带 key） | `window.magic.ai`（豆包） |

⚠ `window.magic` **只在飞书文档端注入**，本地 `file://` 预览必为 `undefined`——所有用法都要判存兜底，否则本地白屏。

## 绘制工作流（每张图都照做）

1. **照配方写一页自包含 HTML 到 `/tmp/x.html`**。从 `gallery.md` / `geo-3d.md` 取对应骨架 + 配方；统一深色背景、容器固定高度（如 `#chart{height:360px}`）、`width:100%`、监听 `resize` 调 `chart.resize()`。系列色按 `gallery.md` 开头的配色约定取用；换了色就先跑一遍色板校验（命令在同一段）。
2. **本地浏览器验证它真渲染、真在动——这步不能跳**。iframe 里任何顶层 JS 错误会让整张图**白屏且不报错**（未捕获异常走 pageerror、不进 console），光读代码看不出来。直接跑封装好的验证脚本——它用全新浏览器 session 打开页面、抓 page error / console、数 canvas/svg 节点、截图，并据此给通过/未过判定：
   ```bash
   scripts/verify.sh /tmp/x.html        # 地图/CDN 重的加等待秒数：scripts/verify.sh /tmp/x.html 5
   ```
   退出码 0 才算初步通过；但「画对没画对、在不在动」机器判不了，**务必再肉眼看它打印的那张截图**。排查白屏见 `references/pitfalls.md`。
3. **落库**：`feishu-cli doc htmlbox create <doc_id> --html-file /tmp/x.html`。
4. **改图**：`feishu-cli doc htmlbox update <doc_id> <block_id> --html-file /tmp/x2.html`（block_id 会变，后续用返回的 `new_block_id`）。

## 命令速记

| 操作 | 命令 |
|---|---|
| 插入 | `feishu-cli doc htmlbox create <doc_id> --html-file x.html`（`--index` 控位置，默认末尾） |
| 替换 | `feishu-cli doc htmlbox update <doc_id> <block_id> --html-file x2.html`（返回 `new_block_id`） |
| 读回 | `feishu-cli doc htmlbox get <doc_id> <block_id> --raw > cur.html` |
| 删除 | `feishu-cli doc htmlbox delete <doc_id> <block_id>` |

输入也支持 `--html '<...>'` 或 `--html-file -`（stdin）。四个命令都支持 `--format` / `--jq`；写类 create/update/delete 另有 `--dry-run` 预览和 `-o` 写文件，get 用 `--raw` 出纯 HTML（get 无 `--dry-run`/`-o`）。默认 **Bot 身份**（操作 feishu-cli 自建文档无需登录）；改他人/手建文档传 `--user-access-token`。

## 画图必避的 5 个坑（详见 `references/pitfalls.md`）

1. **白屏不报错** → 落库前本地浏览器必验（见工作流第 2 步）。这是最高频的坑。
2. **CDN `<script>` 是异步的** → 用前轮询等库就绪（`if(typeof echarts==='undefined') return setTimeout(boot,150)`），每个 `<script src>` 加 `onerror` 兜底。飞书沙箱实测能联 jsdelivr。
3. **真实地图** → ECharts 5 不带地图数据，直接 `type:'map'` 空白；先 `fetch` GeoJSON（借 `echarts@4.9.0/map/json/china.json`）再 `registerMap`。
4. **极坐标 polar bar 不按值上色** → `visualMap` 对它无效，给每个 data 项显式 `itemStyle.color`。
5. **批量画多张** → 每次 `create` 之间 `sleep 0.5s`，否则触发飞书写限流 `99991400`；多图配标题用 `doc content-update --mode append --markdown "## 标题"` 与 create 交替追加。

## 创建后交付（按需）

文档要交给用户时：按 `feishu-cli-write` 的 owner 授权流程（`perm add full_access` + 视配置 `perm transfer-owner`），并按全局规则发一张飞书卡片通知。

## 参考文档与脚本

- `scripts/verify.sh <html> [等待秒数]` — **落库前验证脚本**（工作流第 2 步用它）：全新 session 打开 → 抓 page error/console → 数 canvas/svg → 截图 → 给通过判定；退出码 0 才算初步通过，仍须肉眼看截图
- `scripts/animate_diagram.py --pattern pattern.json --out x.html` — 拓扑 / Agent 编排动画生成器：结构化 JSON → 自包含 SVG 动画 HTML
- `references/gallery.md` — **主力配方库**：4 种通用骨架（ECharts/Canvas/Three.js/SVG-CSS）+ 按图表类型的可直接用配方
- `references/animated-flowchart.md` — 拓扑 / Agent 编排动画 recipe，配套输入格式见 `references/pattern-schema.md`
- `references/geo-3d.md` — 地图 / echarts-gl 3D / Three.js 的完整可跑模板（这几类有 CDN/registerMap/坐标系/着色坑）
- `references/window-magic.md` — **文档小程序运行时**：`window.magic` 能力配方（用户身份 / 读文档 / 持久化 / 多维表 / AI），含判存兜底范式与活数据 Dashboard、文档内 AI 卡等组合配方
- `references/pitfalls.md` — 画图避坑与白屏排查（来自真实创建一篇 47 图大文档）
- `references/mechanism.md` — 块机制 / 身份 / update 速查（只在排查 token、组件 ID、update 行为时才需要）
