# 卡片美观设计指南（三板斧）

> 构造 v2 卡片的**美观守则**：配色、布局、节奏。
> 目标：让 Claude 装配出来的卡片视觉上就像专业设计师做的一样，而不是"能看"的程度。

## 目录

1. [一、配色：主色 + 强调色 + 压平色](#一配色主色--强调色--压平色)
2. [二、布局：Z 字视线 + 节奏感](#二布局z-字视线--节奏感)
3. [三、节奏：用 hr + collapsible_panel 组织信息](#三节奏用-hr--collapsible_panel-组织信息)
4. [四、图标策略（含彩色 `_colorful` token 枚举）](#四图标策略)
5. [五、场景 → 模板映射一览](#五场景--模板映射一览)
6. [六、反模式（常见踩坑）](#六反模式常见踩坑)
7. [七、一页速查](#七一页速查)

## 一、配色：主色 + 强调色 + 压平色

### 1.1 header.template 颜色矩阵

header 的 template 就是整张卡片的"第一印象色"。按场景精准选：

| 场景关键词 | template | 选色理由 | 适用卡片 |
|-----------|----------|---------|---------|
| 通知 / 公告 / 同步 / 周报 | `blue` | 专业、可读、最中性 | notification.json |
| 成功 / 完成 / 发布 / 交付 | `green` | 积极、闭环感 | success-report.json |
| 提醒 / 待办 / 审批 | `orange` | 温和的"请注意" | approval.json |
| 警告 / 风险 / 降级 | `yellow` | 比 orange 更轻的提醒 | 降级通知 |
| 错误 / 故障 / 紧急 | `red` | 立即反应 | alert.json |
| 严重事故 / P0 | `carmine` | 比 red 更重，用于顶级告警 | 严重故障 |
| 数据 / 分析 / Dashboard | `purple` / `indigo` | 品牌感、和数据可视化配色协调 | data-dashboard.json |
| 文章 / 文档 / 长文 | `blue` / `wathet` | 阅读友好 | article-summary.json |
| AI / 智能助手 | `violet` / `purple` | 科技感 | llm-streaming.json |
| 已归档 / 已处理 | `grey` | 压低视觉权重，告诉用户"这条不用再看" | 归档消息 |
| 青春 / 新品 / 轻量 | `turquoise` / `lime` | 活泼但不轻佻 | 营销通知 |

**选不定时的兜底**：用 `blue`。

### 1.1.1 与图表系列色保持同家族（统一色板对应）

卡片 UI（header/tag/font）只认命名枚举，图表（chart 组件的 `chart_spec.color`）用
hex —— 两者取自同一套色相家族，同一张卡里 UI 强调色与图表系列色**同家族**才协调。
对应关系（hex 出处见 `feishu-cli-visual` 统一色板，slot 顺序即图表系列取色顺序）：

| slot | 命名枚举 | 图表 hex |
|---|---|---|
| 1 | `blue` | `#3370ff` |
| 2 | `yellow` | `#d99904` |
| 3 | `turquoise` | `#04b49c` |
| 4 | `orange` | `#ed6d0c` |
| 5 | `purple` | `#7f3bf5` |
| 6 | `red` | `#f54a45` |
| 7 | `carmine` | `#f14ba9` |
| 8 | `green` | `#2ea121` |

取用规则：**多系列图永远按固定 slot 顺序**（首系列 `#3370ff` 起），不为迁就
header 打乱顺序 —— 顺序是色盲安全机制；**单系列图**没有身份混淆问题，可取
header 同家族的 hex 做强调色，让卡片观感一体。
例：header `purple` 的 Dashboard 卡，单系列柱状图用 `#7f3bf5`，旁边的多类目
饼图仍按 `#3370ff` 起的固定顺序；header `red` 的告警卡，错误趋势线用 `#f54a45`。
注入方式与校验见 `vchart-quickref.md` 开头"系列配色"。

### 1.2 markdown 内嵌色的三条铁律

```
铁律 1：<font color='red'> 只用来点关键数字、关键状态
铁律 2：一张卡片的主用色总数不超过 3 种
铁律 3：副文本、备注用 <font color='grey'> 压平视觉权重
```

**好的例子**（2 种色 + 1 种灰）：

```markdown
完成率 <font color='green'>96.5%</font>，异常 <font color='red'>3 单</font>。
<font color='grey'>数据更新于 10:00</font>
```

**坏的例子**（5 种色堆砌，视觉混乱）：

```markdown
完成率 <font color='green'>96.5%</font>，异常 <font color='red'>3 单</font>，
待处理 <font color='orange'>12 单</font>，已取消 <font color='purple'>5 单</font>，
今日 <font color='blue'>新增 200</font>。
```

### 1.3 text_tag 标签色的搭配

header.text_tag_list 放 1-3 个标签，颜色按优先级：

| 标签语义 | 色 |
|---------|---|
| 紧急 / P0 | `red` / `carmine` |
| 重要 / P1 | `orange` |
| 进行中 | `blue` |
| 完成 / 通过 | `green` |
| 等待 / 排队 | `yellow` |
| 已过期 / 归档 | `grey` |
| 品牌 / 特殊 | `purple` / `violet` |

---

## 二、布局：Z 字视线 + 节奏感

### 2.1 垂直密度（vertical_spacing 四级节奏）

```
extra_large (16px) — 大章节切换（很少用）
large       (12px) — 小章节之间、图表之间
medium      (8px)  — 默认节奏（body.vertical_spacing 推荐值）
small       (4px)  — 紧密排列的行（如多个 text_tag、连续短行）
```

**实操建议**：
- `body.vertical_spacing: "medium"` 作全局默认
- 需要强分段处插 `{ "tag": "hr" }`，比加大 spacing 更明确
- 不要每个元素都设 margin，让容器统一控节奏

### 2.2 水平密度（column_set）

并排放多个组件时，用 column_set。常见几种搭配：

```
【等分并排】flex_mode: "bisect"（2 列）/ "trisect"（3 列）
【自适应并排】flex_mode: "none" + 每列 width: "weighted" weight: N
【响应式】flex_mode: "flow"（窄屏自动换行）
【垂直堆叠】flex_mode: "stretch"（移动端改为竖排 100%）
```

**黄金比例**（非对称权重）：
- 图 + 文 → 左图 weight:2 / 右文 weight:3
- 主按钮 + 次按钮 → weight:2 / weight:1
- 缩略图列表 → weight: 1 各列

### 2.3 Z 字视线（元素层级）

用户看卡片的眼动轨迹是 Z 字：

```
┌─────────────────────────────┐
│ HEADER（template 色）       │  ← 第一眼：是什么卡片
├─────────────────────────────┤
│ 核心结论 markdown            │  ← 第二眼：TL;DR
│ （带 <font> 强调关键数字）   │
├─────────────────────────────┤
│ ┌─────┐┌─────┐              │  ← 第三眼：关键指标
│ │KV 1 ││KV 2 │ (div.fields) │     (2×2 或 4 格)
│ └─────┘└─────┘              │
├─────────────────────────────┤
│ chart / table               │  ← 第四眼：数据支撑
├─────────────────────────────┤
│ ▼ 折叠面板（次要信息）       │  ← 懒得看可以不看
├─────────────────────────────┤
│ [主按钮] [次按钮] [第三按钮] │  ← 动作区
├─────────────────────────────┤
│ 灰色小字备注                 │  ← 来源 / 时间戳
└─────────────────────────────┘
```

关键原则：**越重要的信息越往上，越次要的越往下或折起来**。

---

## 三、节奏：用 hr + collapsible_panel 组织信息

### 3.1 hr 切章节

- **什么时候用**：两段信息属于不同话题（如"指标"→"详情"→"操作"）
- **什么时候不用**：同一话题内的多个组件（让 spacing 自己做节奏）

### 3.2 折叠面板藏次要

当卡片内容多于 200 字 / 3 屏信息时，优先把次要信息折起来：

```
顶部 3 屏 = 即时可见信息（结论 + 核心指标）
折叠面板 = 详情 / 原因 / 附加说明 / 历史数据
```

**expanded 初始值策略**：
- 默认展开（`true`）：核心内容，不展开就失去意义
- 默认折叠（`false`）：次要详情、超长清单、技术细节

### 3.3 一张"重卡片"的组件搭配

```
header (template=purple)
├─ 副标题 + 3 个 text_tag
│
├─ markdown 摘要
│   └─ <font> 强调核心数字
│
├─ hr
├─ div.fields 2×2 关键指标
├─ hr
├─ column_set (bisect)
│   ├─ column: chart (bar)
│   └─ column: chart (pie)
│
├─ hr
├─ collapsible_panel (expanded=true) "🎯 核心能力"
│   └─ markdown (项目列表)
├─ collapsible_panel (expanded=false) "📋 详细清单"
│   └─ table 或 长 markdown
├─ collapsible_panel (expanded=false) "⚙️ 技术细节"
│   └─ markdown (配 <raw> 包裹代码)
│
├─ hr
├─ column_set (flex_mode=none, 3 列 weight=1 等分)
│   ├─ column: button (primary) "查看"
│   ├─ column: button (default) "忽略"
│   └─ column: button (default) "归档"
│
└─ markdown (text_size=notation, <font color='grey'>) "来源 · 时间戳"
```

---

## 四、图标策略

### 4.1 header.icon

- 每张卡片**最多一个 header icon**
- 用 `standard_icon` + `token`，从飞书图标库选
- 建议：
  - 通知类 → `bell_filled`
  - 成功类 → `check-circle_filled`
  - 警告类 → `warning_filled`
  - 数据类 → `chart_outlined`

**彩色图标（`_colorful` 后缀）**：token 必须从下表按**完整字符串**照抄，**禁止按名称规律自行拼接**
（拼出来的 token 不渲染且不报错，如 `mail_colorful`/`notice_colorful` 都是无效 token）；
没有合适的就省略 icon 或用单色 token。彩色 token 自带颜色，不要再配 `color` 字段。

| 含义 | token | 含义 | token |
|---|---|---|---|
| 日历 | `calendar_colorful` | 待办 | `todo_colorful` |
| 投票 | `vote_colorful` | 飞书妙记 | `file-lark-minutes_colorful` |
| 多维表格 | `wiki-bitable_colorful` | 表单 | `file-form_colorful` |
| 飞书社区 | `larkcommunity_colorful` | 招聘 | `hirelogo_colorful` |
| 飞书品牌 | `lark-logo_colorful` | Meego | `meego_colorful` |
| AI | `myai_colorful` | aPaaS | `apaas_colorful` |
| 审批 | `approval_colorful` | 通用 AI | `ai-common_colorful` |

全量图标枚举（数百个）以官方图标库为准：
https://open.larkoffice.com/document/feishu-cards/enumerations-for-icons

### 4.2 markdown 里的 emoji

正文里用 emoji 代替 icon，更灵活：

```
🎯 核心定位  📊 数据指标  ✨ 新功能  ⚠️ 注意
🔥 最亮眼改进  📋 清单  💡 提示  🚀 发布
🛡️ 安全  💰 价格  📦 版本  📡 来源
✅ 完成  ❌ 失败  🔒 锁定  🔓 解锁
```

**原则**：标题用 1 个 emoji + 短语，段落里零散 emoji 适度点缀，**不要每句都放 emoji**。

### 4.3 折叠面板 icon

collapsible_panel.header.icon 用 `down-small-ccm_outlined`（展开时 icon_expanded_angle=-180 自动翻转）作为视觉提示，告诉用户"这个可以点开"。

---

## 五、场景 → 模板映射一览

| 用户说 | 场景 | 推荐模板 | template 色 | 必备组件 |
|--------|------|---------|------------|---------|
| "发个通知" | 通用通知 | notification.json | blue | header + markdown + div.fields |
| "发版成功了" | 成功报告 | success-report.json | green | header + 指标 div + chart + collapsible 详情 |
| "服务 500 告警" | 告警 | alert.json | red | header(red) + div.fields(服务/级别/时间/影响) + 2 button(查看/忽略) |
| "求审批" | 审批 | approval.json | orange | header(orange) + person 申请人 + markdown 详情 + 3 button(同意/拒绝/查看)+ confirm |
| "给我做个 dashboard" | 数据大屏 | data-dashboard.json | purple | header(purple) + 2 并排 chart + 折叠面板表格 + 4 button |
| "把文章做成卡片" | 文章摘要 | article-summary.json | blue | header + markdown 摘要 + 多个 collapsible_panel 分章节 |
| "AI 生成中的消息" | 流式输出 | llm-streaming.json | violet | config.streaming_mode=true + summary + element_id 定位更新点 |

---

## 六、反模式（常见踩坑）

### ❌ 反模式 1：堆砌组件

```
header + 5 段 markdown + 3 段 div + 2 个 chart + 8 个 button
```
卡片太长用户不看。→ 折叠次要信息到 collapsible_panel。

### ❌ 反模式 2：配色混乱

```
red header + orange text_tag + green font + blue link + purple button
```
→ 确定一个主色（header.template），强调色不超过 2 个。

### ❌ 反模式 3：按钮过多

4 个以上的按钮平铺放在卡片底部，用户不知道该点哪个。→ 最多 3 个按钮，主按钮 primary、次要 default；更多操作放 overflow（⋯）菜单。

### ❌ 反模式 4：没有视觉焦点

每个元素都一样重要，眼睛不知道先看哪里。→ 用 text_size="heading" 或 <font> 强调一处焦点（关键数字、状态）。

### ❌ 反模式 5：没写 template

header 不设 template，默认黑色，看起来像未装修的卡片。→ 任何卡片都要有 template。

---

## 七、一页速查

```
配色：1 主色（template）+ ≤2 强调色 + grey 压平
布局：body.vertical_spacing="medium" + hr 切章节 + column_set 破单调
密度：顶部 3 屏 = 核心，3 屏以下折叠
按钮：≤ 3 个，主 primary，次 default，更多用 overflow
图标：header 一个，markdown 里 emoji 点缀
强调：<font color='red|green'> 只点关键数字
```
