---
name: custom-visual-card-style
description: feishu-cli-messaging card workflow 的自定义视觉主题指南。用于个人主页、作品集、品牌页、团队名片、暗黑科技风、节日贺卡、海报式、网页/落地页式等视觉卡片。
---

# 自定义视觉卡片

本文件中的数字、日期、标题和文案只演示布局，不是可发送事实。生成业务卡时必须替换为用户提供
或已授权来源中的真实内容，再通过严格 lint。

## 使用边界

当用户的诉求不是典型报表、宣传或表单，而是偏“页面设计/视觉风格/个人或品牌表达”时使用本风格指南。输出仍必须是飞书 Card JSON 2.0，不输出 HTML/CSS，不发明 schema 不支持的 `style`、`class`、CSS 颜色值或背景图属性。

典型触发：个人介绍页、用户 profile、作品集、品牌页、团队名片、暗黑主题、赛博朋克、酷炫科技感、强视觉网页化表达、像网页/落地页一样、视觉冲击、海报式但无现成海报图。

## 设计思考

本风格指南吸收 frontend-design skill 的核心取向：避免通用、保守、模板化的 AI 视觉；先理解上下文，再选择一个清晰、可记忆、能被飞书 schema 承载的美学方向。不要先套模板，要先做设计判断。

- 不要枚举或套用固定布局模式。先理解用户 brief，再自行推导信息层级、视觉叙事、模块节奏和移动端阅读顺序。
- 先判断目的：这张卡片解决什么沟通问题，谁会读，读完应记住什么或做什么。
- 再确定语气：选择一个明确的视觉立场，可以极简、浓烈、编辑化、工业感、玩具感、奢华感、科技感或其它更贴合上下文的方向；关键是意图清晰，不是视觉强度越高越好。
- 识别约束：飞书卡片没有自由 CSS、真实字体、自由定位和动效；因此要把设计压缩到 schema 支持的组件、主题色、分栏、文案节奏、图片位和按钮行为里。
- 找到记忆点：每张自定义视觉卡片至少有一个可被记住的设计选择，例如首屏一句强主张、非对称信息节奏、极克制的留白、强对比色块、独特的模块命名或明确的视觉隐喻。
- 每张卡片只选择一个明确审美方向；不要平均分配颜色和视觉注意力。
- 避免常见泛化审美：不要默认白底紫渐变、圆角堆卡片、无关装饰、同质化标签墙或没有语境的“高级感”。
- 让文字也参与设计：用短标题、强节奏断行、重点加粗、有限的颜色强调和模块标题建立类似网页 typography 的层级；不要写成长段说明。
- 让复杂度匹配视觉方向：浓烈/高能设计可以使用更多分区、强对比和图像；克制/高级设计要依赖精确文案、留白、少量重点元素和严格一致的间距。

## 自由布局规则

视觉卡片默认不要渲染成“模块标题 + Markdown 列表”。当内容是个人信息、能力标签、联系偏好、品牌卖点、作品亮点、团队角色、服务能力或风格宣言时，优先大胆使用 Card JSON 2.0 能承载的自由布局：

- 自定义主题默认先生成窄版卡片，不显式声明 `config.width_mode`；只有用户明确要求宽版、仪表盘、大屏、横向评测或多图表数据展示时，才使用 `width_mode: "fill"`。
- 用非对称 `column_set`、嵌套浅底/深底区块、事实 tile、标签云、短句海报区、并列信息组和强标题块表达信息层级。
- 把“基本信息 / 能力标签 / 联系偏好”这类内容转成区块、标签、短句和分栏，不要机械写成 `-` 列表。
- 列表只用于真实步骤、待办、来源、日志、风险清单或密集事实；如果列表只是为了省事，应改成更有构图感的区块。
- 自由布局不等于发明 CSS。只能使用 `column_set`、`column`、`div`、`markdown`、`text_tag`、`img`、`button`、`hr` 等 schema 组件；多列布局默认 `flex_mode: "stretch"`，移动端按阅读顺序堆叠。
- 大胆渲染要服务于记忆点和扫读效率：第一屏先给最强信息，后续模块用节奏和密度变化承接，不要堆满同质化小卡片。

## 内置主题

### 主题路由边界

当用户要求现代极简、Stripe-like、Linear-like、SaaS/API/企业平台式克制视觉时，不在本文件内继续推导，直接路由到 `../modern-minimal/index.md`。本文件只负责自由视觉主题和 Theme Collection，不承担 `modern-minimal` 的默认或显式路由职责。

### Theme Collection

当用户明确指定 `Lime Slab`、`Editorial Forest`、`Monochrome`、`Macchiato`、`Neo-Grid Bold`、`Court Press`、`Soft Editorial`、`Handdrawn Vibes`、`Handdrawn Dark`、`Wellness Planner`、`Claude Chrome`，或说“学习 beautiful-feishu-whiteboard / 白板配色 / editorial poster / 杂志海报风 / 手绘高亮块 / 暗黑手绘 / 健康计划 / Claude 官网风 / Claude for Chrome 风格”时，读取 `design.md` 和 `../../../assets/themes/custom-visual-palettes.json`。`design.md` 负责结构原型与组装判断，palette manifest 负责颜色、usage 和主题级规则。

主题来源包括 `beautiful-feishu-whiteboard` 的 palette / mood 转译主题、`claude.com/claude-for-chrome` 沉淀出的 `claude-chrome`、手绘图标高亮块沉淀出的 `handdrawn-vibes`、暗黑手绘参考图沉淀出的 `handdrawn-dark`，以及健康习惯与每周餐食规划参考图沉淀出的 `wellness-planner`。这些主题都服务于飞书 Card JSON 2.0，不是网页也不是白板；不要声称已实现 CSS、SVG、渐变、阴影、自由定位或真实字体。

所有 `custom-visual` 子主题统一使用 DOODLE 图标族。需要图标时固定读取 `../../../assets/icons/doodle/manifest.jsonl`，按 `tags` / `filekey` 和当前内容语义选择同目录 PNG；不要再按子主题切换到 flat、origami、interface、phosphor 或其它图标族。

这些主题必须改变卡片结构：`body.padding` 清空，最外层用 `interactive_container` 承载全卡背景和内边距，但不要给这一层加边框和圆角；主题色只映射到最外层背景、顶部高亮分栏、内容交互容器描边、文本颜色和 mock button，不要把图标库或头图库混入主题色。

自定义主题里使用 `collapsible_panel` 时，折叠容器本身的上 padding 必须清空，例如 `padding: "0px 12px 12px 12px"`；`header.padding` 的左右边距要和内容区对齐，例如 `"8px 12px 8px 12px"`。需要内容区额外间距时放到内容元素的 `margin`，不要让折叠 header 被外层 padding 顶开或左右贴边。

标题右侧 `text_tag` 的颜色也要随主题选择，不要固定 `indigo`；例如 forest/court 用 `green`，mono 用 `grey`，macchiato 用 `orange`，neo-grid 用 `yellow`，soft editorial 用 `purple`。

主题生成流程：

1. 从用户 brief 和 `../../../assets/themes/custom-visual-palettes.json` 选择 `style_id`。优先运行
   `python3 ../../../scripts/style_assets.py tokens <style_id>` 生成可直接注入
   `config.style.color` 的 RGBA 明暗模式 token，不手工重复转换。
2. 清空 `body.padding`，首个元素必须是全卡 `interactive_container`；该层只负责背景、内边距和整体节奏，不加边框、不加圆角。
3. 标题区放在 root shell 内，固定只保留 `pill/tag + 主标题 + 副标题` 三层；不要额外添加 eyebrow、meta 说明行、重复来源行或装饰性小标题。
   - 标题区顶部最多保留 1 个小胶囊标签，用于承载最短 meta，例如 `EP.88`、`Minutes`、`Calendar`；不要把期刊、日期、来源拆成多个并列胶囊。
   - 胶囊标签必须是轻量 meta，不承担主标题职责；不要把裸 `interactive_container` 直接作为 root shell 子元素，否则客户端可能把它渲染成通栏。标准结构是 `column_set.flex_mode:"none"` + 单个 `column.width:"auto"` + 内层 `interactive_container.width:"auto"`、`height:"auto"`、`corner_radius:"999px"`、`padding:"3px 6px 3px 6px"`，内部使用小字号（如 `notation` 或主题自定义 meta 字号），文本居中。顶部 pill/tag 严禁使用 `width:"fill"` 或 `height:"fill"`；不要把它撑成通栏条。
   - 期刊、日期、来源和对象归属优先合并进副标题，例如 `EP.88 · 2026-07-08 · 来自 <person> 的飞书妙记`。涉及真实成员时遵循全局人员组件规则：单人用 `person`，多人用 `person_list`，没有真实有效用户 ID 时不要编造。
   - 标题区要紧凑且统一对齐：pill 到主标题、主标题到副标题只保留一档稳定间距，不要在标题上下堆多行解释文字；自定义主题 demo 默认使用标题 `6px 0px 0px 0px`、副标题 `-6px 0px 20px 0px`，用 `-6px` 让标题和副标题形成稳定的紧凑组；更多说明进入标题下方的首个高亮块。
4. 顶部高亮区使用 `column_set` + 2 到 3 个 `column.background_style`，背景必须使用 `column_*` 或同主题更深/更浅/更饱和的独立 token，不能复用 `canvas/body/panel`。分栏背景要在整卡背景的基础上形成可见色阶梯度，至少一档明度、饱和度或色相差；不要让某一列像透明文字直接浮在底色上。
5. 高亮块、指标块、重点对比块这类分栏必须直接使用 `column_set -> column.background_style` 排版；不要在每个 `column.elements` 里再额外嵌套整块 `interactive_container` 作为分栏卡片，否则 AI 生成内容长短不一致时无法保障列高一致。列内只允许保留图标底座、按钮等必要的小型 `interactive_container`。
   - 高亮分栏内不要放 pill 样式短标签，自定义 pill 背景会和分栏背景冲突，请直接省略。
   - 高亮分栏内不要用一个 markdown 的换行完成标题/正文排版，例如 `**工作流变**\n\nagent 进线程`。必须拆成相邻 elements：普通标题 `markdown` 用 `text_size:"normal"` 或 `notation`，确定要突出的关键字标题可用 `heading*`；正文 `markdown` 用 `notation`；数值型指标的 value 按长度使用 `heading-1/heading-2/heading`。
6. 正文优先用图标列表、指标分栏、主题 panel、Markdown table、折叠详情和 Theme Button 组织，不要退化为纯 Markdown bullet list。
7. `design.md` 只提供图标槽位、尺寸、容器和视觉节奏；生成业务卡时读取
   `../../../assets/icons/doodle/manifest.jsonl`，或使用 `style_assets.py assets --family doodle`
   按当前内容语义选择 PNG。`img_key` 可直接写本地路径，lint 与发送时都使用
   `--upload-images`；不要保存或复用跨租户 token。
8. 为所有背景 token 准备对应 `on_*` 前景 token，并在深色或中深色背景上使用反白。
9. 输出前运行 `python3 ../../../scripts/lint_card.py`；发送候选使用 `--strict`，包含素材时
   追加 `--upload-images`。

推荐默认结构：

```text
root shell
├── left-aligned single pill/tag
├── title
├── subtitle
├── high priority highlight block
├── icon list / metric cards
├── optional chart or themed table
├── optional collapsible details
├── theme action button group
└── source / scope note
```

结构硬约束：

- 自定义主题默认先按窄版消息卡生成，不显式声明 `config.width_mode`；只有用户明确要求宽版、仪表盘、大屏、横向评测或多图表数据展示时，才使用 `width_mode: "fill"`。
- `body.padding` 必须是 `"0px 0px 0px 0px"`。
- `body.elements[0]` 必须是最外层 `interactive_container`，承载整张卡的背景、内边距和整体节奏。
- 最外层 `interactive_container.background_style` 使用主题的 `canvas` token。
- 最外层 `interactive_container.padding` 通常为 `"20px 20px 20px 20px"`；所有 4 值 padding 的左右值必须一致，主题可以决定具体数字。
- 最外层 `interactive_container` 不要加边框和圆角：`has_border` 必须为 `false`，`corner_radius` 使用 `""` 或 `"0px"`，避免和飞书消息气泡自身边框/圆角重合。
- 卡片不要使用根级 `header`；标题放在 root shell 内的第一组 `markdown`。

美观度组装规律：

- 标题下方的第一个内容模块必须是高亮块，承接首屏结论、价值主张或最重要状态；不要先放普通 `panel`、指标块、成员区、列表、来源说明或 CTA。高亮块中的核心指标数字必须根据长度动态缩放：1-3 位用 `heading-1`，4-6 位用 `heading-2`，7 位及以上用 `heading`。
- 高亮块分栏内部禁止使用大圆角标签容器；分栏里的 `Signal` / `Proof` / `Step 1` 这类短标签请直接省略，不改成另一种小标签，也不做 eyebrow 文本。
- 高亮块之后如果紧跟顶部行动按钮、CTA surface 或首屏行动说明，必须和高亮块拉开垂直间距，推荐在该模块上设置 `margin: "16px 0px 0px 0px"` 到 `20px 0px 0px 0px"`；不要让行动按钮贴在高亮块下方。
- 核心内容优先用 list + icon 组织。能力清单、功能更新、步骤说明、亮点总结、Agent 能力介绍，都应使用图标列表或图标化分栏；图标固定从 `../../../assets/icons/doodle/manifest.jsonl` 按每条内容语义分别选择，同一组 list 中去重。
- 模板里的 `img_key` 只能作为槽位示例，不是业务生成的默认答案。每次生成时从 DOODLE manifest 按每条标题/摘要语义重新选择图标；如果无法完成图标召回，应明确降级为无图结构或说明缺少图标资产，不要把示例图标当占位符复用。
- 深色主题的图标 shell 必须使用专用静态浅色底座。黑色或深色 doodle / safe16 图标必须放入 `*_icon_surface`，或解析后接近 white / cream 的独立浅色 token；严禁使用 `canvas`、`body`、深色 `panel`、`column_*`、`accent`、半透明黑层或仅靠边框的深色容器作为图标底座。
- 图表 / 表格是可选增强。内容包含趋势、占比、排行、对比、阶段分布或多维数据时，优先使用主题化图表；主题外壳内不嵌套原生 `table`，轻量明细使用 Markdown table。
- 折叠面板是可选详情披露层。安装方式、长说明、完整 changelog、FAQ、风险说明、命令清单等二级信息可放进 `collapsible_panel`；内容区第一个元素应是带非默认 `background_style`、padding、边框和主题圆角的 `interactive_container` surface。
- 行动按钮是默认收口模块。只要输入来源、原文、文档、日程、任务、表格、Base、报告或详情页存在真实可跳转 URL，就必须在主题外壳内生成 Theme Button；没有真实行为或 URL 时不要生成假按钮。
- 按来源配置按钮文案和 `behaviors.open_url.default_url`：妙记/Minutes -> `打开妙记原文`；文档/Doc -> `打开文档`；日历/Calendar -> `查看日程`；Todo/Task -> `查看任务`；Sheet/Base -> `打开表格` / `打开 Base`；报告/详情页 -> `查看详情`；安装指南 -> `查看安装指南`；表单 -> `提交 / 填写表单`。
- 单个行动使用一个通栏 Theme Button：`interactive_container.width:"fill"`，主题 `button` 背景，文本使用 `button_text`，放在来源说明前；按钮容器必须设置 `padding:"8px 12px 8px 12px"` 和 `margin:"20px 0px 0px 0px"`，不要只依赖父级 `vertical_spacing`。
- 多个行动使用单个通栏按钮组：外层 `column_set` 跟随父容器通栏，使用 `flex_mode:"stretch"`；`column_set` 不支持 `width`。每个 `column.width:"weighted"` 且 `weight:1`，列内放一个 `interactive_container.width:"fill"` 的 Theme Button；每个按钮容器必须统一 `padding:"8px 12px 8px 12px"`，按钮组外层必须设置 `margin:"20px 0px 0px 0px"`，不要把多个按钮拆成多行散落模块，也不要让某个按钮独占不等宽。

背景层级必须清楚：

- `canvas`：整卡最底层背景。
- `panel/surface`：普通内容盒和图标列表 item，必须明显区别于 `canvas`。
- `column_*`：分栏和指标卡，必须比 `canvas/panel` 更有重量、更浅、更深或更高饱和度；作为 highlight block 使用时应形成同主题色阶梯度，不得与底色融合，必要时同时准备同色阶边框 token。
- `button/accent`：只用于主行动或单个强强调，不要当普通列表背景。
- 文本颜色：用 `<font color="token">` 调用 `config.style.color` token。多行 Markdown、bullet list、编号列表和公司明细必须逐行包裹 `<font>`，不要用一个 `<font>` 从第一行跨到最后一行；飞书客户端可能只给首行着色，后续行回落默认色，暗色主题会出现黑底黑字或露出 `</font>`。

嵌套容器限制：

- 默认保持视觉完整：轻量排行、对比明细、异常清单、反馈入口和 CTA 应放在主题外壳内，继承 `canvas/panel/ink/muted` 等 token。
- 不要在主题外壳、分栏、内容盒子或折叠面板里嵌套原生 `table`；小型排行和对比明细使用 Markdown table。
- 不要在主题外壳、分栏、内容盒子或折叠面板里嵌套原生 `form`；反馈、报名、申请类诉求优先在主题外壳内生成带 `behaviors.open_url` 的 Theme Button，链接到 `TODO_FORM_LINK`。
- 只有用户明确要求原生表格/表单能力，且确实需要结构化交互并接受视觉断层时，才把 `table` 或 `form` 作为独立顶层 `body.elements` 模块。

对比度规则：

- 文本颜色必须优先使用自定义 `rgba(...)` token，避免飞书 dark mode 对默认文本色、内置色名或 hex 色值做不可控翻转。
- `ink`、`muted_text`、`on_canvas`、`on_panel`、`on_accent`、`on_column_*`、`on_button`、`button_text` 都属于文本 token，必须自定义为 `rgba(...)`。原 palette 的 `muted` 是视觉原色；`style_assets.py tokens/starter` 会在其对 `canvas` 不足 4.5:1 时生成可读的 `muted_text` 回退，正文不要直接使用低对比 `muted`。
- Markdown 文本必须用 `<font color="theme_token">...</font>` 包裹关键文字；不要让标题、正文、图表说明、表格内容裸奔使用平台默认文本色。多行内容必须每一行独立包裹，例如 `- <font color="wb_x_muted_text">**模型优先**：...</font>`，不要生成 `<font color="wb_x_muted_text">- 第一行\n- 第二行</font>`。
- 所有带背景色的区域都必须成对声明文本 token：`canvas/on_canvas`、`body/on_body`、`panel/on_panel`、`accent/on_accent`、`accent_2/on_accent_2`、`button/button_text`、`column_*/on_column_*`。
- 深色或中深色背景必须反白，优先使用 white / cream；浅色背景使用 `ink`，二级说明可以使用 `muted`。

Theme Button 规则：

- 本主题集合不要默认使用飞书原生 `button` 作为视觉主按钮。原生按钮会回落到飞书默认蓝色、白底、边框或平台按钮样式，无法继承主题 token。
- 优先使用可点击 `interactive_container` 模拟按钮色块：容器 `horizontal_align: "center"`，内部 `markdown.text_align: "center"`；有真实跳转时写 `behaviors.open_url`，没有真实行为时不写交互。
- Theme Button 必须根据来源类型选择合适文案和 URL，不能统一写“查看详情”。来源 URL 明确时，`default_url` 使用该真实 URL；来源缺失时向用户确认或删除按钮，不要编造链接。
- 多按钮组必须是一个通栏 `column_set`，内部 2-3 个等比 `weighted` 列；列内按钮统一高度感、统一 `padding:"8px 12px 8px 12px"`、统一 `corner_radius`，按钮之间只通过 `horizontal_spacing` 拉开。
- 底部全局 Theme Button / Theme Button 组必须在最外层行动区域用 `margin:"20px 0px 0px 0px"` 补 20px 上间距；不要使用 CSS 式 `margin-top`，也不要让按钮贴住上方内容盒。
- 只有用户明确要求平台原生按钮、`form_submit` / `form_reset` 或其它必须使用原生按钮的场景，才使用原生 `button`。

图表主题化：

- `color_theme` 使用 `"primary"`，不要使用 `"brand"`，避免出现飞书默认蓝色。
- 展示型直方图必须显式设置 `chart_spec.background`、`chart_spec.color`、label、axis、grid，确保画布、数据系列、轴线文字和网格线彼此可分辨。
- 图表大面积色块不要使用纯黑或接近纯黑；暗色主题若必须使用黑色画布，必须同时使用近白 label/axis 和可见 grid。
- label/axis/grid 要从同主题的 `ink`、`muted`、浅分隔色里选，避免图表局部跳出整卡配色。

折叠面板规则：

- `collapsible_panel.padding` 的上边距必须为 `0px`，推荐 `"0px 12px 12px 12px"`。
- `header.padding` 控制标题行触控区域，推荐 `"8px 12px 8px 12px"`；左右必须和内容区边距对齐，不能让 header 文字贴边。
- 折叠标题条始终使用白色/浅色 surface；即使主题本身大胆或暗黑，也不要把 accent、panel、
  canvas 或饱和语义色铺满标题条。主题色通过标题 `<font>`、图标或细边框表达。
- 内容区需要呼吸感时，给第一个内容元素设置 `margin`，不要把上间距放在折叠容器自身。

主题设计资源：

```text
design.md
../../../assets/themes/custom-visual-palettes.json
```

- 根据用户 brief 和 `../../../assets/themes/custom-visual-palettes.json` 确定 `style_id`，
  再从 `design.md` 选择结构原型；不要从固定整卡 JSON 开始替换文案。
- 结构原型中的指标、图表、折叠面板和 Theme Button 都是可选模块，只在内容需要时生成。
- 设计文件只沉淀结构和主题转译方式，不保存示例业务文案或示例图标 `img_key`；业务卡里的图标必须来自本次内容的语义召回。

| style_id | 风格 | 调性 | 适合 |
| --- | --- | --- | --- |
| `lime-slab` | Lime Slab | electric / SaaS / neobrutalist | 产品说明、功能拆解、对比网格 |
| `editorial-forest` | Editorial Forest | literary / bookish / forest + rose | 策略说明、年度报告、编辑化摘要 |
| `monochrome` | Monochrome | quiet / minimal / no accent | 高管摘要、政策说明、严肃备忘 |
| `macchiato` | Macchiato | warm monochrome / almond + espresso | 温和总结、知识说明、评审文档 |
| `neo-grid-bold` | Neo-Grid Bold | editorial grid / bold / structured | 指标网格、发布 brief、结构化报告 |
| `court-press` | Court Press | sports poster / green + pink clash | 流程说明、团队更新、活动复盘 |
| `soft-editorial` | Soft Editorial | warm magazine / soft pastels | 研究摘要、温和概览、编辑化日报 |
| `handdrawn-vibes` | Handdrawn Vibes | cream paper / doodle icons / blue + yellow highlight cards | 轻量状态、情绪摘要、创作者更新、轻产品介绍 |
| `handdrawn-dark` | Handdrawn Dark | near-black sketchbook / cream text / terracotta + mist blue | 暗黑状态、夜间工作流、创作者更新、轻产品介绍 |
| `wellness-planner` | Wellness Planner | warm cream / lifestyle planner / olive + terracotta + blush | 健康习惯、餐食计划、每日安排、轻量日程 |
| `claude-chrome` | Claude Chrome | elegant / airy / productized / quiet | 产品能力介绍、工具说明、工作流摘要、官网风信息卡 |

主题差异规则：

- `Lime Slab`：electric lime 或 warm cream canvas，正文 white panel；顶部主高亮优先用 warm charcoal / olive charcoal，不把 lime 铺成大面积信息卡。
- `Editorial Forest`：cream 纸面 + forest green + dusty rose；标题、边框、主按钮用 forest，rose 只做一个重点高亮。
- `Monochrome`：无强调色；cream paper + ink-black + graphite；层级来自字号、间距、边框，不来自颜色。
- `Macchiato`：warm almond canvas + espresso ink + taupe；无亮色强调，用暖灰做二级文案。
- `Neo-Grid Bold`：putty paper + ink + single neon lemon；严格网格，方角，边框更硬；neon lemon 背景必须使用 ink-black 前景。
- `Court Press`：chalk page + grass green + dusty pink；green 主导，pink punch，适合步骤、团队节奏、活动复盘。
- `Soft Editorial`：cream page + soft pastel cards；pastel 不是语义状态色；高亮分栏里的 DOODLE 图标必须落进固定 icon slot，不要裸放。
- `Handdrawn Vibes`：奶油纸面 `#FFFAEE`、黑色 ink、浅蓝 `#ADDEFF` 与暖黄 `#FEC554` 并排形成主记忆点；使用 `../../../assets/icons/doodle/manifest.jsonl` 中的 safe16 图标，原始无 padding 的 doodle 图标不得出现在任何场景。
- `Handdrawn Dark`：near-black canvas `#05060A`、deep ink panel `#141722`、neon green `#D0FE56`、hot orange `#FC753D`、soft purple `#A192F9`、warm yellow `#FFEB80`；严禁把 `canvas/body/panel` 都写成纯黑，暗色主题的层级必须靠离散色阶表达，不要声称或尝试使用 CSS gradient；普通内容区优先使用近白正文、灰白辅助文案和中性白边框；黑色 safe16 doodle 图标不能直接压在黑色、深色 panel 或彩色高亮容器上，必须使用近白 `wb_handdrawn_dark_icon_surface` 承载。
- `Wellness Planner`：cream canvas `#F7F1E3`、off-white panel `#FFF8EE`、olive ink/button `#25351F`、terracotta `#E77C53`、sage `#667242`、blush `#EAB4CA`、soft yellow `#F3D95F`；结构优先模拟移动端 planner。
- `Claude Chrome`：off-white canvas + deep cream panel + mint accent；保留 Mock Header，不使用根级 `header`；从 `../../../assets/icons/doodle/manifest.jsonl` 按每条内容语义重新选择 safe16 图标，不复用历史模板中的示例 `img_key`。media list 外层 item surface 保持主题静态浅色底座，左侧图标列使用 `width:"auto"` 且 `horizontal_align:"left"`；图标使用 `preview: false`、`transparent: true`、`size: "32px 32px"`、`scale_type: "crop_center"`，不要使用 `custom_width` 或 `mode`。

## 场景 Pattern

当用户要的不是单个原子组件，而是一整张带视觉叙事的业务卡，应从 `design.md` 的四个结构原型
选择最接近的一种，再按真实内容裁剪。典型组合适合：

- 封面图 + 叠压标题
- 单对象业务摘要
- 指标卡 + 表格 + 图表混排；指标卡结构复用 `../modern-minimal/components/03-surface-metrics/`，业务 pattern 只定义场景顺序
- 洞察 + 巡查关注点 + 下一步动作

如果场景需要封面图和 body 内自定义 hero，不必强行使用根级 `header`；应在场景 pattern 中定义自己的 header 结构。

## 视觉炼化

把 frontend-design 的视觉维度转译为飞书卡片能力：

- Typography -> 文案层级：飞书不能选择字体，因此用标题长度、断行、粗体、引用式短句、模块标题和数值强调来建立字体层次。
- Color & Theme -> 主题承诺：选择一个主色倾向和少量锐利强调，不要做平均、胆怯的彩色拼盘。用 `header.template`、`background_style`、`<font color="...">` 和按钮类型表达。
- Motion -> 静态节奏：飞书不支持页面动效，把动效意图转成首屏冲击、逐段揭示的信息顺序、分割节奏和 CTA 的位置。
- Spatial Composition -> 结构张力：用 `column_set`、单列强调区、模块间距、分栏权重和内容密度制造不那么可预测的构图；仍要保证移动端堆叠后可读。
- Backgrounds & Visual Details -> 氛围细节：飞书不支持自由背景和纹理，把氛围转成模块底色、边界感、图片 `img_key`、短标签、分区标题和少量颜色强调。

## Schema 映射

把 HTML/CSS 设计思路翻译为飞书可表达组件，而不是输出 HTML：

- 页面区块、hero、分屏、面板、网格、标签、CTA、媒体位等概念都要重新组织为 `body.elements`、`markdown`、`column_set`、`table`、`img`、`button` 等 Card JSON 2.0 组件。
- 自定义视觉主题默认优先保持整卡视觉连续性：轻量表格、反馈入口、排行、异常提醒、CTA 等都应尽量放在主题外壳内，避免后半张卡突然掉回飞书默认白底。
- 自定义视觉主题常用 `interactive_container`、`column_set`、`collapsible_panel` 等容器做整卡背景、分区和叙事；这些嵌套容器内不要放原生 `table` 组件，避免触发飞书容器嵌套限制或发送失败。
- 自定义视觉主题中的小型排行、对比、明细、清单型表格，优先用 `markdown` 组件写 Markdown table；需要强调的单元格用加粗、短标签或 `<font color="...">`，不要为了视觉表格强行嵌套 `table`。
- 只有当用户明确需要原生表格能力（冻结首列、分页、数字格式、结构化列类型），且接受它脱离主题外壳时，才使用顶层 `table` 组件；此时不要把它包进 `interactive_container` 或自定义视觉外壳里。
- 自定义视觉主题中的 `form` 也不要放进 `interactive_container`、`column` 或 `collapsible_panel` 等视觉外壳；需要“反馈/报名/申请入口”时，默认在主题外壳内生成带 `behaviors.open_url` 的主题化 `interactive_container` 按钮，链接到 `TODO_FORM_LINK`。只有用户明确要求内联原生表单并接受视觉断层时，才把原生 `form` 作为独立的顶层 `body.elements` 模块。
- 自定义视觉主题的主按钮和按钮组默认不要用飞书原生 `button`，因为原生按钮会回落到平台蓝色/白底/边框样式，不能继承主题 token。优先使用可点击 `interactive_container` 作为 Theme Button；原生 `button` 只用于用户明确要求平台原生按钮、`form_submit`/`form_reset` 或其它必须使用原生按钮的场景。
- 色彩只能使用飞书支持的 `header.template`、`background_style`、`<font color="...">` 等能力。需要暗黑/酷炫时，优先用 `grey`、`indigo`、`purple`、`carmine` 等可用主题和高对比文案近似表达。
- 图表、排行、指标面板等数据可视化区域不要使用纯黑或接近纯黑的大面积底色；dark mode 下会和飞书背景混淆。暗色主题应使用带色相和明度差的深灰蓝、靛蓝、紫蓝、墨绿或暖灰，并确保图例、坐标轴、网格线、数据色和容器边界清晰可见。
- 主题模板的文本颜色尽量使用 `config.style.color` 中自定义的 `rgba(...)` token，并通过 `<font color="theme_token">...</font>` 引用；不要依赖 `default`、`grey`、`blue` 等内置文本色，避免 dark mode 自动翻转导致对比度异常。
- 需要现代极简时，优先用 `header.template: "default"`/`"grey"`、`body.padding`、`column_set.flex_mode: "stretch"`、浅灰 `background_style`、`interactive_container` 的 8px 圆角边框和原生按钮表达；非交互模块不要为了边框使用空回调。
- 需要图片、头像、封面或视觉主图时使用 `img_key`。它可以是当前 App 的真实 key，也可以是
  本素材库中存在的本地 PNG 路径；后者要求 lint 与发送同时使用 `--upload-images`。不直接放 URL。
- 多列布局默认设置 `flex_mode: "stretch"`，保证移动端可读；不要为了桌面视觉牺牲窄屏阅读。
- 有行动入口时使用 `button.behaviors`；没有链接、回调或提交诉求时不强行生成按钮。

### Markdown 表格替代规则

当自定义视觉卡片需要表格感，但当前模块在 `interactive_container`、`column`、`collapsible_panel` 或其它嵌套容器内时，使用 `markdown` 表格：

```json
{
  "tag": "markdown",
  "content": "| 门店 | GMV | 状态 |\\n| --- | ---: | --- |\\n| 仓山奥体中心店 | ¥128,450 | <font color=\"green\">领先</font> |\\n| 台江万达店 | ¥96,420 | 关注客单价 |",
  "text_align": "left",
  "text_size": "notation"
}
```

Markdown 表格适合 2-5 列、3-8 行的轻量展示；超过这个范围应改成“摘要指标 + Top N 列表 + 详情按钮”，或把原生 `table` 放到 `body.elements` 顶层。

表格标题、说明段和 Markdown table 必须拆成多个相邻 `markdown` elements。Markdown table 的 `content` 必须直接从 `| 列名 | ... |` 开始，不要在同一个字符串前面拼 `## 标题`、粗体标题或说明段，避免飞书渲染时标题和表格粘连。

## 输出约束

- 输出必须是有效 Card JSON 2.0，并符合 `../../components.md`。
- 不要输出 HTML、CSS、SVG 或 Markdown 伪装页面。
- 不要使用 schema 未定义的任意 CSS 字段；飞书不支持的视觉效果要用支持组件近似表达。
- 不要声称完整实现网页字体、CSS 动画、渐变背景、遮罩、自由定位等飞书 schema 不支持的效果。
- 不要编造真实链接、真实头像、真实职位、真实数据；不可验证信息使用明确占位符。
- 复杂卡片必须运行 `../../../scripts/lint_card.py` 校验；本地素材追加 `--upload-images`。
