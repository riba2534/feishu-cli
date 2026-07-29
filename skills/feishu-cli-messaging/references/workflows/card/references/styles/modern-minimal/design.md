# Modern Minimal Design Blueprints

本文件保存组件的结构意图、组合顺序、token 角色和关键参数，不保存整卡示例 JSON、示例业务文案、固定链接或固定 `img_key`。字段定义以 `../../components.md` 为准，组件选择与硬约束以 `components.md` 为准。

## 使用流程

1. 先按 `index.md` 确认 Modern Minimal 路由，再读取 `basic.md` 和 `components.md`。
2. 从本文件选择必要蓝图，通常组合 3–5 个模块；不要完整复刻全部模块。
3. 生成业务内容后按语义选择 header 状态、图标和真实行为。
4. 图片从当前 Skill manifest 选择；可直接使用本地素材路径并在 lint/发送时声明
   `--upload-images`，不保存跨租户 key。
5. 运行 `../../../scripts/lint_card.py`，并检查 PC 与移动端预览。

## 整卡顺序

```text
Header
→ Split Hero / Summary Callout
→ optional Key Point Columns
→ Surface Metrics / Rule Section / Media List / business content
→ optional CTA
```

- 模块顺序、高亮聚合间距、token 使用边界与图标要求以 `components.md` 的 Split Hero / Key Point Columns 为唯一来源；本节只说明蓝图的组合意图。
- Surface Metrics 属于高亮后的结构化信息，不替代三点提炼。
- 使用 body mock header 时清空 `body.padding`；正文模块自行提供左右 margin 和底部留白。
- 所有 column surface 都必须相对父级背景形成可见色阶变化；不要让分栏背景和 body 画布、mock header 下方背景或普通 panel 解析成同一颜色。

## Token Roles

| token | 用途 | 约束 |
| --- | --- | --- |
| `mm_surface_subtle` | 普通浅色 surface | light 为近白；dark 与画布拉开一级明度 |
| `mm_text_subtle` | 二级说明 | light/dark 分别保持可读，不用于主标题 |
| `mm_rule` | 细边界与分隔 | 使用低透明度，不模拟阴影 |
| `mm_key_point_*_surface` | Summary / 顶部重点分栏高亮块 | 具体使用边界见 `components.md` 的高亮聚合总则 |
| `mm_media_item_surface` | Media List item | 必须区别于外层画布 |
| `mm_media_icon_surface` | 黑色线性图标底座 | light/dark 都保持接近白色 |
| `mm_key_point_icon_surface/border` | 高亮聚合图标底座 | 具体使用边界见 `components.md` 的高亮聚合总则 |
| `mm_media_icon_border` | 图标底座边界 | 与浅色底座可见但克制 |

只有蓝图实际使用某个 token 时才注入 `config.style.color`，不要把全量 token 复制到每张卡。

## Header

### Body mock header

```text
body padding = 0
img (body.elements[0])
title markdown
subtitle markdown
business content
```

- 资源选择、上传、图片字段、标题覆盖偏移和原生 `header` 限制都以 `components.md` 的 Cover Header 为唯一来源。

## Split Hero / Summary Callout

适用：Header 后的一句话结论、洞察、风险或核心摘要。

```text
column_set (single subtle highlight surface)
├── auto column: one asset-library icon slot
└── weighted column
    ├── short conclusion
    └── one-line explanation
```

- 必须紧跟 Header 标题区之后，作为首屏高亮块承接标题；不作为正文中段的普通强调块复用，正文强调改用灰阶 Rule Section 或普通中性 surface。
- `flex_mode: "stretch"`，桌面端可双栏，移动端自然堆叠；左侧放结论/标题/核心动作，右侧放摘要/指标/风险/补充信息。
- auto 图标列保持紧凑，weighted 文案列承担主要宽度；移动端按图标→结论→说明阅读。
- 图标必须来自 phosphor-icons / linear-icon，放入固定浅色 `mm_key_point_icon_surface` 图标底座；禁止 emoji / Unicode 符号。
- surface 使用静态浅色 token，正文显式使用匹配的 ink / muted token。
- surface 必须比周围画布更可见；若浅色底在预览中融入页面，改用同色系更深/更饱和一档 token 或补 border token。
- 不要把 Summary Callout 误做成两张等权 KPI 卡；它只有一个核心结论。

## Key Point Columns

适用：源内容密集，需要继续提炼 2-3 个重点、原则、亮点或风险。

```text
column_set (stretch, 8px gutter)
├── weighted column: icon slot + label + title + one sentence
├── weighted column: icon slot + label + title + one sentence
└── optional weighted column: icon slot + label + title + one sentence
```

- 必须紧跟 Split Hero；2-3 列分别使用 blue / lime / lavender 浅色 surface，只有 2 个分栏时任选其中 2 个，不要补空列。
- 与 Split Hero 的有效垂直 gap 必须等于 `horizontal_spacing`（8px），见「整卡顺序」的高亮聚合间距规则。
- 列使用 `column.background_style`，不伪造 border；每列约 14px padding，内容垂直间距约 8px。
- 三列 surface 必须互相可区分，并与父背景形成浅色梯度；禁止某列复用普通 panel 或页面背景。
- 图标底座 48px 见方（12px padding + 24px 图标），12px 圆角，浅底带细边界；`preview:false`、`transparent:true`。
- 每列只有短 label、加粗标题和一句提炼说明；不要塞原始长段落。

## Surface Metrics

适用：2–4 个真实 KPI、配额、比例或对象摘要。

```text
column_set (stretch)
└── 2–4 weighted columns
    ├── label (notation, muted)
    ├── value (standalone, prominent)
    └── note (notation, muted)
```

- 每列使用浅色 surface，label/value/note 必须拆分。
- 每列 surface 必须相对外层背景有一档明度/饱和度差，必要时加同色系边界。
- 纯数字按长度缩放：1–3 位 `heading-1`，4–6 位 `heading-2`，7 位及以上 `heading`。
- 没有用户提供或可验证数字时删除指标模块，或在第一屏明确标注示例数据。

## Rule Section

适用：规则、步骤、风险、结论拆解。

```text
short section title
one concise paragraph or true list
hr
next short section
```

- 标题、正文和下一段拆成相邻元素；不在一个 Markdown 字符串里模拟整页。
- 只对真实步骤、待办、风险或密集事实使用列表。

## CTA Row

- 默认：一个 `primary_filled`、`width:"fill"` 的真实主操作。
- 两个动作同等重要时：等分 `column_set`，每列一个 fill 按钮，并明确主次或保持同等视觉权重。
- 底部 CTA Row 的最外层按钮或按钮组必须用四值 `margin` 补 20px 上间距；有左右页边距时写成 `margin:"20px 20px 0px 20px"`，不要使用 CSS 式 `margin-top`。
- 所有按钮必须有真实 behavior / 表单含义；示例 URL 和装饰按钮禁止进入业务卡。
- CTA 不用于填补版面；没有行动就删除整个模块。

## Clickable Surface

两种模式二选一：

1. 整卡点击：外层 `interactive_container` 挂唯一 `open_url`，内部只放标题和摘要。
2. 静态 surface + CTA：外层无 behavior，内部放明确按钮行。

- 不同时让外层和内部 CTA 承担冲突的点击语义。
- 常用结构为 fill、约 16–24px padding、16px 圆角、`mm_rule` 细边界；非交互分组不使用空 behavior。

## Status Pill

- 只表达 live / plan / risk / status 等短状态，保留 1–2 个。
- 状态语义与颜色一致；不生成标签墙。
- 状态 pill 是标题辅助，不替代标题、结论或 CTA。

## Invoice Panel

```text
panel title + billing period
prominent total / quota
hr
key-value details
due date / risk note
```

- 金额、单位、周期和到期日可核对；数值格式统一。
- 详情多时使用单列键值或少量分栏，不在首屏堆密集账单行。

## Media List

适用：图文资源、文章、商品、文件或图标能力清单。

```text
interactive_container item surface
└── column_set (16px gap)
    ├── auto column
    │   └── disabled icon surface
    │       └── img
    └── weighted column
        └── title + concise summary
```

- 3 条以内适合首屏；超过 5 条改成精选 3 条 + 真实“查看全部”。
- item 使用 `mm_media_item_surface`、约 12px 圆角、16px 级 padding；只有真实整条跳转时才让 item 可点击。
- icon surface 使用接近白色的 `mm_media_icon_surface` 和细边界；约 8px padding、12px 圆角。
- 图标固定 `preview:false`、`transparent:true`、`size:"32px 32px"`、`scale_type:"crop_center"`，不得使用 `mode/custom_width`。
- 每项按当前语义从 manifest 选图，不保存模板 key；缺 key 时生成无图列表或先完成上传映射。

## 设计门禁

- 是否只加载并组合当前任务需要的蓝图，而不是复刻旧 demo 整卡。
- Header 状态、标题和首屏结论是否一致。
- Split Hero 与 Key Point Columns 是否连续，普通分栏是否位于其后。
- 是否只注入实际使用的 token，light/dark 均可读。
- 所有图片、链接、指标和交互是否真实或明确占位。
- 移动端堆叠后是否仍保留正确阅读顺序和清晰点击目标。
