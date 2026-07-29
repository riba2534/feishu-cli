# Custom Visual Design

本文件描述可迁移的设计骨架、主题差异和组装判断，不提供固定业务文案。颜色 token 从
`../../../assets/themes/custom-visual-palettes.json` 读取；字段写法按 `../../components.md`；
全局工作流与硬规则按 card `workflow.md` 和校验器执行。

## 使用流程

1. 从 brief 判断内容目标与视觉语气，再从 `../../../assets/themes/custom-visual-palettes.json` 选择一个 `style_id`。
2. 在下方选择最接近内容叙事的结构原型；不要因为主题名称固定套用同一种业务文案。
3. 先构建信息层级和组件树，再注入主题 token；不要从旧示例反推内容。
4. 所有 Custom Visual pattern 固定从 `../../../assets/icons/doodle/manifest.jsonl` 按内容
   逐项选择同目录 PNG。Card JSON 的 `img_key` 可以直接写该本地素材路径，lint 与发送时都使用
   `--upload-images`；也可先用 `style_assets.py copy` 原样复制素材。
5. 删除内容不需要的可选模块。不存在真实行为时删除 Theme Button，不保留装饰性交互。
6. 用 `../../../scripts/lint_card.py` 校验，并在 PC 与移动端预览中检查层级、对比度和堆叠顺序。

## 共同外壳

所有 Theme Collection 卡片先满足以下结构：

```text
body (padding: 0)
└── root shell: interactive_container
    ├── one compact meta pill
    ├── title
    ├── subtitle
    ├── first highlight block
    ├── primary content sequence
    ├── optional details
    ├── source-aware action button group
    └── source / scope note
```

- 默认窄版，不显式设置 `config.width_mode`；只有用户明确要求宽版时才使用 `fill`。
- root shell 使用主题 `canvas`，承担整卡 padding；不加边框和圆角。
- 不使用根级 `header`。标题区只保留 pill、主标题、副标题三层。
- 顶部 meta pill/tag 必须保持内容自适应；不要把裸 `interactive_container` 直接作为 root shell 子元素。标准结构是 `column_set.flex_mode:"none"` + 单个 `column.width:"auto"` + 内层 `interactive_container.width:"auto"`、`height:"auto"`、`corner_radius:"999px"`、`padding:"3px 6px 3px 6px"`；禁止使用 `"fill"`，避免渲染成横向通栏。
- 标题后的第一个业务模块必须是高亮分栏，不先放来源、普通列表、成员区或 CTA。
- 高亮分栏直接使用 `column_set -> column.background_style`；不要给每列再套整块交互容器。
- 高亮分栏背景必须从主题底色派生出可见色阶：每列使用独立 `column_*` / highlight token，和 `canvas/body/panel` 至少有一档明度、饱和度或色相差；禁止某一列复用黑底、白底或普通 panel 导致和背景融在一起。
- 只要来源或主行动有真实 URL，就生成 Theme Button；单个动作是通栏按钮，多个动作是一个通栏 `column_set`，内部 2-3 个等比 `weighted` 按钮列。Theme Button 容器必须统一设置 `padding:"8px 12px 8px 12px"`；底部全局行动区域的最外层容器必须设置 `margin:"20px 0px 0px 0px"`。按钮文案按来源类型配置，例如妙记用 `打开妙记原文`、文档用 `打开文档`、日历用 `查看日程`、任务用 `查看任务`。
- 每个背景 token 都提供匹配的 `on_*` 文本 token；主题正文显式引用 `wb_*` 颜色。
- 多行 Markdown 必须逐行显式引用 `wb_*` 颜色 token；不要用单个 `<font>` 跨行包裹 bullet list、编号列表或表格说明，避免客户端只给首行着色导致暗色主题对比度失效。

## 结构原型

### A. Editorial Showcase

适用主题：`lime-slab`、`editorial-forest`、`monochrome`、`macchiato`、`neo-grid-bold`、`court-press`、`soft-editorial`。

```text
compact meta pill
title + subtitle
2–3 column highlight
icon list or fact list
metric columns
optional themed chart
optional collapsible details
source-aware Theme Button group
source / scope note
```

设计重点：

- 用同一信息架构表达产品说明、策略摘要、流程复盘或结构化报告，但根据内容删除不必要的指标、图表和折叠区。
- 高亮块只放最强结论、价值或状态；分栏内不放额外 pill/eyebrow。
- 指标块用于真实可比较数字；没有真实指标时改为事实卡或明确标注示例数据。
- 图表只在趋势、占比、排行或对比关系存在时出现，不把它当主题装饰。
- `monochrome` 和 `macchiato` 依靠字号、留白和边界建立层级；`neo-grid-bold` 使用严格网格与方角；其余主题按主题矩阵控制强调色比例。

### B. Handdrawn Status

适用主题：`handdrawn-vibes`、`handdrawn-dark`。

```text
compact status pill
short title + time/context subtitle
2 column emotional/status highlight
doodle icon list
optional progress or comparison block
optional chart
optional collapsible details
optional Theme Button
```

设计重点：

- 主要记忆点来自 2 个大色块与语义化 doodle 图标，不来自大量小标签。
- 同一列表使用一个 icon family，图标逐项去重并按语义召回。
- `handdrawn-vibes` 使用 cream 纸面、浅蓝与暖黄主高亮；正文保持轻盈。
- `handdrawn-dark` 使用 near-black 画布和近白文字；黑色或深色 doodle 必须放在近白 `icon_surface` 专用图标 shell，不能直接压在暗底、深色 panel、彩色高亮块或半透明黑层上。
- 图表不是默认模块；存在数据关系时才加入，并确保轴、标签和网格在暗色主题中可见。

### C. Wellness Planner

适用主题：`wellness-planner`。

```text
category pill
weekly title + short intention
2–3 priority highlight columns
day / habit / meal groups
lightweight progress summary
optional schedule details
optional real action
```

设计重点：

- 结构模拟窄版生活计划器，优先按一天、习惯或餐食分组，不照搬数据看板。
- olive 用于主文字和行动，terracotta / blush / soft yellow 用于有限的分类块。
- 每个分组只保留一个明确任务或节奏；避免把健康建议写成密集长文。
- 只有真实、用户提供或明确标注为示例的数据才能生成进度与统计。

### D. Product Showcase

适用主题：`claude-chrome`。

```text
product-state pill
product title + positioning subtitle
primary value highlight
workflow / use-case media list
adoption or proof metrics
optional themed chart
optional collapsible details
primary real action
```

设计重点：

- 保持 airy、安静、产品化；信息密度通过留白、短文案和稳定重复的 media item 控制。
- media list 使用静态浅色 item surface；图标列 `width: auto`、左对齐，safe16 doodle 使用 32px 固定槽位。
- 不复用任何旧 demo 的业务文案或 `img_key`。每条能力按当前语义重新选择图标。
- 证明指标必须来自用户或可验证来源；没有真实 adoption 数据时删除该区块或使用显式占位。

## 主题差异矩阵

| style_id           | 主视觉承诺                                                  | 结构与强调                                                |
|--------------------|--------------------------------------------------------|------------------------------------------------------|
| `lime-slab`        | electric lime + warm charcoal                          | lime 作为环境色；首要高亮用 charcoal，平面块、硬边界                    |
| `editorial-forest` | cream + forest + one dusty rose                        | forest 主导标题与边界；rose 只承担一个重点                          |
| `monochrome`       | cream + ink，无强调色                                       | 用字号、间距、边界制造层级；低圆角                                    |
| `macchiato`        | almond + espresso + taupe                              | 暖单色、低刺激；不加入亮色强调                                      |
| `neo-grid-bold`    | putty + ink + one neon lemon                           | 严格网格、方角、较硬边界；lemon 只出现一次主强调                          |
| `court-press`      | chalk + grass green + dusty pink                       | green 主导，pink 点题；适合步骤与团队节奏                           |
| `soft-editorial`   | cream + soft pastel cards                              | 大留白、柔和圆角；pastel 不承担状态语义                              |
| `handdrawn-vibes`  | cream paper + blue/yellow doodle blocks                | 两块主高亮、safe16 doodle、轻量列表节奏                           |
| `handdrawn-dark`   | near-black + deep panel + neon/terracotta + cream type | 暗底反白；canvas 与 panel 必须有可见明度差；深色图标必须使用专用浅色 icon shell |
| `wellness-planner` | cream + olive + lifestyle accents                      | 移动端 planner 分组；分类色少量使用                               |
| `claude-chrome`    | off-white + deep cream + mint                          | airy 产品展示、media list、克制证明模块                          |

具体颜色值、usage 和主题级 palette 规则只维护在 `../../../assets/themes/custom-visual-palettes.json`，不要在本文件复制第二套颜色数据。

## 组件取舍

- `chart`：仅用于真实数据关系；`color_theme` 使用 `primary` 并显式主题化画布、系列、轴、标签和网格。
- Markdown table：用于主题外壳内的轻量明细；2–5 列、3–8 行。标题与表格拆成相邻元素。
- 原生 `table` / `form`：只有用户明确需要其原生能力时，作为独立顶层模块放在主题外壳之外。
- `collapsible_panel`：承载 FAQ、长说明、完整变更或风险；不隐藏主结论和主行动。标题条使用
  白色/浅色 surface，主题 accent 只进入标题文字、图标或细边框，不铺满横条。
- Theme Button：优先使用带真实 `open_url` 的可点击 `interactive_container`；按钮容器固定 `padding:"8px 12px 8px 12px"`；根据来源类型配置按钮文案和 URL。单动作通栏，多个动作放进同一个通栏 `column_set`，内部等比 `weighted` 列；底部全局行动区域统一用四值 `margin` 补 20px 上间距，不使用 CSS 式 `margin-top`；没有真实行为就删除。
- 图片与图标：使用真实 `img_key`，或使用存在的本地素材路径并声明 `--upload-images`；
  召回失败时降级为无图结构。

## 设计门禁

- 卡片是否从 brief 推导，而不是从某个旧示例替换文案。
- 首屏是否只有一个主焦点，标题后的第一个模块是否为高亮块。
- 是否按实际内容删除了无意义的图表、指标、折叠区和按钮。
- 背景与前景 token 是否成对，暗色和浅色区域是否都可读。
- 图标是否按当前语义重新召回、同组去重，并满足主题图标底座规则。
- 移动端堆叠后是否仍保持正确阅读顺序。
- 是否完全没有固定示例文案、示例 ID、示例链接或旧 demo `img_key` 泄漏到业务卡。
