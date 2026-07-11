# feishu-cli-visual - 飞书可视化设计系统

一张图表是**给人读的，由你来执行的**。本技能把"画得好看"变成一套带检查的流程：
结果靠构造保证正确，而不是靠品味。核心习惯只有一个：

> **配色是可计算的，就用计算。** 永远不要用眼睛判断色板是否色盲安全 ——
> 跑 `scripts/validate_palette.js`。

## 定位：方法层 + 参数层

- **方法层**（本 SKILL + references）：选形式 → 选管线 → 按职能配色 → 校验 → 标记规格 → 渲染自检。方法与载体无关。
- **参数层**（`references/palette.md`）：飞书色系派生的统一色板实例 —— categorical 8 色（light/dark）、sequential 蓝 ramp、diverging 对、status 四色、中性 chrome 色，全部在飞书明暗底色上通过校验。四条渲染管线共用这一份参数，不再各自维护配色。

## 流程 —— 按顺序执行，配色永远最后定

1. **选形式**。数据的任务是什么 —— 比较量级、区分系列、体现极性、还是只有一个数字？
   任务决定形式，有时答案根本不是图表（用大数字/指标行更诚实）。→ `references/choosing-a-form.md`
2. **选管线**。见下方路由表。形式 + 载体共同决定走哪条管线。
3. **按职能配色**。categorical（身份）/ ordinal（有序档位）/ sequential（量级）/
   diverging（极性）/ status（状态），每种职能一条规则，色值一律取自
   `references/palette.md`，**不自创色值、不循环生成第 9 个色**。
4. **校验色板 —— 跑脚本，不要目测**（`scripts/` 相对本技能根目录）：
   ```bash
   node scripts/validate_palette.js
   node scripts/validate_palette.js "#3370ff,#d99904,..." --mode light
   node scripts/validate_palette.js "..." --mode dark --surface "#1f1f1f"
   ```
   无参数会复验 `palette.json` 的 light/dark categorical、circular、all-pairs、
   HTMLBox 深蓝画布和 light/dark ordinal 定稿组合。
   文档一致性检查识别 `#RGB/#RGBA/#RRGGBB/#RRGGBBAA`：`#RGB` 展开后匹配
   canonical；带 alpha 的 4/8 位色必须在 `palette.json.docHexExceptions` 按文件、
   按原值精确登记，禁止静默丢弃 alpha 后放行。
   **原样按序取用 `palette.md` 定稿色值时无需重跑**（定稿结论已写在该文件里）；
   改色值、换底色、或走 all-pairs 场景时才需要。FAIL 必须修掉才能继续；
   WARN（色盲 ΔE 8–12 地板区、对比度不足 3:1 救济区）只在配齐二级编码
   （直接标签 / 留白间隔 / 表格视图）时才合法。
   饼图/环图加 `--pairs circular`（首尾也相邻）；散点/气泡/地图加 `--pairs all` —— 该场景**不用全 8 色**，用 `palette.md`
   的 all-pairs 安全子集（4 色）；单色 ramp 加 `--ordinal`。
5. **应用标记规格与自检**。细标记（bar ≤ 24px、线 2px）、相邻填充留 2px 底色间隔、
   网格用发丝线实线、文字永远穿文字色（系列身份由旁边的色块承载，不给文字上系列色）。
   完成后对照 `references/anti-patterns.md` 逐条检查，命中即错；最后渲染出来亲眼看一遍
   （htmlbox 用 `../htmlbox/scripts/verify.sh` 截图、board 用 svg-export、card 实际发送预览）。

## 管线路由

| 需求特征 | 管线 | 技能 |
|---|---|---|
| 要"动"：交互 tooltip、动画、实时刷新、3D、复杂 dashboard | HTML 小组件（ECharts 等） | `feishu-cli-visual` |
| 图表出现在**消息/通知卡片**里 | 卡片 chart 组件（VChart） | `feishu-cli-messaging` |
| 图表是**文档的一部分**，随 Markdown 批量导入，静态即可 | Mermaid → 画板 | `feishu-cli-docs` |
| 单独一张**画板**：自由视觉/架构图/信息图，节点可编辑 | SVG → 原生节点 等 5 路径 | `feishu-cli-visual` |
| 只有一两个数字 + 状态 | 不画图：卡片大数字/fields，文档 Callout | `feishu-cli-messaging` / `feishu-cli-docs` |

各管线可控度不同，规范落地深度也不同：
htmlbox（全控：色板 + 标记 + 交互 + 浏览器内自动校验）＞ board SVG（色板 + 标记 + 排版；
**渲染后无 tooltip 兜底，直接标签要比默认更充分**）＞ card VChart（color 数组 + 形式选择）
＞ import Mermaid（classDef 配色 + 形式选择）。

## 非协商规则（四条管线一体适用）

> 完整验收清单以 `references/anti-patterns.md` 为准，本节是其中最高频规则的速览；
> 两处如有出入，以 anti-patterns.md 为权威。

- **单轴**。永远不画双 Y 轴图（两个量纲 → 拆两张图、小倍数、或统一基期指数化）。
- **categorical 色按固定顺序取用，永不循环**。第 9 个系列不是新颜色，
  是折叠进"其他"、拆小倍数、或复合编码（色相 × 形状）。散点/气泡/地图
  （任意两色可相邻）用 all-pairs 安全子集（≤4 系列），超出同样走折叠/小倍数。
- **颜色跟随实体，不跟随排名**。过滤掉一个系列后，剩下的系列不重新上色。
- **sequential = 单色相浅→深；diverging = 两个冷暖对立色 + 中性灰中点**。
  永远不用彩虹 ramp；diverging 中点永远不带色相。
- **status 四色是保留色**（good/warning/serious/critical），不拿来当"第 4 个系列"；
  出现时必须配图标 + 文字，不靠颜色单独传义。
- **文字穿文字色**。数值、标签、图例文字用中性 ink，不穿系列色；
  身份由文字旁的色块/短线承载。
- **≥ 2 系列必有图例**（单系列不要图例盒，标题已说明）；直接标签只标端点/极值/主角系列，
  **绝不逐点标数**。

## 文件索引

| 文件 | 回答什么问题 |
|---|---|
| `references/palette.md` | **统一色板实例** —— 所有色值的唯一出处（人读版），含各管线取用方式 |
| `references/choosing-a-form.md` | 选哪种图表形式？是不是根本不该画图？ |
| `references/anti-patterns.md` | **验收清单** —— 出手前逐条对照，命中即错 |
| `scripts/validate_palette.js` | 可运行校验器（Node CLI + 浏览器 data-palette 双模式；可 require 复用其函数） |
| `scripts/palette.json` | 色板机器可读版（含废弃色清单），改色时与 palette.md 一起改 |
| `scripts/check_docs.js` | 文档一致性检查：扫描全部技能文档的废弃色残留 + palette.md 完整性 |
