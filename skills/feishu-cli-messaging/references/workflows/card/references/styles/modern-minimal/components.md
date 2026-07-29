# Modern Minimal 原子组件库

本文件汇集所有原子组件。每个组件的字段值引用 `basic.md` 的 token 系统：颜色查 §1、间距查 §2、字体查 §3、表面查 §4、文案查 §5。

> 本文件只定义视觉/组件字段；schema 必填与校验见 `../../components.md` 与 `../../../scripts/lint_card.py`。不要复制样例卡里缺 `alt`、缺 `disabled`、缺 `background_style/flex_mode` 或包含 `summary.i18n_content` 的无效写法。

**标准模块顺序**（按需裁剪、顺序不变）：Cover Header → Split Hero 通栏总结高亮块（可选，紧跟标题区） → Key Point Columns 2-3 色重点分栏（可选，使用时必须紧跟 Split Hero） → 顶部说明 markdown（可选；使用高亮聚合时置于其后，或并入 Split Hero 的一行说明） → Notice Band / Surface Card → Metric Section / Media List / Review Form & Actions / Collapsible Digest → Footer Note。

**高亮聚合总则**：彩色高亮块只允许聚合在标题区正下方，不进入正文列表或后续模块。只有一句话结论或核心摘要 → 只用 Split Hero；内容多需要提炼 2-3 个重点 → Key Point Columns 必须紧跟 Split Hero，形成连续高亮聚合，中间不能插入任何普通分栏、指标或列表；还需要结构化信息 → 在高亮聚合之后使用 Notice Band、Surface Card、Metric Section、Media List 或 Collapsible Digest，统一中性灰阶 surface，禁用 `mm_key_point_*` token。

## 目录

- [Theme Defaults](#theme-defaults)
- [Cover Header](#cover-header)
- [Status Pill](#status-pill)
- [Split Hero](#split-hero)
- [Key Point Columns](#key-point-columns)
- [Notice Band](#notice-band)
- [Surface Card](#surface-card)
- [Media List](#media-list)
- [Metric Section](#metric-section)
- [Collapsible Digest](#collapsible-digest)
- [Review Form & Actions](#review-form--actions)
- [Footer Note](#footer-note)
- [Clickable Surface](#clickable-surface)

---

## Theme Defaults

**作用**：提供组件库所依赖的根级默认配置。

唯一配置来源为 `basic.md` §7「Card Foundation」：从该处复制完整 JSON，再按实际组件补充高亮聚合 token。不要在本文件复制或修改第二份 `config` / `body` 默认值。

---

## Cover Header

**作用**：固定本地封面图 + 独立标题 markdown + 独立副标题 markdown。代替原生 `header`。
**token 引用**：字体 §3.1 封面标题/副标题角色、间距 §2.1 `title-overlap`/`subtitle-overlap`/`flush`。

```json
[
  {
    "tag": "img",
    "img_key": "TODO_IMG_KEY",
    "alt": { "tag": "plain_text", "content": "卡片封面图", "text_align": "left" },
    "preview": false,
    "transparent": true,
    "scale_type": "fit_horizontal",
    "margin": "0px 0px 0px 0px"
  },
  {
    "tag": "markdown",
    "content": "**<font color='indigo'>智能决策工作台日报</font>**<text_tag color='orange'>需关注</text_tag>",
    "text_align": "left",
    "text_size": "heading",
    "margin": "-24px 20px 0px 20px"
  },
  {
    "tag": "markdown",
    "content": "\n<font color='grey'>2026-06-16 09:30 · 审批静默预审批次</font>",
    "text_align": "left",
    "text_size": "normal",
    "margin": "-8px 20px 0px 20px"
  }
]
```

样式规则：

- **NEVER** 使用原生 `header` 字段；本主题顶部只用 Cover Header。
- 从 `../../../assets/headers/modern-minimal/manifest.json` 按语义状态选择本地封面图；未指定状态时
  使用 `header-background-info.png`。可直接把该本地路径写入 `img_key`，lint 与发送时使用
  `--upload-images`；也可先复制到业务目录。不生成新图、不使用图片 URL。
- 封面图全幅贴边：`margin: flush`、`scale_type: "fit_horizontal"`、`transparent: true`；`alt` 必填。
- 标题 `margin: title-overlap`（-24px 20px 0px 20px），`text_size: heading`；内容为粗体 indigo 标题 + 0-1 个 Status Pill。
- 副标题 `margin: subtitle-overlap`（-8px 20px 0px 20px），`text_size: normal`；grey 呈现时间、对象、口径。
- 标题一般 ≤30 字，超长时简化为对象名 + 状态。

---

## Status Pill

**作用**：内联胶囊标签，标记状态或场景。
**token 引用**：颜色 §1.1 角色表、文案 §5.3 标签文案。

样式规则：

- 写法：`<text_tag color='orange'>需关注</text_tag>`。
- 颜色按语义选：`indigo`=主场景、`orange`=风险/待处理、`blue`=信息、`green`=通过、`red`=失败、`neutral`=中性。
- 顶部标题行最多 1 个 pill；正文内最多 3 个。内容 ≤16 字。

---

## Split Hero

**作用**：通栏总结高亮块。承接 Cover Header 的一句话结论、洞察、风险或核心摘要，是首屏高亮聚合的第一段。
**token 引用**：颜色 §1.2 高亮聚合面、§1.3 通栏总结高亮块/高亮图标底座状态、§1.4 静态浅色 token；表面 §4.1；间距 §2.3 高亮聚合规则。

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "height": "auto",
  "corner_radius": "10px",
  "has_border": false,
  "disabled": false,
  "background_style": "mm_key_point_blue_surface",
  "padding": "14px 16px 14px 16px",
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "8px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "margin": "8px 20px 0px 20px",
  "elements": [
    {
      "tag": "column_set",
      "flex_mode": "stretch",
      "background_style": "default",
      "horizontal_spacing": "12px",
      "horizontal_align": "left",
      "columns": [
        {
          "tag": "column",
          "width": "auto",
          "weight": 1,
          "direction": "vertical",
          "horizontal_spacing": "8px",
          "vertical_spacing": "8px",
          "horizontal_align": "left",
          "vertical_align": "top",
          "elements": [
            {
              "tag": "interactive_container",
              "width": "48px",
              "height": "48px",
              "corner_radius": "12px",
              "has_border": true,
              "border_color": "mm_key_point_icon_border",
              "background_style": "mm_key_point_icon_surface",
              "padding": "12px 12px 12px 12px",
              "direction": "vertical",
              "horizontal_spacing": "8px",
              "vertical_spacing": "8px",
              "horizontal_align": "center",
              "vertical_align": "top",
              "disabled": false,
              "elements": [
                {
                  "tag": "img",
                  "img_key": "TODO_MEDIA_ICON_KEY_1",
                  "preview": false,
                  "transparent": true,
                  "scale_type": "crop_center",
                  "size": "24px 24px",
                  "alt": { "tag": "plain_text", "content": "结论图标", "text_align": "left" }
                }
              ]
            }
          ]
        },
        {
          "tag": "column",
          "width": "weighted",
          "weight": 1,
          "direction": "vertical",
          "horizontal_spacing": "8px",
          "vertical_spacing": "4px",
          "horizontal_align": "left",
          "vertical_align": "top",
          "elements": [
            {
              "tag": "markdown",
              "content": "**<font color='mm_key_point_ink'>审批积压已回落至安全水位</font>**",
              "text_align": "left",
              "text_size": "normal"
            },
            {
              "tag": "markdown",
              "content": "<font color='mm_key_point_muted'>近 7 日待办清理率 92%，剩余 8 单今日可完成。</font>",
              "text_align": "left",
              "text_size": "notation"
            }
          ]
        }
      ],
      "margin": "0px 0px 0px 0px"
    }
  ]
}
```

样式规则：

- 必须紧跟 Cover Header 标题区（标题/副标题 markdown）之后，作为首屏第一个内容模块承接标题；不要放到指标、成员、列表之后。
- 不作为正文中段的普通强调块复用；正文需要强调时改用中性灰阶的 Surface Card 或语义色 Notice Band。
- 通栏 surface 任选一个 `mm_key_point_*_surface`（默认 `mm_key_point_blue_surface`），静态浅色、必须比周围画布更可见；若浅色底在预览中融入页面，改选三色中与画布对比更明显的另一色，不要退回 grey；若三色均不可辨，属于主题 token 需要调整——按 §1.3 高亮 / Callout 边界原则补同色系更深一档 token 或 border token（修改 basic.md §1.4 并同步 §1.3 矩阵），不得在单卡生成期自造色值或直接加边框。
- 块内文字必须显式使用 `mm_key_point_ink`（结论，加粗）与 `mm_key_point_muted`（一行说明），不能用默认文字色或 `grey`（§1.4 静态浅色规则）。
- 内部 `column_set` 用 `flex_mode: "stretch"`：左侧 auto 列放图标底座，右侧 weighted 列放短结论 + 一行说明；移动端自然堆叠为图标 → 结论 → 说明。需要双栏对照时，左侧放结论/标题/核心动作，右侧放摘要/指标/风险/补充信息。
- 图标列与文案列之间 gutter 用 `related`（12px，§2.1），宽于默认 8px，用于区分图标底座与文案区。
- 图标必须是 `../../../assets/icons/phosphor/manifest.jsonl` 中的 phosphor / linear 线性 PNG，放入固定浅色 `mm_key_point_icon_surface` + `mm_key_point_icon_border` 图标底座（48px 见方 = 12px padding + 24px 图标，12px 圆角，`preview: false`、`transparent: true`）；禁止 emoji / Unicode 符号充当图标。
- 只有一句话结论时，高亮聚合到此为止；需要继续提炼 2-3 个重点时，Key Point Columns 必须紧跟本模块。

---

## Key Point Columns

**作用**：三色重点分栏。源内容密集时提炼 2-3 个重点/原则/亮点/风险，紧跟 Split Hero 组成标题区正下方的连续高亮聚合。
**token 引用**：颜色 §1.2 高亮聚合面、§1.3 高亮重点分栏/高亮图标底座状态、§1.4 静态浅色 token；间距 §2.3 高亮聚合规则；表面 §4.1。

```json
{
  "tag": "column_set",
  "flex_mode": "stretch",
  "background_style": "default",
  "horizontal_spacing": "8px",
  "horizontal_align": "left",
  "columns": [
    {
      "tag": "column",
      "width": "weighted",
      "weight": 1,
      "background_style": "mm_key_point_blue_surface",
      "padding": "14px 14px 14px 14px",
      "direction": "vertical",
      "horizontal_spacing": "8px",
      "vertical_spacing": "8px",
      "horizontal_align": "left",
      "vertical_align": "top",
      "elements": [
        {
          "tag": "interactive_container",
          "width": "48px",
          "height": "48px",
          "corner_radius": "12px",
          "has_border": true,
          "border_color": "mm_key_point_icon_border",
          "background_style": "mm_key_point_icon_surface",
          "padding": "12px 12px 12px 12px",
          "direction": "vertical",
          "horizontal_spacing": "8px",
          "vertical_spacing": "8px",
          "horizontal_align": "center",
          "vertical_align": "top",
          "disabled": false,
          "elements": [
            {
              "tag": "img",
              "img_key": "TODO_MEDIA_ICON_KEY_1",
              "preview": false,
              "transparent": true,
              "scale_type": "crop_center",
              "size": "24px 24px",
              "alt": { "tag": "plain_text", "content": "效率图标", "text_align": "left" }
            }
          ]
        },
        {
          "tag": "markdown",
          "content": "<font color='mm_key_point_muted'>效率</font>",
          "text_align": "left",
          "text_size": "notation"
        },
        {
          "tag": "markdown",
          "content": "**<font color='mm_key_point_ink'>清理率 92%</font>**",
          "text_align": "left",
          "text_size": "normal"
        },
        {
          "tag": "markdown",
          "content": "<font color='mm_key_point_muted'>较上周提升 11 个百分点。</font>",
          "text_align": "left",
          "text_size": "notation"
        }
      ]
    }
  ],
  "margin": "0px 20px 0px 20px"
}
```

> 示例只展示第 1 列（浅蓝）；第 2、3 列按完全相同的四层结构复制（图标底座 + label + 标题 + 说明各自独立 element），`background_style` 分别换成 `mm_key_point_lime_surface`、`mm_key_point_lavender_surface`，图标与文案按各列语义替换。

样式规则：

- 只在内容密集、需要继续提炼 2-3 个重点/原则/亮点/风险时使用；必须紧跟 Split Hero，中间不能插入任何普通分栏、指标、成员或列表。
- `column_set` 固定 `flex_mode: "stretch"`、`horizontal_spacing: "8px"`，2-3 个 weighted 列。
- 三列背景固定浅色 token：`mm_key_point_blue_surface`（浅蓝 #ECF9FE）、`mm_key_point_lime_surface`（浅绿 #F4F8DF）、`mm_key_point_lavender_surface`（浅紫 #F5F5FD）；只有 2 个分栏时任选其中 2 个，不要补空列。
- 分栏用 `column.background_style` 承载浅色底，不伪造 border；边界感靠 8px gutter、三色浅色差和图标底座细边界形成。
- 与 Split Hero 的有效垂直间距必须等于 gutter（8px）：body 已显式 `vertical_spacing: "8px"`，本模块 `margin: "0px 20px 0px 20px"` 不叠加额外上间距；高亮聚合结束后的首个普通模块用四值 `margin` 补 `tight`（4px）上间距作为段落切换。
- 每列固定四层：图标底座 → 短 label（`notation` + `mm_key_point_muted`）→ 加粗重点标题（`normal` + `mm_key_point_ink`）→ 1 句提炼说明（`notation` + `mm_key_point_muted`）；拆成独立 markdown elements，不把原始长段落塞进分栏。
- 图标底座与 Split Hero 相同（`mm_key_point_icon_surface` + `mm_key_point_icon_border`、12px padding/圆角、24px 图标、`preview: false`、`transparent: true`）；按每列语义从 phosphor manifest 选不同图标，禁止 emoji。
- 三色 token 只服务标题下方高亮聚合区；Notice Band、Surface Card、Metric Section 等后续模块一律禁用 `mm_key_point_*`，统一中性灰阶 surface。
- 图标资产统一从 `../../../assets/icons/phosphor/manifest.jsonl` 按 `tags`/`filekey` 选择同目录 PNG，
  或用 `style_assets.py assets --family phosphor` 检索。直接使用本地路径时，lint 与发送都声明
  `--upload-images`；不要保存或复用跨租户 token。
- 真实资源列表优先 Media List。

---

## Notice Band

**作用**：轻提示/风险/状态判断容器。
**token 引用**：表面 §4.1 提示条形态、颜色 §1.3 提示条状态、间距 §2.1 `related`。

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "height": "auto",
  "corner_radius": "8px",
  "has_border": false,
  "disabled": false,
  "background_style": "orange-50",
  "padding": "12px 8px 12px 8px",
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "8px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "margin": "12px 20px 0px 20px",
  "elements": [
    {
      "tag": "markdown",
      "content": "<font color='orange'>**风险提示**</font>：存在异常阻塞，建议优先复核待确认批次。",
      "text_align": "left",
      "text_size": "normal",
      "margin": "0px 6px 0px 6px"
    }
  ]
}
```

样式规则：

- `basic.md` §1.3 的状态决策矩阵是背景、圆角和边框的唯一来源；本节示例只是提示条的落地写法。
- 按语义选择 `orange-50`（风险）、`blue-50`（信息）或 `indigo-50`（品牌）。
- 每个 Notice Band 只承载 1-2 个 markdown；长内容进 Surface Card 或 Collapsible Digest。

---

## Surface Card

**作用**：10px 圆角状态卡/明细卡。
**token 引用**：表面 §4.1 状态卡/深色块形态、颜色 §1.3 状态卡/深色操作区状态。

普通状态卡：

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "height": "auto",
  "corner_radius": "10px",
  "has_border": false,
  "disabled": false,
  "background_style": "grey-50",
  "padding": "14px 16px 14px 16px",
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "8px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "margin": "12px 20px 0px 20px",
  "elements": [
    {
      "tag": "markdown",
      "content": "**阻塞项**\n<font color='grey'>已识别 8 项异常阻塞，其中 3 项需要补充材料。</font>",
      "text_align": "left",
      "text_size": "normal",
      "margin": "0px 0px 0px 0px"
    }
  ]
}
```

深色操作区：

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "height": "auto",
  "corner_radius": "10px",
  "has_border": false,
  "disabled": false,
  "background_style": "grey-900",
  "padding": "8px 18px 8px 18px",
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "8px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "margin": "0px 0px 0px 0px",
  "elements": [
    {
      "tag": "markdown",
      "content": "<font color='white'>查看明细</font>",
      "text_align": "left",
      "text_size": "normal",
      "margin": "0px 0px 0px 0px"
    }
  ]
}
```

样式规则：

- `basic.md` §1.3 的状态决策矩阵是背景、圆角和边框的唯一来源；本节示例只是状态卡的落地写法。
- 普通 surface 使用 `grey-50`；风险/提示按语义选择 `orange-50`、`blue-50` 或 `indigo-50`。
- 展示型不写 `behaviors`；真实整块点击时必须写有效 `behaviors` 并可加 `hover_tips`。
- 深色面内文字必须反白（无障碍 §6.2）。

---

## Media List

**作用**：图文列表、资源推荐、文章/商品/文件列表、模板库、能力清单。
**token 引用**：表面 §4.1 媒体列表项/媒体图标容器形态、颜色 §1.3 媒体列表项状态、间距 §2.1 `related`/`section`。

图标列表项：

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "height": "auto",
  "corner_radius": "12px",
  "has_border": false,
  "disabled": false,
  "background_style": "mm_media_item_surface",
  "padding": "16px 16px 16px 16px",
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "4px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "margin": "12px 20px 0px 20px",
  "elements": [
    {
      "tag": "column_set",
      "horizontal_spacing": "8px",
      "horizontal_align": "left",
      "flex_mode": "stretch",
      "background_style": "default",
      "columns": [
        {
          "tag": "column",
          "width": "auto",
          "weight": 1,
          "padding": "0px 0px 0px 0px",
          "direction": "vertical",
          "horizontal_spacing": "8px",
          "vertical_spacing": "8px",
          "horizontal_align": "left",
          "vertical_align": "top",
          "elements": [
            {
              "tag": "interactive_container",
              "width": "fill",
              "height": "54px",
              "corner_radius": "50px",
              "has_border": true,
              "border_color": "mm_media_icon_border",
              "background_style": "mm_media_icon_surface",
              "padding": "14px 14px 14px 14px",
              "direction": "vertical",
              "horizontal_spacing": "8px",
              "vertical_spacing": "8px",
              "horizontal_align": "center",
              "vertical_align": "top",
              "disabled": false,
              "elements": [
                {
                  "tag": "img",
                  "img_key": "TODO_MEDIA_ICON_KEY_1",
                  "preview": false,
                  "transparent": true,
                  "scale_type": "crop_center",
                  "size": "24px 24px",
                  "corner_radius": "12px",
                  "alt": { "tag": "plain_text", "content": "审批流程图标", "text_align": "left" }
                }
              ]
            }
          ]
        },
        {
          "tag": "column",
          "width": "weighted",
          "weight": 1,
          "direction": "vertical",
          "horizontal_spacing": "8px",
          "vertical_spacing": "8px",
          "horizontal_align": "left",
          "vertical_align": "top",
          "elements": [
            {
              "tag": "markdown",
              "content": "**审批可以发起、查询和处理**\n<font color='grey'>Agent 可以发起审批，也能查询审批状态、处理待办审批。</font>",
              "text_align": "left",
              "text_size": "normal",
              "margin": "0px 0px 0px 4px"
            }
          ]
        }
      ],
      "margin": "0px 0px 0px 0px"
    }
  ]
}
```

样式规则：

- 用户明确要求多条「带图片的列表项」、资源推荐、文章/商品/文件列表、模板库时使用 Media List；普通文本列表继续用 markdown、Surface Card 或 Collapsible Digest。
- 能力清单、功能列表、Agent 能做什么、带图标说明、icon list、feature list，优先用图标列表变体，不使用大图缩略图。
- 每条 item 外层使用 `interactive_container`，固定 `background_style: "mm_media_item_surface"`、`corner_radius: "12px"`、`has_border: false`、`padding: "16px 16px 16px 16px"`。
- 图标容器使用 54px 圆形面：`height: "54px"`、`corner_radius: "50px"`、`has_border: true`、`background_style: "mm_media_icon_surface"`、`border_color: "mm_media_icon_border"`。
- 图标本身使用透明 PNG：`preview: false`、`transparent: true`、`scale_type: "crop_center"`、`size: "24px 24px"`，并补可读 `alt`。
- 图文列表变体仍使用同样的 `column_set` 结构；左侧 `img` 可用 `scale_type: "crop_center"` 或 `"fit_horizontal"`，右侧标题加粗，摘要用 `<font color='grey'>...</font>`，摘要不超过 2 行。
- 3 条以内可平铺在首屏；超过 5 条改成「精选 3 条 + 查看全部按钮」或进入 Collapsible Digest。
- 图标列表不需要 `hr` 分割；靠统一底色、8px item 间距和 12px 圆角形成节奏。
- 如果整条 item 可点击，把真实 `behaviors.open_url` 加在外层 item 上，并补 `hover_tips`；不要给图标容器单独加跳转。
- `img_key` 使用当前 App 已上传的 key，或使用存在的本地素材路径并声明 `--upload-images`；
  不写 HTTP URL、Base 附件 URL 或跨租户 token。
- 图标资产统一从 `../../../assets/icons/phosphor/manifest.jsonl` 按 `tags`/`filekey` 选择同目录 PNG，
  或用 `style_assets.py assets --family phosphor` 检索。

---

## Metric Section

**作用**：数据驱动板块：章节标题 → KPI 列 → 表格/图表 → 洞察。
**token 引用**：字体 §3.1 章节标题/正文/指标标注角色、间距 §2.1 `section`/`element`/`related`。

章节标题：

```json
{
  "tag": "markdown",
  "content": "**门店经营能力**",
  "text_align": "left",
  "text_size": "heading-3",
  "margin": "20px 20px 0px 20px"
}
```

表格：

```json
{
  "tag": "table",
  "columns": [
    { "data_type": "markdown", "name": "metric", "display_name": "指标", "horizontal_align": "left", "width": "auto" },
    { "data_type": "markdown", "name": "value", "display_name": "数值", "horizontal_align": "left", "width": "auto" },
    { "data_type": "markdown", "name": "compare", "display_name": "对比", "horizontal_align": "left", "width": "auto" }
  ],
  "rows": [
    { "metric": "本月营收", "value": "116.25 万元", "compare": "日均较上月 -25.26%" }
  ],
  "row_height": "auto",
  "header_style": { "text_align": "left", "background_style": "grey", "bold": true },
  "page_size": 6,
  "margin": "8px 20px 0px 20px"
}
```

图表：

```json
{
  "tag": "chart",
  "chart_spec": {
    "type": "bar",
    "data": { "values": [ { "name": "营收日均", "value": -25.26 }, { "name": "客单价", "value": -11.75 } ] },
    "xField": "name",
    "yField": "value",
    "label": { "visible": true },
    "axes": [ { "orient": "left", "title": { "visible": true, "text": "%" } } ]
  },
  "preview": true,
  "color_theme": "rainbow",
  "height": "auto",
  "margin": "8px 20px 0px 20px"
}
```

洞察：

```json
{
  "tag": "markdown",
  "content": "**核心洞察**\n- 本月销售呈下行压力，建议现场关注客流高峰时段及商品结构。\n- 线上渠道偏弱，建议核查履约与平台展示状态。",
  "text_align": "left",
  "text_size": "normal",
  "margin": "12px 20px 0px 20px"
}
```

样式规则：

- 指标少时用「标题 + Key Point Columns + 洞察」即可；不强行补表格和图表。
- `table.header_style` 按状态决策矩阵：`background_style: "grey"`、`bold: true`。
- `chart.color_theme` 固定 `rainbow`（颜色 §1.5）；只在有真实数据时生成。
- 饼图形态遵循 `basic.md` §1.5：使用实心扇区，不设置 `innerRadius`。
- 洞察文案遵循 §5.4：2-4 条 bullet，每条包含「现象 + 建议动作」。
- 除非用户主动要求设置`aspect_ratio`字段或要求图表宽高比，否则不添加该字段

---

## Collapsible Digest

**作用**：收纳长列表、明细、补充说明，减少首屏噪音。
**token 引用**：表面 §4.1 折叠面板形态、颜色 §1.3 折叠面板状态、无障碍 §6.3 标题规则。

```json
{
  "tag": "collapsible_panel",
  "expanded": false,
  "margin": "4px 20px 0px 20px",
  "padding": "4px 16px 12px 16px",
  "background_color": "grey-50",
  "header": {
    "title": { "tag": "plain_text", "content": "展开常规问题（21项）", "text_align": "left" },
    "expanded_title": { "tag": "plain_text", "content": "收起常规问题（21项）", "text_align": "left" },
    "padding": "12px 16px 12px 16px",
    "icon_position": "follow_text",
    "icon_expanded_angle": 180,
    "width": "fill",
    "icon": { "tag": "standard_icon", "token": "down_outlined", "color": "grey" }
  },
  "elements": [
    {
      "tag": "markdown",
      "content": "**物料与证照**\n- 小程序未更新最新营业执照。\n- 部分物料未按要求归档。",
      "text_align": "left",
      "text_size": "normal"
    }
  ]
}
```

样式规则：

- 默认 `expanded: false`；标题必须体现数量或收纳对象（无障碍 §6.3）。
- 背景固定 `grey-50`；外层 margin 可用 `tight`（4px）或 `flush`（0px） + 左右 `section`（20px）。
- `header` 不设置饱和语义色背景；保持继承 `grey-50` 浅色 surface。需要状态色时只调整标题
  markdown、折叠图标或细边框，不生成蓝色、紫色、红色等整条色带。
- 面板内按主题分组，每组一个 `**分组标题**` + bullet 的 markdown。

---

## Review Form & Actions

**作用**：表单与按钮区。
**token 引用**：文案 §5.1 按钮文案规则、间距 §2.1 `section`/`related`、表面 §4.1。

```json
{
  "tag": "form",
  "elements": [
    {
      "tag": "markdown",
      "content": "**当前页面处理**\n<font color='grey'>选择一个重点事项，补充复核备注后确认建议或驳回申请。</font>",
      "text_align": "left",
      "text_size": "normal",
      "margin": "0px 0px 0px 0px"
    },
    {
      "tag": "select_static",
      "name": "decision",
      "required": true,
      "type": "default",
      "width": "fill",
      "placeholder": { "tag": "plain_text", "content": "选择处理结论", "text_align": "left" },
      "options": [
        { "text": { "tag": "plain_text", "content": "确认通过", "text_align": "left" }, "value": "approve" },
        { "text": { "tag": "plain_text", "content": "退回补充", "text_align": "left" }, "value": "return" }
      ]
    },
    {
      "tag": "input",
      "name": "comment",
      "input_type": "multiline_text",
      "rows": 3,
      "label": { "tag": "plain_text", "content": "备注", "text_align": "left" },
      "label_position": "top",
      "placeholder": { "tag": "plain_text", "content": "补充说明", "text_align": "left" },
      "default_value": "",
      "max_length": 300,
      "width": "fill",
      "required": false,
      "margin": "12px 0px 0px 0px"
    },
    {
      "tag": "column_set",
      "flex_mode": "flow",
      "background_style": "default",
      "horizontal_spacing": "8px",
      "horizontal_align": "left",
      "columns": [
        {
          "tag": "column",
          "width": "weighted",
          "weight": 1,
          "vertical_align": "top",
          "elements": [
            {
              "tag": "button",
              "text": { "tag": "plain_text", "content": "提交处理", "text_align": "left" },
              "type": "primary_filled",
              "width": "fill",
              "form_action_type": "submit",
              "name": "Button_submit_review"
            }
          ]
        },
        {
          "tag": "column",
          "width": "weighted",
          "weight": 1,
          "vertical_align": "top",
          "elements": [
            {
              "tag": "button",
              "text": { "tag": "plain_text", "content": "暂不处理", "text_align": "left" },
              "type": "default",
              "width": "fill",
              "form_action_type": "reset",
              "name": "Button_reset_review"
            }
          ]
        }
      ],
      "margin": "0px 0px 0px 0px"
    }
  ],
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "8px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "padding": "20px 20px 0px 20px",
  "margin": "0px 0px 0px 0px",
  "name": "Form_review"
}
```

样式规则：

- 表单前必须有摘要或说明，让用户知道提交对象和当前状态。
- 按钮文案必须遵循 §5.1「动作 + 对象」：如"提交审批"、"驳回申请"，不写"确定"、"取消"。
- 表单容器不声明 schema 不支持的 `width`；控件和按钮用 `width: "fill"`。
- `placeholder`、`label`、`options[].text`、`button.text` 全部补 `text_align: "left"`。
- 按钮 1-2 个；表单内用 `form_action_type: "submit"` / `"reset"`，表单外必须有真实 `callback` 或 `open_url`。没有真实动作时不生成按钮。
- 主按钮 `type: "primary_filled"`，次按钮 `type: "default"`——按状态决策矩阵 §1.3。
- 没有 Footer Note 的表单卡可设 `body.padding: "0px 0px 16px 0px"`。
- 不可逆操作必须加 `confirm`；禁用按钮需 `disabled_tips`。

---

## Footer Note

**作用**：页尾批注条，承载来源、生成链路、免责声明。
**token 引用**：表面 §4.1 页尾条形态、颜色 §1.3 页尾条状态、字体 §3.1 页尾角色。

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "height": "auto",
  "corner_radius": "",
  "has_border": false,
  "disabled": false,
  "background_style": "bottom_bg",
  "padding": "12px 4px 12px 4px",
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "8px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "margin": "12px 0px 0px 0px",
  "elements": [
    {
      "tag": "markdown",
      "content": "<font color='footer_text'>来自：智能决策工作台 · 自动生成</font>",
      "text_align": "left",
      "text_size": "small",
      "margin": "0px 20px 0px 16px"
    }
  ]
}
```

样式规则：

- `background_style` 固定 `bottom_bg`——颜色 §1.2 中的页尾 token；不用 `blue-50` 或 `mm_media_*` 替代。
- 容器贴边：`margin: related`（12px） + 左右 `flush`（0px）。
- 文案字体角色：页尾（`small` + `footer_text`）；不用 `grey` 或默认文字色；不包含图标icon。
- 作为最后一个元素收尾。

---

## Clickable Surface

**作用**：整块可点击的资源/任务/入口卡。视觉延续 Surface Card，但必须有真实行为。
**token 引用**：表面 §4.1 状态卡形态、颜色 §1.3 可点击入口卡状态。

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "height": "auto",
  "corner_radius": "10px",
  "has_border": true,
  "disabled": false,
  "background_style": "grey-50",
  "padding": "14px 16px 14px 16px",
  "direction": "vertical",
  "horizontal_spacing": "8px",
  "vertical_spacing": "8px",
  "horizontal_align": "left",
  "vertical_align": "top",
  "margin": "12px 20px 0px 20px",
  "hover_tips": { "tag": "plain_text", "content": "打开详情", "text_align": "left" },
  "behaviors": [
    { "type": "open_url", "default_url": "TODO_REAL_URL" }
  ],
  "elements": [
    {
      "tag": "markdown",
      "content": "**查看详情**\n<font color='grey'>打开完整审批明细与处理记录。</font>",
      "text_align": "left",
      "text_size": "normal",
      "margin": "0px 0px 0px 0px"
    }
  ]
}
```

样式规则：

- 仅在整块内容确实可点击时使用，`behaviors` 必须非空且真实。
- 非交互展示内容使用 Surface Card，不要写假链接或空 callback。
- 视觉与 Surface Card 一致（状态卡形态），区别仅在于 `behaviors` 和 `hover_tips`。
