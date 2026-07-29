# Modern Minimal 设计系统

本目录中的数字、日期、标题和文案只演示布局，不是可发送事实。生成业务卡时必须替换为用户提供
或已授权来源中的真实内容，再通过严格 lint。

本主题采用 token 化设计系统：AI 在生成卡片时只做**选择题**（判断角色/状态 → 选 token），不做**开放题**（猜颜色/字号/间距）。所有视觉决策都已在 basic.md 的 token 表中预定义。

## 使用边界

只在用户明确点选以下主题时使用：`modern-minimal`、现代极简、Stripe-like、Linear-like、ElevenLabs-like、SaaS 官网感、企业级平台、API 平台、开发者工具、B2B、克制高级、白底黑字、极简科技。

不要因为内容里出现 KPI、图表、日报、活动、报名、审批等词就自动使用本主题。主题由用户点选；内容只决定需要哪些 schema 组件。

## 读取方式

1. 先读本文件，确定主题边界和组件清单。
2. 必读 `basic.md`——设计 token 系统。一次读入即获取颜色、间距、字体、表面、文案、无障碍的全部决策规则和 Card Foundation 默认值。
3. 必读 `components.md`——原子组件库。根级 `config` 与 `body` 默认值只取自 `basic.md` §7；再按需选取组件，字段值以各组件样式规则为准，样式规则引用 `basic.md` 中的 token。
4. 文案、信息真实性、来源、`TODO_*` 占位符等**内容规范**统一见 `../../content-quality.md`，不在本主题内重复。
5. schema 结构、必填字段、字段校验等**硬性规则**见 `../../components.md`，输出前用 `../../../scripts/lint_card.py` 校验。主题只描述视觉风格，不重复也不覆盖 schema 约束。

## 设计系统概览

basic.md 按以下 token 维度组织，每个维度都提供**决策路径**——AI 先判断状态/角色，再查表取值：

| 维度 | 核心问题 | 决策产出 |
|---|---|---|
| 颜色 | 这个元素传达什么语义？处于什么状态？ | background_style、font color、text_tag color |
| 间距 | 这个元素和上一个元素是什么关系？ | margin、padding 具体像素值 |
| 字体 | 这段文字是什么角色？ | text_size + Markdown 格式 |
| 表面 | 这个容器是什么形态？ | corner_radius、has_border、padding 组合 |
| 文案 | 这段文案的功能是什么？ | 按钮/报错/成功/标签的文案模板 |
| 无障碍 | 这个状态信息有几个感知通道？ | 颜色+文字双通道校验 |

## 原子组件清单

| 组件                    | 位置                                      | 用途                                    |
|-----------------------|-----------------------------------------|---------------------------------------|
| Design Tokens         | `basic.md`                              | 颜色/间距/字体/表面/文案/无障碍 token 系统           |
| Card Foundation       | `basic.md` § Card Foundation            | config、body 默认值                       |
| Quality Gates         | `basic.md` § Quality Gates              | 视觉质量门禁                                |
| Theme Defaults        | `basic.md` § Card Foundation            | config 默认 JSON、bottom_bg/footer_text/media token、summary、body |
| Cover Header          | `components.md` § Cover Header          | manifest 本地封面图 + 分离标题/副标题               |
| Status Pill           | `components.md` § Status Pill           | 内联 `<text_tag>` 状态标签                  |
| Split Hero            | `components.md` § Split Hero            | 标题下通栏总结高亮块，首屏高亮聚合第一段            |
| Key Point Columns     | `components.md` § Key Point Columns     | 2-3 色重点分栏，紧跟 Split Hero 组成高亮聚合      |
| Notice Band           | `components.md` § Notice Band           | 提示色弱提示容器                              |
| Surface Card          | `components.md` § Surface Card          | 10px 圆角状态卡/明细卡                        |
| Metric Section        | `components.md` § Metric Section        | 指标卡组、表格、图表、洞察                         |
| Media List            | `components.md` § Media List            | 图文/图标列表、资源推荐、文章/商品/文件列表              |
| Collapsible Digest    | `components.md` § Collapsible Digest    | 折叠面板收纳区                               |
| Review Form & Actions | `components.md` § Review Form & Actions | 表单、选择器、输入框、提交按钮                       |
| Footer Note           | `components.md` § Footer Note           | bottom_bg + footer_text 页尾批注条            |
| Clickable Surface     | `components.md` § Clickable Surface     | 真实可点击整块入口卡                            |

## 加载边界

`components.md` 是单文件，一次读入即可获取全部原子组件；下表指导**选用**哪些组件。

| 场景              | 选用组件                                                                                                | 避免误用               |
|-----------------|-----------------------------------------------------------------------------------------------------|--------------------|
| 首屏一句话结论 / 核心摘要  | Split Hero，紧跟 Cover Header                                                                          | 不散入正文中段；不做成两张等权 KPI 卡 |
| 内容密集需提炼 2-3 个重点 | Key Point Columns，紧跟 Split Hero 形成高亮聚合                                                              | 不被普通分栏/指标打散；后续模块禁用三色 token |
| 通知、日报、监控播报、审批摘要 | Cover Header + Key Point Columns + Notice Band + Surface Card + Footer Note                        | 不强行补 checker；不堆满图表 |
| 画像/巡检/经营分析报告    | Cover Header + Key Point Columns + Notice Band + Metric Section + Collapsible Digest + Footer Note | checklist 不是默认模块   |
| 表单、审批、任务处理      | Review Form & Actions + Surface Card + Footer Note                                                  | 按钮必须有真实提交/回调含义     |
| 图文列表、能力清单、资源推荐  | Media List + Footer Note                                                                             | 无 `img_key` 时不要伪造图片 |
| 长问题列表、二级明细、补充说明 | Collapsible Digest                                                                                  | 不把低优先级长列表平铺到首屏     |
| 可点击资源卡、入口卡      | Surface Card、Media List 或 Clickable Surface                                                        | 纯展示卡不写假回调          |

## 组合规则

组件的标准顺序、高亮聚合约束与首屏布局只在 `components.md` 开头维护；根级 token、间距和 Quality Gates 只在 `basic.md` 维护。本文件只负责主题选择与组件路由，避免产生第二套规则。
