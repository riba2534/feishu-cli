---
name: feishu-cli-card
description: >-
  构造美观、元素丰富的飞书 V2.0 交互式卡片（interactive card）。支持折叠面板、
  多栏布局、图表、彩色标签、按钮组、人员卡、流式更新等 20+ 组件，内置 7 个场景模板
  （通知 / 成功报告 / 告警 / 审批 / 数据大屏 / 文章摘要 / AI 流式）和配色布局规范。
  当用户请求"发卡片"、"发通知"、"做告警"、"发报告"、"做审批"、"做 dashboard"、
  "美化消息"、"interactive 卡片"、"v2 卡片"、"带图表的消息"、"带按钮的消息"、
  "带折叠面板的消息"、"飞书卡片"、"Lark card"、"构造卡片"时使用。
  即使用户只说"发个消息告诉 XX"，只要内容有结构（多字段 / 多链接 / 图表 / 状态），
  都应优先用本技能构造卡片而非纯文本。
  构造出的 JSON 写入 /tmp/<name>-card.json，随后交给 feishu-cli-msg 用
  --msg-type interactive 发送（msg 使用 App Token，无需 auth login）。
argument-hint: <场景描述>
user-invocable: true
allowed-tools: Bash(feishu-cli msg:*), Bash(feishu-cli media:*), Read, Write
---

# 飞书 V2 卡片构造技能

> **职责**：把用户的意图 → 构造出视觉丰富的 **v2** 卡片 JSON，写入 `/tmp/<name>-card.json`，
> 再交给 `feishu-cli msg send --msg-type interactive --content-file` 发送。
>
> **只做 v2**：所有输出必须以 `"schema": "2.0"` 开头，用 `body` 包裹 elements。
> v1 相关迁移见 `references/v2-vs-v1.md`。

## 1. 决策树（30 秒选模板）

```
用户意图 → 推荐模板 → header.template
├─ 通知 / 公告 / 信息同步      → notification.json      → blue
├─ 操作完成 / 构建成功 / 交付   → success-report.json    → green
├─ 告警 / 故障 / 错误           → alert.json             → red / carmine
├─ 审批请求 / 待办 / 确认       → approval.json          → orange
├─ 数据展示 / 报表 / 指标       → data-dashboard.json    → purple / indigo
├─ 长文摘要 / 文章总结 / 多章节 → article-summary.json   → blue / wathet
└─ AI 流式输出 / 生成中         → llm-streaming.json     → blue + streaming_mode
```

**不确定时用 `notification.json` 兜底**（蓝色、结构通用、覆盖度最广）。

用户没说具体是什么场景但内容有结构（多字段/多链接/图表/状态指标）——也优先选卡片而不是纯文本。

## 2. v2 最小可运行骨架

```json
{
  "schema": "2.0",
  "config": {
    "update_multi": true,
    "enable_forward": true,
    "width_mode": "fill"
  },
  "header": {
    "template": "blue",
    "title": { "tag": "plain_text", "content": "标题" },
    "subtitle": { "tag": "plain_text", "content": "可选副标题" }
  },
  "body": {
    "direction": "vertical",
    "vertical_spacing": "medium",
    "elements": [
      { "tag": "markdown", "content": "**正文**\n\n支持完整 CommonMark + `<font>`/`<at>`/`<link>` 等 HTML 标签" }
    ]
  }
}
```

**四大必备块**：
1. `schema: "2.0"` — 顶层声明，v2 必须
2. `config` — 全局行为：`update_multi` v2 只支持 `true`；`width_mode` 用 `fill`/`compact`/省略（默认 600px）
3. `header` — 标题区：`template` 定主题色，`title` 必填，`subtitle` 可选
4. `body.elements` — 组件数组，按顺序从上到下渲染

## 3. 工作流（6 步）

1. **选模板**：按决策树确定 `templates/<name>.json`，Read 进来作为起点
2. **填内容**：替换所有 `__PLACEHOLDER__` 标记位（标题、字段值、URL、数据、人员 ID）
3. **选配色**：`header.template` 按场景挑（参见 `references/design.md` 配色矩阵），markdown 内 `<font>` 强调色不超过 3 种
4. **校验**：
   - `python -m json.tool /tmp/xxx-card.json` 过 JSON 解析
   - 检查无 v2 禁用字段（见第 6 节禁区）
5. **落盘**：Write 到 `/tmp/<name>-card.json`（复用已有路径避免污染）
6. **发送**：
   ```bash
   feishu-cli msg send \
     --receive-id-type email \
     --receive-id user@example.com \
     --msg-type interactive \
     --content-file /tmp/<name>-card.json
   ```
   成功返回 `message_id: om_xxx`。

## 4. 组件速查矩阵

**展示类**（只看、不交互）：

| tag | 用途 | 典型属性 |
|-----|------|---------|
| `markdown` | 富文本正文 | `content` + `text_size` + `text_align` + `icon` |
| `div` | 带 fields 的多列键值 | `fields: [{is_short, text}]` |
| `hr` | 分割线 | 无其它属性 |
| `img` | 单图 | `img_key` + `alt` |
| `img_combination` | 多图混排（双/三/四/六/九图） | `combination_mode` + `img_list` |
| `chart` | VChart 图表 | `chart_spec` + `aspect_ratio` + `color_theme` |
| `person` | 单人员 | `user_id` + `show_name` + `show_avatar` |
| `person_list` | 人员列表 | `persons: [{user_id}]` |
| `table` | 表格 | `rows` + `columns` + `row_height` |

**容器类**（装其他组件）：

| tag | 用途 | 关键属性 |
|-----|------|---------|
| `column_set` | 横向分栏 | `flex_mode` + `horizontal_spacing` + `columns` |
| `collapsible_panel` | 折叠面板 | `expanded` + `header.{title,icon,background_color}` + `elements` |
| `form` | 表单容器 | `name` + `elements`（收集一批 input 后一次提交） |
| `interactive_container` | 交互容器 | 统一样式和点击交互，包多个组件 |

**交互类**（可点/可输入）：

| tag | 用途 | 关键属性 |
|-----|------|---------|
| `button` | 按钮 | `type` + `text` + `behaviors: [{type: "open_url"|"callback"}]` |
| `input` | 单行输入 | `name` + `placeholder` + `default_value` |
| `textarea` | 多行输入 | 同 input，多 `rows` |
| `select_static` | 下拉单选 | `options: [{text, value}]` |
| `multi_select_static` | 下拉多选 | 同上，`selected_values` 数组 |
| `date_picker` | 日期 | `initial_date` + `placeholder` |
| `picker_time` | 时间 | 同上 |
| `picker_datetime` | 日期时间 | 同上 |
| `overflow` | 折叠菜单（⋯按钮）| `options: [{text, value, url}]` |

**完整字段、嵌套规则、坑点** → 读 `references/components.md`。

## 5. 美观三板斧（总纲）

### 5.1 配色

- `header.template` 从 13 种里选一种定主色（见 `design.md` 颜色矩阵）：
  - blue = 通知 / 信息 | green = 成功 | orange = 警告 | red = 错误 | grey = 归档 | purple = 品牌/数据
- markdown 内嵌色只用 `<font color='...'>` 点关键数字，**一张卡片主色不超过 3 种**
- 用 `grey` 压副文本视觉权重

### 5.2 布局节奏

- `body.vertical_spacing` 设 `medium`（8px）作默认节奏
- 重要信息 → `large` (12px) 拉大
- 紧凑行（如状态胶囊）→ `small` (4px)
- 场景切换 → 插 `hr` 做硬分割

### 5.3 元素密度（Z 字视线）

```
顶部  │ header（标题 + 副标题 + 最多 3 个 text_tag）
中上  │ markdown 摘要（核心结论，带 1-2 处 <font> 强调）
中部  │ div.fields（2×2 关键指标） 或 column_set（左图右文）
中下  │ chart / table（数据可视化）
底部  │ collapsible_panel（折叠次要信息）
操作区│ column_set 里放 2-3 个 button（primary 放中间）
```

细节展开见 `references/design.md`。

## 6. 禁区（v2 严格校验，未知属性会报错！）

| ❌ 禁止 | ✅ 替代 | 原因 |
|--------|--------|------|
| 顶层 `elements` 直接放 | 放进 `body.elements` | v2 必须 body 包裹 |
| `"tag": "action"` 交互模块 | 按钮直接放 `body.elements`，间距用 spacing | v2 废弃 action 模块 |
| `"tag": "note"` 备注组件 | `markdown` + `<font color='grey'>` + `text_size: "notation"` | v2 废弃 note |
| `config.wide_screen_mode` | `config.width_mode: "fill"` | v1 旧属性 |
| `config.update_multi: false` | `update_multi: true`（或不写） | v2 只支持 true |
| `i18n_elements` | `i18n_content`（局部多语言） | v2 不支持全局多语言 |
| hex 颜色 `#FF0000` | 颜色枚举 `red` 或 `rgba(255,0,0,1)` | v2 不认 hex |
| `[text]($urlVal)` 差异化跳转 | `<link icon='' url='' pc_url='' ...>` 标签 | v2 已删 |
| `stretch_without_padding` 图片通栏 | `margin: "0 -12px"` 负边距 | v2 已删 |
| `flex_mode: "fixed"` | `flex_mode: "none"` 或 `"bisect"/"trisect"` | 枚举不包含 fixed |
| `column.width: 1/2/3` 数字 | `width: "weighted"` + `weight: 1~5` | 列宽语法变了 |
| `collapsible_panel.header.padding: "4px 8px"` 两值 | 写成 `"4px 8px 4px 8px"` 四值 | 服务端对 header padding 严格校验，只接受单值或四值 |

**兜底检查**：写完 JSON 后搜一下 `note|action|wide_screen_mode`，有任何一个都改掉。

## 7. 完整范例（可直接复制）

```json
{
  "schema": "2.0",
  "config": { "update_multi": true, "enable_forward": true, "width_mode": "fill" },
  "header": {
    "template": "blue",
    "title": { "tag": "plain_text", "content": "🚀 本周迭代完成" },
    "subtitle": { "tag": "plain_text", "content": "2026-04-17 发版报告" }
  },
  "body": {
    "direction": "vertical",
    "vertical_spacing": "medium",
    "elements": [
      { "tag": "markdown", "content": "共交付 **12 个功能** / **修复 8 个 Bug**，<font color='green'>全部测试通过</font>。" },
      { "tag": "hr" },
      {
        "tag": "div",
        "fields": [
          { "is_short": true, "text": { "tag": "lark_md", "content": "**📦 版本**\n`v2.3.0`" } },
          { "is_short": true, "text": { "tag": "lark_md", "content": "**⏱ 耗时**\n5 天" } },
          { "is_short": true, "text": { "tag": "lark_md", "content": "**👥 参与**\n7 人" } },
          { "is_short": true, "text": { "tag": "lark_md", "content": "**✅ 状态**\n<font color='green'>已上线</font>" } }
        ]
      },
      {
        "tag": "collapsible_panel",
        "expanded": false,
        "header": {
          "title": { "tag": "markdown", "content": "**📋 详细变更清单（点击展开）**" },
          "background_color": "grey",
          "padding": "4px 8px 4px 8px",
          "icon": { "tag": "standard_icon", "token": "down-small-ccm_outlined", "size": "16px 16px" },
          "icon_position": "right",
          "icon_expanded_angle": -180
        },
        "elements": [
          { "tag": "markdown", "content": "- 新增卡片 V2 构造能力\n- 支持流式更新\n- 修复图表渲染问题" }
        ]
      },
      {
        "tag": "column_set",
        "flex_mode": "none",
        "horizontal_spacing": "8px",
        "columns": [
          {
            "tag": "column", "width": "weighted", "weight": 1, "vertical_align": "center",
            "elements": [{
              "tag": "button",
              "text": { "tag": "plain_text", "content": "📖 发版文档" },
              "type": "primary", "width": "fill",
              "behaviors": [{ "type": "open_url", "default_url": "https://example.com/release" }]
            }]
          },
          {
            "tag": "column", "width": "weighted", "weight": 1, "vertical_align": "center",
            "elements": [{
              "tag": "button",
              "text": { "tag": "plain_text", "content": "📊 监控大盘" },
              "type": "default", "width": "fill",
              "behaviors": [{ "type": "open_url", "default_url": "https://example.com/monitor" }]
            }]
          }
        ]
      },
      { "tag": "markdown", "content": "<font color='grey'>📡 由 feishu-cli-card 技能生成 · v2 schema 2.0</font>" }
    ]
  }
}
```

## 8. 发送衔接

```bash
# 方式 1：直接发给默认接收人（env FEISHU_OWNER_EMAIL 或 user@example.com）
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/my-card.json

# 方式 2：发群
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx \
  --msg-type interactive --content-file /tmp/my-card.json

# 方式 3：带本地图片（--upload-images 自动上传 markdown 里的 ./img.png）
feishu-cli msg send --receive-id-type email --receive-id ... \
  --msg-type interactive --content-file /tmp/my-card.json --upload-images
```

**发送成功标志**：返回 `消息 ID: om_xxxxx`。如果报 `content format error` 或 `invalid element`，
回到第 6 节禁区表逐项检查。

---

## 深度资源

按需 Read：

| 文件 | 什么时候读 | 规模 |
|------|----------|------|
| `references/components.md` | 要用某个组件但不记得字段 / 要查嵌套规则 / 要查颜色枚举 | ~400 行，带 TOC |
| `references/design.md` | 要判断配色 / 布局节奏 / 场景 → 模板映射 | ~250 行 |
| `references/v2-vs-v1.md` | 有 v1 历史代码要迁移 / 要解释用户的 v1 示例为什么跑不通 | ~120 行 |
| `references/vchart-quickref.md` | 要画 chart（bar/pie/line/radar/gauge 等） | ~180 行 |
| `templates/<name>.json` | 7 个开箱即用骨架，每次都先选一个而不是从零写 | 各 80-150 行 |

---

## 和 feishu-cli-msg 的边界

- **feishu-cli-card（本技能）**：**构造** v2 卡片 JSON。职责是"写得漂亮"。
- **feishu-cli-msg**：**发送** 消息（各种类型，含 interactive）。职责是"送得出去"。

构造完成 → 调 msg 发送。两者解耦，不要混在一起写。
