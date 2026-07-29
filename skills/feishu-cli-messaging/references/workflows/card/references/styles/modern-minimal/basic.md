# Modern Minimal 设计 Token 系统

> **核心原则**：AI 只做选择题，不做开放题。
> 每个视觉决策 = 判断角色/状态 → 查 token 表 → 输出值。
> 执行阶段零自由发挥。

本文件定义 6 个 token 维度 + Card Foundation + Quality Gates。组件层（`components.md`）的字段值依赖本文件的 token。文案与信息真实性见 `../../content-quality.md`；schema 必填与字段校验见 `../../components.md`。

## 目录

- [1. 颜色系统](#1-颜色系统)
- [2. 间距系统](#2-间距系统)
- [3. 字体系统](#3-字体系统)
- [4. 表面系统](#4-表面系统)
- [5. 文案规则](#5-文案规则)
- [6. 无障碍](#6-无障碍)
- [7. Card Foundation](#7-card-foundation)
- [8. Quality Gates](#8-quality-gates)

---

## 1. 颜色系统

### 1.1 颜色角色

每种颜色只有一个语义。AI 先判断「这个元素传达什么信息」，再选颜色。

| 角色 | 颜色 token | 用途 | 禁止 |
|---|---|---|---|
| 品牌/主色 | `indigo` | 主标题文字、品牌标签 | 不用于警告/错误 |
| 风险/关注 | `orange` | 异常、待整改、需关注 | 不用于装饰 |
| 信息/中性 | `blue` | 信息标签、普通提示 | 不用于强调 |
| 正向 | `green` | 上升、通过、成功 | 只标记数值/状态词 |
| 负向 | `red` | 下降、拒绝、失败 | 只标记数值/状态词 |
| 次要 | `grey` | 副标题、时间、来源、label | — |
| 中性标签 | `neutral` | 无倾向性标签 | — |
| 反色文字 | `white` | 深色面（grey-900）上的文字 | 只在深色面内使用 |

着色规则：只给数值或状态词上色，不要把指标名称、单位、整段描述一起染色。

### 1.2 背景面

每种背景面对应一个使用场景，不可混用。

| 面 token | 语义 | 唯一用途 |
|---|---|---|
| `default` | 无底色 | 卡片主体、column_set 外层 |
| `grey-50` | 信息面 | KPI 列、信息列、状态卡、折叠面板 |
| `orange-50` | 风险面 | 风险/异常提示容器 |
| `blue-50` | 信息面 | 普通提示、脱敏示例容器 |
| `indigo-50` | 品牌面 | 分组提示、阶段说明容器 |
| `grey-900` | 深色面 | 深色短操作区，内部文字必须反白 |
| `bottom_bg` | 页尾面 | 仅用于 Footer Note，不得他用 |
| `mm_media_item_surface` | 媒体列表项面 | 仅用于 Media List 每条 item 的底色 |
| `mm_media_icon_surface` | 媒体图标容器面 | 仅用于 Media List 图标列表变体的 54px 图标容器 |
| `mm_key_point_blue_surface` | 高亮聚合面（浅蓝） | 仅用于标题区正下方高亮聚合：Split Hero 通栏底色或 Key Point 分栏底色 |
| `mm_key_point_lime_surface` | 高亮聚合面（浅绿） | 仅用于标题区正下方高亮聚合；高亮聚合之外的一切模块禁用 |
| `mm_key_point_lavender_surface` | 高亮聚合面（浅紫） | 仅用于标题区正下方高亮聚合；高亮聚合之外的一切模块禁用 |
| `mm_key_point_icon_surface` | 高亮图标底座面 | 仅用于高亮聚合区（Split Hero / Key Point Columns）的图标底座，静态接近白色 |

不使用渐变、玻璃拟态、阴影、blur；不整卡铺满彩色底。

### 1.3 状态决策矩阵

AI 的颜色决策路径：**判断组件状态 → 查此表 → 输出样式组合**。不要在此表之外发明组合。

| 组件状态      | background_style                | has_border | corner_radius | 文字色                                         | 典型组件                               |
|-----------|---------------------------------|------------|---------------|---------------------------------------------|------------------------------------|
| 信息列（静态展示） | grey-50                         | —          | —             | 默认                                          | column                             |
| 提示条（风险）   | orange-50                       | false      | 8px           | orange 标题                                   | interactive_container              |
| 提示条（信息）   | blue-50                         | false      | 8px           | blue 标题                                     | interactive_container              |
| 提示条（品牌）   | indigo-50                       | false      | 8px           | indigo 标题                                   | interactive_container              |
| 状态卡/明细卡   | grey-50                         | false      | 10px          | 默认                                          | interactive_container              |
| 风险状态卡     | orange-50                       | false      | 10px          | orange 标题                                   | interactive_container              |
| 可点击入口卡    | grey-50                         | true       | 10px          | 默认 + hover_tips                             | interactive_container + behaviors  |
| 深色操作区     | grey-900                        | true       | 10px          | white                                       | interactive_container              |
| 媒体列表项     | mm_media_item_surface           | false      | 12px          | 默认 + grey 摘要                                | interactive_container + column_set |
| 媒体图标容器    | mm_media_icon_surface           | true       | 50px          | img alt                                     | interactive_container + img        |
| 通栏总结高亮块   | mm_key_point_*_surface（任选 1 色）  | false      | 10px          | mm_key_point_ink 结论 + mm_key_point_muted 说明 | interactive_container + column_set |
| 高亮重点分栏    | mm_key_point_*_surface（三色各 1 列） | —          | —             | mm_key_point_ink 标题 + mm_key_point_muted 说明 | column                             |
| 高亮图标底座    | mm_key_point_icon_surface       | true       | 12px          | img alt                                     | interactive_container + img        |
| 折叠面板      | grey-50                         | —          | —             | 默认                                          | collapsible_panel                  |
| 表格头       | grey                            | —          | —             | 粗体                                          | table.header_style                 |
| 主按钮       | —                               | —          | —             | —                                           | button type: primary_filled        |
| 次按钮       | —                               | —          | —             | —                                           | button type: default               |
| 页尾条       | bottom_bg                       | false      | 无             | footer_text + small                         | interactive_container              |

**高亮 / Callout 边界原则**：浅色高亮底需要更强边界感时，采用「浅色底 + 同色系更深一档边框」，不要把 highlight 退回 grey。矩阵中没有的组合（例如给通栏总结高亮块加边框）属于**主题维护动作**：先在 §1.4 补同色系 border token 并同步本矩阵，再使用；不得在单卡生成期自造色值或组合（受 §8 `mm_state_matrix` 门禁约束）。

### 1.4 暗色适配

本主题声明页尾（`bottom_bg`/`footer_text`）、Media List 和高亮聚合区所需的自定义颜色 token，其余颜色使用飞书内建枚举，系统自动适配暗色模式。亮色和暗色共用同一套 token 名称。

```json
{
  "bottom_bg": {
    "light_mode": "rgba(246, 248, 255, 1)",
    "dark_mode": "rgba(10, 17, 41, 1)"
  },
  "footer_text": {
    "light_mode": "rgba(95, 102, 117, 1)",
    "dark_mode": "rgba(170, 180, 204, 1)"
  },
  "mm_media_item_surface": {
    "light_mode": "rgba(247, 247, 245, 1)",
    "dark_mode": "rgba(41, 41, 39, 1)"
  },
  "mm_media_icon_surface": {
    "light_mode": "rgba(255, 255, 255, 1)",
    "dark_mode": "rgba(55, 55, 52, 1)"
  },
  "mm_media_icon_border": {
    "light_mode": "rgba(31, 35, 41, 0.10)",
    "dark_mode": "rgba(255, 255, 255, 0.14)"
  },
  "mm_key_point_blue_surface": {
    "light_mode": "rgba(236, 249, 254, 1)",
    "dark_mode": "rgba(236, 249, 254, 1)"
  },
  "mm_key_point_lime_surface": {
    "light_mode": "rgba(244, 248, 223, 1)",
    "dark_mode": "rgba(244, 248, 223, 1)"
  },
  "mm_key_point_lavender_surface": {
    "light_mode": "rgba(245, 245, 253, 1)",
    "dark_mode": "rgba(245, 245, 253, 1)"
  },
  "mm_key_point_icon_surface": {
    "light_mode": "rgba(255, 255, 255, 1)",
    "dark_mode": "rgba(255, 255, 255, 1)"
  },
  "mm_key_point_icon_border": {
    "light_mode": "rgba(31, 35, 41, 0.10)",
    "dark_mode": "rgba(31, 35, 41, 0.10)"
  },
  "mm_key_point_ink": {
    "light_mode": "rgba(31, 35, 41, 1)",
    "dark_mode": "rgba(31, 35, 41, 1)"
  },
  "mm_key_point_muted": {
    "light_mode": "rgba(100, 106, 116, 1)",
    "dark_mode": "rgba(100, 106, 116, 1)"
  }
}
```

`mm_key_point_*` 是**静态浅色** token：light/dark 同值，暗色模式下不翻转（三色 surface 对应浅蓝 `#ECF9FE`、浅绿 `#F4F8DF`、浅紫 `#F5F5FD`）。因此高亮聚合区内的文字必须显式使用静态深色 `mm_key_point_ink`（标题/结论）和 `mm_key_point_muted`（label/说明），图标底座必须使用静态接近白色的 `mm_key_point_icon_surface` + `mm_key_point_icon_border`（不能借用暗色自适应的 `mm_media_icon_*`），不能用默认自适应文字色或 `grey`，否则暗色模式下会出现浅底浅字或深色底座。仅在卡片实际使用 Split Hero / Key Point Columns 时注入这组 token。

### 1.5 图表主题

`chart.color_theme` 固定使用 `rainbow`。只在有真实数据时生成图表，禁止 ASCII 伪图表。饼图使用实心扇区，**不要设置 `innerRadius`**；不要生成细线圆环。

---

## 2. 间距系统

### 2.1 允许的间距值

本主题只允许以下像素值，不得出现其他数字。这套受限集合保证页面节奏一致。

| 值 | 语义代号 | 含义 |
|---|---|---|
| `-24px` | `title-overlap` | 标题上浮覆盖封面图 |
| `-8px` | `subtitle-overlap` | 副标题上浮紧随标题 |
| `0px` | `flush` | 贴边（封面图、页尾与卡片边缘；同组内元素无额外间距） |
| `4px` | `tight` | 折叠面板与上文的极紧间距；高亮聚合结束后首个普通模块的段落切换 |
| `8px` | `element` | 同组兄弟元素间距；body 全局 spacing |
| `12px` | `related` | 同模块内子块间距；页尾与正文间距 |
| `16px` | `inset` | 面内填充（KPI 信息列、Surface 内部） |
| `20px` | `section` | 内容网格左右边距；新模块顶部间距 |

### 2.2 间距决策路径

AI 先判断两个元素之间的关系，再选间距语义：

```
「这个元素和上一个元素是什么关系？」

 ├─ 与卡片边缘贴齐（图片、页尾）     → flush (0px)
 ├─ 上浮覆盖到上方元素               → title-overlap (-24px) 或 subtitle-overlap (-8px)
 ├─ 同一组件内的兄弟                 → element (8px)
 ├─ 同一模块内的下一个子块           → related (8px 或 12px)
 ├─ 进入一个全新的内容模块           → section (20px)
 └─ 容器/面内部的填充               → inset (12px 或 16px)
```

### 2.3 内容网格

整卡遵循 **20px 内容网格 + 8px 元素间距**。

- 贴边元素（封面图、页尾）：`margin: "0px 0px 0px 0px"` 或 `"12px 0px 0px 0px"`。
- 正文模块：`margin: "Xpx 20px 0px 20px"`，X 按关系选择 `section`/`related`/`element`。
- 页尾内文：`margin: "0px 20px 0px 16px"`。
- 高亮聚合（Split Hero → Key Point Columns）：两者之间的**有效垂直间距必须等于分栏 gutter（8px）**。body 已显式声明 `vertical_spacing: "8px"`，因此 Key Point Columns 用 `margin: "0px 20px 0px 20px"`，不再叠加额外上间距，不依赖飞书默认 sibling spacing 参与计算；高亮聚合结束后的首个普通模块用四值 `margin` 补 `tight`（4px）上间距作为段落切换。

### 2.4 布局默认值

- `body`：`direction: "vertical"`、`horizontal_spacing: "8px"`、`vertical_spacing: "8px"`。
- `column_set`：`flex_mode: "stretch"`、`background_style: "default"`、`horizontal_spacing: "8px"`。
- `column`：`width: "weighted"`、`weight: 1`、`direction: "vertical"`。
- 2 列 = 成对信息，3 列 = KPI 总览；多列必须 `flex_mode: "stretch"` 保证移动端可读。
- 按钮行用 `flex_mode: "flow"`。

---

## 3. 字体系统

### 3.1 角色定义

所有文字先判断角色，角色决定 `text_size` + Markdown 格式。**不要先想字号，先想角色。**

| 角色 | text_size | Markdown 格式 | 用途 |
|---|---|---|---|
| 封面标题 | `heading` | `**<font color='indigo'>标题</font>**` + 可选 pill | 卡片顶部主标题，≤30 字 |
| 副标题 | `normal` | `<font color='grey'>时间 · 对象 · 口径</font>` | 标题下方上下文信息 |
| 章节标题 | `normal` | `**模块标题**` | 分区/模块标题，粗体区分 |
| 正文 | `normal` | 普通文本 | 说明、描述、列表 |
| 指标标注 | `notation` | 普通文本或 `<font color='grey'>label</font>` | KPI label、指标说明、弱信息 |
| 页尾 | `small` | `<font color='footer_text'>来源信息</font>` | Footer Note 或极弱信息 |

### 3.2 决策路径

```
「这段文字是什么角色？」

 ├─ 整张卡片的主标题        → 封面标题 (heading + 粗体 + indigo)
 ├─ 标题下的上下文          → 副标题 (normal + grey)
 ├─ 一个内容区域的标题      → 章节标题 (normal + 粗体)
 ├─ 描述、解释、正文内容    → 正文 (normal)
 ├─ 指标 label、维度、注释  → 指标标注 (notation)
 └─ 来源、免责、系统信息    → 页尾 (small + footer_text)
```

### 3.3 格式约束

- 所有模块统一 `text_align: "left"`，形成统一左对齐阅读动线。
- 标题不使用 `#`（Markdown 原生标题会引入不可控样式）。
- 颜色只标记在数值或状态词上，不要整段染色。
- 上升：`<font color="green">↑ ...</font>`，下降：`<font color="red">↓ ...</font>`。
- 核心数字默认 `**粗体**`；需要特别强视觉时可用 `# **数字**`，但不作为默认做法。
- emoji 只允许作为真实业务标记的少量文本符号（如 ⚠ 配合异常文字说明），不作为主要图标体系。

---

## 4. 表面系统

### 4.1 形态词汇

每种容器形态有固定的样式组合。AI 先判断容器形态，再查此表输出字段。

| 形态   | 载体                      | corner_radius | has_border | padding                                                               | disabled |
|------|-------------------------|---------------|------------|-----------------------------------------------------------------------|----------|
| 信息列  | `column`                | —             | —          | `16px 16px 16px 16px`（信息）或 `12px 12px 12px 12px`（KPI）                 | —        |
| 提示条  | `interactive_container` | `8px`         | `false`    | `12px 8px 12px 8px` 或 `8px 12px 8px 12px`                             | `false`  |
| 状态卡  | `interactive_container` | `10px`        | `false`    | `14px 16px 14px 16px` 或 `12px 16px 12px 16px` 或 `16px 16px 16px 16px` | `false`  |
| 深色块  | `interactive_container` | `10px`        | `false`    | `8px 18px 8px 18px`                                                   | `false`  |
| 媒体列表项 | `interactive_container` | `12px`        | `false`    | `16px 16px 16px 16px`                                                  | `false`  |
| 媒体图标容器 | `interactive_container` | `50px`        | `true`     | `14px 14px 14px 14px`                                                  | `false`  |
| 通栏总结高亮块 | `interactive_container` | `10px`        | `false`    | `14px 16px 14px 16px`                                                  | `false`  |
| 高亮重点分栏 | `column`                | —             | —          | `14px 14px 14px 14px`                                                  | —        |
| 高亮图标底座 | `interactive_container` | `12px`        | `true`     | `12px 12px 12px 12px`                                                  | `false`  |
| 折叠面板 | `collapsible_panel`     | —             | —          | `4px 16px 12px 16px` 或 `12px 16px 12px 16px`                          | —        |
| 页尾条  | `interactive_container` | `""`          | `false`    | `12px 4px 12px 4px`                                                   | `false`  |

### 4.2 决策路径

```
「这个容器是什么形态？」

 ├─ 成对/成组信息展示        → 信息列 (column + grey-50)
 ├─ 标题下的一句话结论高亮    → 通栏总结高亮块 (10px 圆角 + mm_key_point_*_surface)
 ├─ 紧跟其后的 2-3 个重点分栏 → 高亮重点分栏 (column + 三色 mm_key_point_*_surface)
 ├─ 短提示/风险/判断         → 提示条 (8px 圆角 + 语义色面)
 ├─ 有边界的状态/明细块      → 状态卡 (10px 圆角 + 边框 + grey-50)
 ├─ 深色强调操作区           → 深色块 (10px 圆角 + grey-900 + 反白文字)
 ├─ 多条图文/图标列表        → 媒体列表项 (12px 圆角 + mm_media_item_surface)
 ├─ 列表左侧语义图标         → 媒体图标容器 (50px 圆形 + mm_media_icon_surface + mm_media_icon_border)
 ├─ 可收纳的长列表/明细      → 折叠面板 (grey-50 + 默认折叠)
 └─ 页尾来源批注             → 页尾条 (bottom_bg + 贴边)
```

### 4.3 交互约束

- 展示型 `interactive_container` 不配 `behaviors`；真实可点击整块容器必须有 `behaviors`。
- 不要为了装饰写空 callback 或假交互。
- 非交互模块不使用 `interactive_container` 的圆角边框做纯装饰。

---

## 5. 文案规则

### 5.1 按钮文案

必须是「**动作 + 对象**」，让用户一眼知道点击后会发生什么。不能写模糊动词。

| ✓ 正确 | ✗ 错误 | 原因 |
|---|---|---|
| 提交审批 | 提交 | 提交什么？ |
| 驳回申请 | 驳回 | 驳回什么？ |
| 查看明细 | 查看 | 查看什么？ |
| 删除记录 | 删除 | 删除什么？ |
| 下载报告 | 确定 | 完全不知道会发生什么 |

### 5.2 状态信息

**报错** = 「发生了什么 + 该怎么办」：

| ✓ 正确 | ✗ 错误 |
|---|---|
| 文件超过 50MB 限制，请压缩后重新上传 | 上传失败，请重试 |
| 审批已过期，请重新发起 | 操作失败 |
| 构建失败：打包体积超过限制，请缩减依赖或调整阈值 | 构建失败，请稍后重试 |

**成功** = 「发生了什么变化」，不说"成功"二字：

| ✓ 正确 | ✗ 错误 |
|---|---|
| 审批已通过 | 审批成功 |
| 记录已删除 | 删除成功 |
| 报告已发送至邮箱 | 发送成功 |

提示框弹出来本身就说明操作完成了，"成功"是冗余信息。

### 5.3 标签/Pill 文案

- ≤16 字，短促明确。
- 常用模式：状态词（需关注 / 已完成 / 待审批）、场景词（线下巡检 / QSC）、口径词（日均 / 环比）。
- 顶部标题行最多 1 个 pill；正文内最多 3 个 pill。

### 5.4 洞察/结论文案

- 2-4 条 bullet，每条包含「现象 + 解读或建议动作」。
- 不写空泛总结（"整体表现良好，建议继续关注"）。
- 说具体：什么指标、变化了多少、建议谁做什么。

### 5.5 文案精简原则

- 精确，不填充。保持简洁，去掉无信息量的词。
- sentence case（中文语境：不堆叠修饰语，不用"您好"开头）。
- 进行中状态用省略号：`正在部署…`、`保存中…`。
- 跳过"请"和营销最高级。

---

## 6. 无障碍

### 6.1 双通道原则

任何用颜色传达的状态信息，必须同时有文字或图标说明。

| ✓ 正确 | ✗ 错误 |
|---|---|
| `<font color="red">↓ -25%</font>` + 文字"较上月下降" | 只用红色数字，无文字说明 |
| `<text_tag color='orange'>需关注</text_tag>` | 只用橙色背景块，无文字 |
| `<font color="green">↑ 15%</font>` + 文字"环比上升" | 只用绿色数字 |

### 6.2 深色面反白

`grey-900` 背景内文字必须使用 `<font color="white">`。

### 6.3 折叠面板标题

折叠面板标题必须体现数量或收纳对象（如"展开常规问题（21项）"），用户不展开也能获取关键信息。

### 6.4 降级感知

`table`、`chart`、`interactive_container` 为高级组件，在旧客户端或降级场景可能不显示。关键结论必须同时存在于 markdown 文本中，不完全依赖高级组件传达信息。

---

## 7. Card Foundation

### 7.1 根级默认值

```json
{
  "schema": "2.0",
  "config": {
    "update_multi": true,
    "compact_width": false,
    "enable_forward": true,
    "streaming_mode": false,
    "summary": { "content": "TODO_SUMMARY" },
    "style": {
      "color": {
        "bottom_bg": {
          "light_mode": "rgba(246, 248, 255, 1)",
          "dark_mode": "rgba(10, 17, 41, 1)"
        },
        "footer_text": {
          "light_mode": "rgba(95, 102, 117, 1)",
          "dark_mode": "rgba(170, 180, 204, 1)"
        },
        "mm_media_item_surface": {
          "light_mode": "rgba(247, 247, 245, 1)",
          "dark_mode": "rgba(41, 41, 39, 1)"
        },
        "mm_media_icon_surface": {
          "light_mode": "rgba(255, 255, 255, 1)",
          "dark_mode": "rgba(55, 55, 52, 1)"
        },
        "mm_media_icon_border": {
          "light_mode": "rgba(31, 35, 41, 0.10)",
          "dark_mode": "rgba(255, 255, 255, 0.14)"
        }
      }
    }
  },
  "body": {
    "direction": "vertical",
    "horizontal_spacing": "8px",
    "vertical_spacing": "8px",
    "horizontal_align": "left",
    "vertical_align": "top",
    "padding": "0px 0px 0px 0px",
    "elements": []
  }
}
```

### 7.2 规则

- `summary.content` 写卡片主标题或主对象；不要写 `summary.i18n_content`。
- 默认不声明 `width_mode`；确有全宽要求时可加 `"width_mode": "fill"`。
- 默认不声明 `config.style.text_size` 和 `mm_*` 自定义字号 token。
- `body.padding` 默认 `"0px 0px 0px 0px"`；只有没有 Footer Note 且底部是表单/按钮区时，可用 `"0px 0px 16px 0px"` 补底部呼吸感。
- 封面素材选择、上传与标题覆盖布局以 `components.md` 的 Cover Header 为唯一来源；`img.transparent` 固定为 `true`。
- `mm_media_*` token 只供 Media List 使用；不把这些 token 借给普通 Surface Card、Notice Band 或 Footer Note。
- 不使用渐变、玻璃拟态、阴影字段、伪浏览器、伪代码窗口。

> schema 结构与必填/校验不在主题内重复，统一见 `../../components.md` 与 `../../../scripts/lint_card.py`。

---

## 8. Quality Gates

输出前对照以下门禁检查。schema 合法性由 `../../../scripts/lint_card.py` 保证，内容真实性见 `../../content-quality.md`。

### error（必须通过）

- `mm_sample_header`：必须遵循 `components.md` 的 Cover Header：使用 manifest 本地封面图、标题/副标题覆盖布局，且不使用原生 `header`。
- `mm_schema_hygiene`：不得复制样例卡的 schema 缺陷；`img.alt`、`interactive_container.disabled`、`column_set.background_style/flex_mode`、`column.weight`、`plain_text.text_align` 必须齐全；`summary.i18n_content` 不写。
- `mm_bottom_bg`：`bottom_bg`/`footer_text` 只能用于 Footer Note，且色值固定为 §1.4 中的 light/dark RGBA；页尾文字不得回退为 `grey`。
- `mm_state_matrix`：组件的颜色/边框/圆角组合必须出现在 §1.3 状态决策矩阵中，不得自行发明组合。
- `mm_media_img_key`：Media List 内每个 `img` 必须有真实 `img_key`，或有存在的本地 PNG
  路径并在 lint/发送时声明 `--upload-images`；必须补齐 `alt`，不可写 HTTP URL。
- `mm_highlight_zone`：Split Hero / Key Point Columns 的顺序、图标结构与 token 使用边界必须遵循 `components.md` 的高亮聚合总则及对应组件规则。

### warning（应当通过）

- `mm_typography`：默认不用 `mm_*` token；模块字号使用 §3.1 角色定义表中的 `text_size`。
- `mm_surface`：10px 有边界 surface 允许用于状态卡/明细卡/行动卡，但不能写假交互或空 callback。
- `consistent_block_margin`：同一视觉组内的兄弟元素应使用一致 margin，间距值限于 §2.1 允许范围。
- `degrade_awareness`：高级组件（table/chart/interactive_container）的关键结论必须同时存在于 markdown。
- `no_decorative_gradient_or_glass`：不声称或伪造 gradient、glassmorphism、blur、shadow 字段。
- `controlled_emoji`：emoji 只允许少量真实状态文本，不作为主要图标或装饰。
- `a11y_dual_channel`：任何用颜色传达的状态/风险/异常信息必须同时有文字或图标说明。
- `button_action_object`：按钮文案必须是「动作 + 对象」模式，不能写模糊单动词。

输出前使用 `python3 ../../../scripts/lint_card.py --strict <card.json>` 校验卡片；使用预制头图
或图标时追加 `--upload-images`。
