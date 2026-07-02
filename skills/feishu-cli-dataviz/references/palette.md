# 统一色板 —— 飞书可视化参数实例

这是设计系统的**参数层**：方法（流程、检查、标记规格）与载体无关，本文件提供
飞书场景下每个参数的具体取值。所有色值由飞书开放平台色系家族（blue / turquoise /
yellow / green / purple / red / carmine / orange）在 OKLCH 空间派生，并在飞书
明暗底色上通过 `scripts/validate_palette.js` 全部检查。

机器可读版在 `scripts/palette.json`（含废弃色清单）。**改色流程**：改 palette.json
与本文件 → 重跑校验器 → 跑 `node scripts/check_docs.js` 同步核查所有技能文档
（扫描废弃色残留 + 本文件完整性），全绿才算改完。原样取用本文件色值时无需重跑校验。

## Categorical 色板（系列身份）

8 个 slot，**顺序固定，按序取用，永不循环**。slot 顺序不是审美选择，而是色盲安全
机制 —— 由枚举全部排列、最大化"最小相邻色盲 ΔE"得出。dark 列不是独立色板，
是同 8 个色相针对暗色底面重新定步。

| slot | 家族 | light | 白底对比度 | dark | 暗底对比度 |
|---|---|---|---|---|---|
| 1 | blue | `#3370ff` | 4.28 | `#3370ff` | 3.85 |
| 2 | yellow | `#d99904` | 2.47 ⚠ | `#bf8600` | 5.20 |
| 3 | turquoise | `#04b49c` | 2.62 ⚠ | `#00ad96` | 5.82 |
| 4 | orange | `#ed6d0c` | 3.09 | `#ea6a03` | 5.15 |
| 5 | purple | `#7f3bf5` | 5.46 | `#7f3bf5` | 3.02 |
| 6 | red | `#f54a45` | 3.53 | `#f54a45` | 4.67 |
| 7 | carmine | `#f14ba9` | 3.32 | `#f04aa8` | 4.91 |
| 8 | green | `#2ea121` | 3.37 | `#2ea121` | 4.89 |

校验结论（2026-07 定稿）：
- light 最差相邻色盲 ΔE = **53.8**（protan），dark = **52.9** —— 远超 ≥ 12 目标。
- ⚠ light 模式 slot 2/3 对白底不足 3:1：**救济规则生效** —— 用到这两个色时必须
  配可见数值标签或表格视图，这是义务不是建议。
- dark 模式 8 色对 `#1f1f1f` 全部 ≥ 3:1。

一键复验（`scripts/` 相对本技能根目录；在其他位置调用时先定位：
`VP=~/.claude/skills/feishu-cli-dataviz/scripts/validate_palette.js`，仓库内开发则为
`skills/feishu-cli-dataviz/scripts/validate_palette.js`）：

```bash
node scripts/validate_palette.js "#3370ff,#d99904,#04b49c,#ed6d0c,#7f3bf5,#f54a45,#f14ba9,#2ea121" --mode light
node scripts/validate_palette.js "#3370ff,#bf8600,#00ad96,#ea6a03,#7f3bf5,#f54a45,#f04aa8,#2ea121" --mode dark --surface "#1f1f1f"
```

### 散点/气泡/地图：all-pairs 安全子集

以上校验按"相邻对"衡量（柱/线/堆叠只有相邻系列接触）。散点、气泡、地图、
小倍数里**任意两色都可能相邻**（校验加 `--pairs all`），全 8 色在该标准下不通过
—— 这不是缺陷，而是任何 8 色板的物理极限。此类图表改用 all-pairs 安全子集
（任意两色 CVD ΔE ≥ 12，两模式同一组 slot）：

| 模式 | 子集（slot 1/2/3/6，按此序取用） | 最差全对 ΔE |
|---|---|---|
| light | `#3370ff` `#d99904` `#04b49c` `#f54a45` | 24.3 |
| dark | `#3370ff` `#bf8600` `#00ad96` `#f54a45` | 17.0 |

超过 4 个系列的散点：拆小倍数、复合编码（色相 × 形状）、或折叠"其他"——
**不要**回退到全 8 色硬画。

## Sequential 蓝 ramp（连续量级：热力图、地图着色、深浅编码）

单色相（飞书蓝），浅→深 13 步：

| 步 | hex | 步 | hex | 步 | hex | 步 | hex |
|---|---|---|---|---|---|---|---|
| 100 | `#cedfff` | 250 | `#8ab1ff` | 400 | `#4b80f5` | 550 | `#2b57bd` |
| 150 | `#b7d0ff` | 300 | `#73a1ff` | 450 | `#3e72e5` | 600 | `#244ca5` |
| 200 | `#a0c0ff` | 350 | `#5c90ff` | 500 | `#3364d2` | 650 | `#1f418c` |
| | | | | | | 700 | `#1c3672` |

- **sequential 用法**（连续量级）：全程 100→700，最浅步表示"接近零"，允许贴近底色。
- **ordinal 用法**（离散有序档位：漏斗阶段/等级/年龄段，隔步取 4–6 个）：贴近底色的
  一端必须 ≥ 2:1 —— light 模式起步不浅于 **250**（`#8ab1ff`，2.14:1）；dark 模式
  止步不深于 **600**（`#244ca5`，对暗底 2.08:1）。校验加 `--ordinal`。
- 同一视图出现第二个 sequential 语境时，第二个用 turquoise 家族自建单色 ramp，
  不与蓝混排成彩虹。

## Diverging 对（极性：高于/低于基线、正负偏差）

**blue ↔ orange**（冷暖对立，全色盲类型下都可分），两臂步数相等，中点必须是
中性灰不带色相：light `#f2f3f5`，dark `#373a3d`。
两极直接取 categorical slot 1/4 的同家族深浅步（如 `#2b57bd` ↔ `#893b00` 到
中性灰渐变）。不用 red 做极——红在飞书语境强绑"错误/危险"，中性极性对比会被误读。

## Status 四色（状态语义，保留色，永不当系列色用）

| 角色 | hex | 白底对比度 | 暗底对比度 |
|---|---|---|---|
| good | `#217a12` | 5.44 | 3.03 |
| warning | `#b97d00` | 3.50 | 4.71 |
| serious | `#c9571c` | 4.32 | 3.82 |
| critical | `#c02622` | 5.93 | 2.78 ⚠ |

- 明暗两模式共用同一组步（critical 对暗底 2.78 略低于 3:1 —— 状态色本就强制
  **图标 + 文字**并行传义，这是既定救济）。
- 四色的亮度步与 categorical slot 刻意错开（good ≠ slot 8 green、critical ≠
  slot 6 red……），状态色永远不会被误读成某个系列。
- 判据：当一个系列**本身表示**好/坏（错误率、通过/失败）时穿 status 色；
  它只是"第 4 个系列"时穿 categorical —— 一张图里永不混用两种语义。

## 浅底/深边派生对（节点填充场景）

Mermaid classDef、board 分组背景、流程图节点等"浅色底 + 深色边框/文字"场景，
按 slot 家族取派生对（fill 为 L≈0.95 淡底，stroke 为 L≈0.45 深边，文字色 = stroke）：

| 家族 | fill（淡底） | stroke（深边/文字） |
|---|---|---|
| blue | `#e6efff` | `#1446c2` |
| yellow | `#fdecd1` | `#714e00` |
| turquoise | `#d3f8ef` | `#006456` |
| orange | `#ffeadf` | `#893b00` |
| purple | `#efecff` | `#611dc5` |
| red | `#ffe9e6` | `#a30011` |
| carmine | `#ffe7f1` | `#990064` |
| green | `#e0f6dd` | `#0c6800` |

8 个家族的 stroke 对白底对比度全部 ≥ 7:1，可直接作节点文字色。

另有两组**中性角色对**（取自下方 chrome 中性阶，供"前端/用户"与"外部系统"类节点用）：

| 角色 | fill | stroke | 语义 |
|---|---|---|---|
| 中性（fe） | `#f2f3f5` | `#646a73` | 前端/用户等本系统的中性部分 |
| 外部（ext） | `#ffffff` | `#8f959e` | 第三方/外部系统，白底灰边读作"空心" |

## Chrome 与 ink（图表家具：文字/网格/轴/底色）

| 角色 | light | dark |
|---|---|---|
| 图表底色 surface | `#ffffff` | `#1f1f1f` |
| 主文字 primary ink | `#1f2329` | `#ebebeb` |
| 次文字 secondary ink | `#646a73` | `#a6a6a6` |
| 弱文字/轴标签 muted | `#8f959e` | `#8f959e` |
| 网格线（发丝线，实线） | `#dee0e3` | `#383838` |
| 基线/轴/连线灰 | `#bbbfc4` | `#4a4a4a` |
| 卡片/分区淡底 | `#f2f3f5` | `#2a2a2a` |

中性色取自飞书通用中性色阶；dark surface `#1f1f1f` 为飞书暗色文档底色近似值，
若实际渲染底色不同（如 htmlbox 自设画布色），**用实际底色重跑校验**：
`--surface <实际色>`。

## 语义方向色（delta / 涨跌）

"涨/跌、好/坏"是方向语义，不是系列身份，也不占用 status 四色的告警语义：

- **业务指标 delta**（DAU +12% 这类"涨=好"）：白底用 status 的 good `#217a12` /
  critical `#c02622`；深蓝画布（htmlbox）用 `#00ad96`（升）/ `#f54a45`（降），
  亮度更适配暗底。
- **金融 K 线**遵循市场惯例红涨青跌：`#f54a45`（阳线）/ `#00ad96`（阴线）。
- 同一张图里这两个色承担方向语义时，多系列取色跳过 slot 3/6，避免"身份"与
  "方向"双语义撞色。

## 各管线取用方式

### htmlbox（妙笔BOX，深蓝画布）

**该管线的权威约定在 `feishu-cli-htmlbox/references/gallery.md` 开头**：画布固定
深蓝 `#0f1729`（管线视觉身份，不随系统明暗切换），系列色取 **dark 列**按序截取，
单系列主色用 `#00ad96`（画布与 slot 1 蓝同色相，蓝让位）。dark 列已在 `#0f1729`
上单独校验通过（最差相邻 ΔE 52.9，对比度全 ≥ 3:1）。本文件不另立 htmlbox 默认，
以免与配方库冲突。

### 独立 HTML 页面 / apps 部署（明暗自适应）

不在妙笔BOX 里、需要跟随系统明暗的独立页面（如 `feishu-cli-apps` 部署的站点），
用 CSS custom properties 一处切换：

```html
<style>
  :root {
    --surface: #ffffff; --ink-1: #1f2329; --ink-2: #646a73;
    --grid: #dee0e3;
    --s1: #3370ff; --s2: #d99904; --s3: #04b49c; --s4: #ed6d0c;
    --s5: #7f3bf5; --s6: #f54a45; --s7: #f14ba9; --s8: #2ea121;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --surface: #1f1f1f; --ink-1: #ebebeb; --ink-2: #a6a6a6;
      --grid: #383838;
      --s2: #bf8600; --s3: #00ad96; --s4: #ea6a03; --s7: #f04aa8;
    }
  }
</style>
```

ECharts `color` 数组按所处模式取对应列、按序截取。如需在测试页里跑浏览器版
校验器：把 `validate_palette.js` 复制到 HTML 同目录，`<body>` 挂
`data-palette`/`data-mode`/`data-surface` 后引入即可（script 的 src 相对 HTML
文件解析，不要写跨目录相对路径）。

### card（VChart 图表组件）

- 图表 spec 注入 `"color": ["#3370ff","#d99904",...]`（light 列）。多系列永远按
  固定顺序；单系列可取 header 同家族色做强调（规则见
  `feishu-cli-card/references/design.md` §1.1.1）。
- 卡片 UI 部分（header/tag/font）不认 hex，用飞书命名色枚举 —— slot 家族与
  枚举名一一对应：blue→`blue`、yellow→`yellow`、turquoise→`turquoise`、
  orange→`orange`、purple→`purple`、red→`red`、carmine→`carmine`、green→`green`。

### import（Mermaid classDef）

flowchart 的 classDef 直接使用"浅底/深边派生对"，见
`feishu-cli-import/references/mermaid-spec.md` §4（已从本色板派生）。

### board（SVG 自由作图 / 手写 JSON）

- 数据系列上色：light 列按序取用（原样取用无需校验；改色/散点 all-pairs 场景才跑）。
- 分组背景/节点填充：用"浅底/深边派生对"；连线/辅助元素用主题连线灰
  （默认 `#bbbfc4`，科技暗色主题 `#4a4a4a`）。
- 主题变体见 `feishu-cli-board/references/style.md`（同一 8 色相体系上的排列组合）。
