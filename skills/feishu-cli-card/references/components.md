# 飞书 V2 卡片组件速查

> 每个组件包含：tag、最小示例、关键属性表、嵌套规则、常见坑点。
> 官方文档：`https://open.feishu.cn/document/feishu-cards/card-json-v2-components/...`

## 目录

- [卡片级](#卡片级) — schema / config / card_link / header / body
- [展示组件](#展示组件) — markdown / div / hr / img / img_combination / chart / table / person / person_list
- [容器组件](#容器组件) — column_set / collapsible_panel / form / interactive_container
- [交互组件](#交互组件) — button / input / textarea / select_static / multi_select / date_picker / overflow / checker
- [通用布局属性](#通用布局属性-v2-新增) — margin / padding / spacing / align / element_id
- [颜色枚举](#颜色枚举)
- [标题组件 header](#标题组件-header)

---

## 卡片级

### `schema`
- 必须 `"2.0"`，否则按 v1 渲染
- v2 要求飞书客户端 **7.20+**

### `config`
```json
{
  "update_multi": true,              // v2 只支持 true（共享卡片）
  "enable_forward": true,            // 是否允许转发
  "enable_forward_interaction": false, // 转发后是否保留回传交互
  "width_mode": "fill",              // compact(400px) | fill(撑满) | 省略(600px)
  "use_custom_translation": false,   // 是否用自定义翻译
  "streaming_mode": false,           // 流式更新（v2 新增）
  "streaming_config": {              // 仅 streaming_mode=true 时用
    "print_frequency_ms": { "default": 30, "pc": 50 },
    "print_step": { "default": 2 },
    "print_strategy": "fast"          // fast | delay
  },
  "summary": {                       // 聊天列表预览文案
    "content": "AI 生成中...",
    "i18n_content": { "zh_cn": "", "en_us": "" }
  },
  "locales": ["en_us", "ja_jp"],     // 生效的多语言白名单
  "style": {                         // 自定义字号和颜色
    "text_size": { "cus-0": { "default": "medium", "pc": "medium", "mobile": "large" } },
    "color": { "cus-0": { "light_mode": "rgba(5,157,178,0.52)", "dark_mode": "rgba(78,23,108,0.49)" } }
  }
}
```

### `card_link`（卡片整体可点击）
```json
{
  "url": "https://default.example.com",
  "pc_url": "...", "ios_url": "...", "android_url": "..."
}
```

### `body`（正文容器）
```json
{
  "direction": "vertical",          // vertical | horizontal
  "vertical_spacing": "medium",     // small(4px) | medium(8px) | large(12px) | extra_large(16px) | [0,99]px
  "horizontal_spacing": "medium",
  "horizontal_align": "left",       // left | center | right
  "vertical_align": "top",          // top | center | bottom
  "padding": "12px 8px 12px 8px",   // [0,99]px
  "elements": [ /* 组件数组 */ ]
}
```

### 约束
- **单卡片最多 200 个元素**（含 tag=plain_text 的元素）
- 卡片 JSON 总字节数 **≤ 30 KB**

---

## 展示组件

### markdown（富文本，最常用）

```json
{
  "tag": "markdown",
  "element_id": "md_1",
  "content": "# 支持标准 CommonMark\n**粗体** *斜体* ~~删除线~~ `code`\n[链接](url) ![图](img_key)\n- 列表\n| 表 | 格 |\n|---|---|\n\n支持 HTML 标签：\n<font color='red'>红字</font>\n<text_tag color='blue'>标签</text_tag>\n<at id='ou_xxx'></at> / <at id='all'></at>\n<person id='ou_xxx' show_name=true show_avatar=true style='normal'></person>\n<link icon='chat_outlined' url='https://...' pc_url='' ios_url='' android_url=''>带图标链接</link>\n<local_datetime millisecond='1635360000000' format_type='date_num'></local_datetime>\n<number_tag background_color='grey' font_color='white'>1</number_tag>",
  "text_size": "normal",            // 见字号枚举
  "text_align": "left",             // left | center | right
  "icon": {
    "tag": "standard_icon",
    "token": "chat-forbidden_outlined",
    "color": "blue"
  },
  "margin": "0"
}
```

**text_size 枚举**（15 种）：
- 语义化：`heading-0`(30px) / `heading-1`(24) / `heading-2`(20) / `heading-3`(18) / `heading-4`(16) / `heading`(16) / `normal`(14) / `notation`(12)
- 数值化：`xxxx-large`(30) / `xxx-large`(24) / `xx-large`(20) / `x-large`(18) / `large`(16) / `medium`(14) / `small`(12) / `x-small`(10)

**支持的 HTML 标签**（仅列 v2 可用）：
- `<br>` / `<br/>` 换行
- `<hr>` / `<hr/>` 分割线
- `<font color='enum或rgba'>...</font>` 颜色，支持嵌套
- `<at id='open_id|all'></at>` @某人或全员
- `<person id='user_id' show_name=true show_avatar=true style='normal|capsule'></person>` 人员卡（需配合 v2 客户端）
- `<link icon='' url='' pc_url='' ios_url='' android_url=''>...</link>` 带图标的差异化链接
- `<text_tag color=''>...</text_tag>` 行内标签
- `<local_datetime millisecond='' format_type=''></local_datetime>` 本地时区时间戳
- `<number_tag background_color='' font_color=''>N</number_tag>` 数字角标（1-99）
- `<a href='url'>...</a>` 纯文本链接（可以用 `[text](url)` 替代）
- `<raw>...</raw>` 原样输出，不解析 Markdown

**坑点**：
- 软换行（1 个 Enter）可能被忽略，硬换行（2 个 Enter）总是生效
- `<font color=''>` 支持的颜色范围比 lark_md 大，但 hex 还是不认
- 差异化跳转 **必须** 用 `<link>` 标签，旧 `[text]($urlVal)` 语法 v2 已删

### div（带 fields 的多列键值）

```json
{
  "tag": "div",
  "text": { "tag": "lark_md", "content": "主内容" },
  "fields": [
    { "is_short": true, "text": { "tag": "lark_md", "content": "**字段 1**\n值 1" } },
    { "is_short": true, "text": { "tag": "lark_md", "content": "**字段 2**\n值 2" } },
    { "is_short": false, "text": { "tag": "lark_md", "content": "**长字段**\n占满整行" } }
  ],
  "icon": { "tag": "standard_icon", "token": "info_filled", "color": "blue" }
}
```

- `is_short: true` 占一半宽度（两列并排）；`false` 占满整行
- 推荐 2×2 或 4 个并排展示关键指标，不要超过 6 个

### hr（分割线）
```json
{ "tag": "hr" }
```
无其它属性。

### img（单图）
```json
{
  "tag": "img",
  "img_key": "img_v2_xxx",        // 上传图片 API 返回
  "alt": { "tag": "plain_text", "content": "描述" },
  "mode": "fit_horizontal",        // fit_horizontal | crop_center
  "preview": true,                 // 点击大图预览
  "size": "medium",                // small | medium | large 或省略自适应
  "margin": "0 0 0 0"
}
```
- v2 **不支持** `stretch_without_padding` 通栏；通栏用 `margin: "0 -12px"` 负边距实现

### img_combination（多图混排）

把 2-9 张图按预设布局拼成一组展示，避免手写 `column_set` + 多个 `img` 的繁琐布局。

```json
{
  "tag": "img_combination",
  "element_id": "imgc_1",
  "combination_mode": "bisect",       // double | triple | bisect | trisect | nine
  "corner_radius": "5px",              // 圆角，[0,99]px
  "img_list": [
    { "img_key": "img_v2_xxx1" },
    { "img_key": "img_v2_xxx2" },
    { "img_key": "img_v2_xxx3" },
    { "img_key": "img_v2_xxx4" }
  ],
  "margin": "0"
}
```

**combination_mode 枚举与图片数量**：

| mode | 布局 | 图片数 |
|------|------|--------|
| `double` | 双图并排 | 必须 2 张 |
| `triple` | 三图（左大右两小） | 必须 3 张 |
| `bisect` | 四宫格（2×2） | 必须 4 张 |
| `trisect` | 六宫格（2×3） | 必须 6 张 |
| `nine` | 九宫格（3×3） | 必须 9 张 |

**坑点**：
- `img_list` 长度必须与 `combination_mode` 严格匹配，多/少都会渲染失败
- 每张图需先通过 `feishu-cli media upload` 上传得到 `img_key`
- 客户端 7.4+ 才支持；老版本会回退为单图列表

### chart（图表）

```json
{
  "tag": "chart",
  "element_id": "chart_1",
  "aspect_ratio": "16:9",          // 1:1 | 2:1 | 4:3 | 16:9
  "height": "auto",                 // auto 或 [100,999]px
  "color_theme": "brand",           // brand | rainbow | complementary | ...
  "preview": true,                  // 是否支持独立窗口查看
  "chart_spec": {
    "type": "bar",                  // bar | line | area | pie | scatter | radar | gauge | funnel | wordCloud | combination
    "data": { "values": [{ "x": "A", "y": 10 }] },
    "xField": "x",
    "yField": "y",
    "label": { "visible": true }
    /* VChart 完整 spec 见 references/vchart-quickref.md */
  }
}
```

**嵌套规则**：
- chart 支持在 `body.elements`、`column_set.columns[].elements`、`collapsible_panel.elements`、`interactive_container.elements` 中使用
- **单卡片建议不超过 5 个 chart**（性能限制）
- 移动端不支持纹理填充 / conical 渐变 / grid 词云

### table（表格）

```json
{
  "tag": "table",
  "page_size": 5,                   // 分页大小
  "row_height": "low",              // low | middle | high
  "header_style": {
    "background_style": "grey",
    "text_align": "center",
    "text_size": "normal",
    "bold": true
  },
  "columns": [
    { "name": "col1", "display_name": "姓名", "data_type": "lark_md", "horizontal_align": "left", "width": "auto" },
    { "name": "col2", "display_name": "状态", "data_type": "options", "width": "80px" }
  ],
  "rows": [
    { "col1": "张三", "col2": [{ "text": "完成", "color": "green" }] },
    { "col1": "<at id='ou_xxx'></at>", "col2": [{ "text": "进行", "color": "orange" }] }
  ]
}
```

**data_type 枚举**：`lark_md` / `plain_text` / `options`（彩色标签）/ `number` / `person` / `person_list` / `date` / `markdown`

**坑点**：
- table **不能**嵌在 `column_set` / `collapsible_panel` / `form` 里（实测报错：`type of element is not supported tag: table`）
- table 只能放 `body.elements` 直接层，或 `interactive_container.elements` 里
- 要在折叠面板/分栏里展示表格 → 用 `markdown` 表格语法（`| 列1 | 列2 |\n|---|---|`）替代
- `rows` 里的 key 必须和 `columns.name` 完全一致

### person（单人员卡）

```json
{
  "tag": "person",
  "user_id": "ou_xxx",
  "user_id_type": "open_id",        // open_id | user_id | union_id
  "style": "normal",                // normal | capsule
  "show_name": true,
  "show_avatar": true,
  "size": "medium"                  // extra_small | small | medium | large
}
```
也可以用 markdown 里的 `<person>` 标签内嵌。

### person_list（人员列表）
```json
{
  "tag": "person_list",
  "persons": [
    { "user_id": "ou_1" },
    { "user_id": "ou_2" }
  ],
  "show_name": true,
  "show_avatar": true,
  "size": "medium"
}
```

---

## 容器组件

### column_set（分栏，布局骨架）

```json
{
  "tag": "column_set",
  "element_id": "cols_1",
  "flex_mode": "none",              // none | stretch | flow | bisect | trisect
  "horizontal_spacing": "8px",      // small(4) | medium(8) | large(12) | extra_large(16) | [0,99]px
  "horizontal_align": "left",
  "background_style": "default",    // default | 颜色枚举 | rgba
  "margin": "0",
  "action": {                        // 可选：点整个分栏跳转
    "multi_url": { "url": "...", "pc_url": "...", "ios_url": "...", "android_url": "..." }
  },
  "columns": [
    {
      "tag": "column",
      "width": "weighted",            // auto | weighted | [16,600]px
      "weight": 1,                    // 1-5 整数，仅 width=weighted 生效
      "background_style": "default",
      "vertical_align": "top",        // top | center | bottom
      "horizontal_align": "left",
      "vertical_spacing": "8px",
      "horizontal_spacing": "8px",
      "direction": "vertical",
      "padding": "8px",
      "margin": "0",
      "elements": [ /* 不能嵌 form 和 table */ ]
    }
  ]
}
```

**flex_mode 枚举（窄屏自适应策略）**：
- `none`：不自适应，按比例压缩
- `stretch`：窄屏改竖排，每列 100% 宽
- `flow`：流式换行，放不下的列往下换行
- `bisect`：两列等分
- `trisect`：三列等分

**嵌套规则**：
- 最多嵌 **5 层**
- `column.elements` 不能放 `form` 和 `table`
- 嵌套时上层 `background_style` 覆盖下层

**最常用组合**：
- 并排按钮 → `flex_mode: "none"` + 每列 `width: "weighted"` `weight: 1`
- 左右图文 → `flex_mode: "none"` + 左 `weight: 2` 右 `weight: 3`
- 纯等分 → `flex_mode: "bisect"` 或 `"trisect"`

### collapsible_panel（折叠面板）

```json
{
  "tag": "collapsible_panel",
  "element_id": "panel_1",
  "expanded": false,                 // 初始是否展开
  "direction": "vertical",
  "vertical_spacing": "8px",
  "padding": "8px",
  "background_color": "grey",        // 整体背景色（颜色枚举或 rgba）
  "border": {                        // 可选边框
    "color": "grey",
    "corner_radius": "5px"
  },
  "header": {
    "title": {                       // 支持 plain_text 或 markdown（含 lark_md）
      "tag": "markdown",
      "content": "**面板标题**"
    },
    "background_color": "grey",
    "vertical_align": "center",
    "padding": "4px 8px",
    "position": "top",               // top | bottom（标题位置）
    "width": "fill",                 // fill | auto | auto_when_fold（V7.32+）
    "icon": {
      "tag": "standard_icon",
      "token": "down-small-ccm_outlined",
      "color": "white",
      "size": "16px 16px"
    },
    "icon_position": "right",        // left | right | follow_text
    "icon_expanded_angle": -180      // 展开时旋转角度，正顺时针负逆时针
  },
  "elements": [
    { "tag": "markdown", "content": "折叠内容" }
  ]
}
```

**嵌套规则**：
- 不能嵌 `form`
- 其它所有组件都可以嵌入

**坑点**：
- 折叠面板 **不支持搭建工具**，必须手写 JSON
- `header.title.tag` 必须是 `plain_text` 或 `markdown`，**不是** `lark_md`
- `header.width: auto_when_fold` 需要客户端 7.32+
- `header.padding` **不接受两值格式** `"4px 8px"`，必须写单值 `"8px"` 或四值 `"4px 8px 4px 8px"`。
  服务端报错：`invalid panel header padding`

### form（表单容器）

```json
{
  "tag": "form",
  "name": "my_form",                 // 表单唯一标识
  "element_id": "form_1",
  "padding": "8px",
  "elements": [
    { "tag": "input", "name": "user_name", "placeholder": { "tag": "plain_text", "content": "姓名" } },
    { "tag": "date_picker", "name": "birthday" },
    {
      "tag": "button",
      "text": { "tag": "plain_text", "content": "提交" },
      "type": "primary",
      "form_action_type": "submit",  // submit | reset（仅 form 内生效）
      "behaviors": [{ "type": "callback", "value": { "action": "submit" } }]
    }
  ]
}
```

**嵌套规则**：不能嵌 `form`、`table`、`chart`；form 内 button 需设 `form_action_type`。

### interactive_container（交互容器）

```json
{
  "tag": "interactive_container",
  "element_id": "ic_1",
  "width": "fill",                  // fill | auto | [16,999]px
  "padding": "8px",
  "background_style": "grey",
  "border": { "color": "grey", "corner_radius": "5px" },
  "behaviors": [{ "type": "open_url", "default_url": "https://..." }],
  "hover_tips": { "tag": "plain_text", "content": "点击跳转" },
  "elements": [ /* 可嵌其它组件 */ ]
}
```

用于把多个组件打包成一个可点击单元（如整个"卡片 + 按钮"区域一起点）。

---

## 交互组件

### button（按钮）

```json
{
  "tag": "button",
  "element_id": "btn_1",
  "text": { "tag": "plain_text", "content": "点击" },
  "type": "primary",                 // default | primary | danger | text | primary_text | danger_text | primary_filled | danger_filled | laser
  "size": "medium",                  // tiny | small | medium | large
  "width": "default",                // default | fill | [100,∞)px
  "icon": { "tag": "standard_icon", "token": "send_outlined", "color": "white" },
  "hover_tips": { "tag": "plain_text", "content": "PC 端 hover 提示" },
  "disabled": false,
  "disabled_tips": { "tag": "plain_text", "content": "禁用原因" },
  "confirm": {                       // 二次确认弹窗
    "title": { "tag": "plain_text", "content": "确认删除？" },
    "text": { "tag": "plain_text", "content": "不可撤销" }
  },
  "behaviors": [
    {
      "type": "open_url",
      "default_url": "https://...",
      "pc_url": "...", "ios_url": "...", "android_url": "..."
    },
    {
      "type": "callback",
      "value": { "action": "xxx", "id": 123 }
    }
  ]
}
```

**type 视觉区分**：
- `default` 黑字边框 | `primary` 蓝字边框 | `danger` 红字边框
- `_text` 无边框变体
- `_filled` 填充色变体（primary_filled 蓝底白字）
- `laser` 镭射高亮

**behaviors 策略**：
- 纯跳转 → 一个 `open_url`
- 仅回传 → 一个 `callback`
- 跳转 + 埋点 → `open_url` + `callback` 两个都写

### input（单行输入）

```json
{
  "tag": "input",
  "element_id": "input_1",
  "name": "field_name",              // form 内使用
  "placeholder": { "tag": "plain_text", "content": "请输入" },
  "default_value": "",
  "width": "fill",                   // default | fill | [0,999]px
  "max_length": 100,
  "input_type": "text",              // text | password
  "required": false,
  "label": { "tag": "plain_text", "content": "标签" },
  "label_position": "top",           // top | left
  "behaviors": [{ "type": "callback", "value": { "action": "input" } }]
}
```

### textarea（多行）
同 input，额外 `rows`（默认 3）。

### select_static / multi_select_static

```json
{
  "tag": "select_static",            // 或 multi_select_static
  "element_id": "sel_1",
  "name": "city",
  "placeholder": { "tag": "plain_text", "content": "选择城市" },
  "initial_option": "bj",            // 默认选中 value（multi 用 initial_options 数组）
  "width": "fill",
  "options": [
    { "text": { "tag": "plain_text", "content": "北京" }, "value": "bj", "icon": { /* ... */ } },
    { "text": { "tag": "plain_text", "content": "上海" }, "value": "sh" }
  ],
  "behaviors": [{ "type": "callback", "value": { "key": "city" } }]
}
```

### date_picker / picker_time / picker_datetime

```json
{
  "tag": "date_picker",              // 或 picker_time / picker_datetime
  "element_id": "dp_1",
  "name": "due_date",
  "placeholder": { "tag": "plain_text", "content": "选择日期" },
  "initial_date": "2026-04-17",      // picker_time 用 initial_time "HH:mm"；datetime 用 initial_datetime "yyyy-MM-dd HH:mm"
  "width": "fill",
  "behaviors": [{ "type": "callback", "value": { "field": "due_date" } }]
}
```

### overflow（⋯折叠菜单）

```json
{
  "tag": "overflow",
  "element_id": "ov_1",
  "options": [
    {
      "text": { "tag": "plain_text", "content": "查看" },
      "value": "view",
      "icon": { "tag": "standard_icon", "token": "eye_outlined" }
    },
    {
      "text": { "tag": "plain_text", "content": "删除" },
      "value": "delete",
      "confirm": {
        "title": { "tag": "plain_text", "content": "确认？" },
        "text": { "tag": "plain_text", "content": "不可撤销" }
      }
    }
  ],
  "behaviors": [{ "type": "callback", "value": { "menu": "overflow_1" } }]
}
```

### checker（勾选器）
仅 JSON 可用，不在搭建工具里。用于任务列表勾选打钩。

```json
{
  "tag": "checker",
  "checked": false,
  "text": { "tag": "plain_text", "content": "完成这件事" },
  "overall_checkable": true,
  "button_area": {
    "buttons": [
      { "tag": "button", "text": { /* ... */ }, "behaviors": [/* ... */] }
    ]
  },
  "behaviors": [{ "type": "callback", "value": { "check": "task_1" } }]
}
```

---

## 通用布局属性（v2 新增）

所有组件（除 header）都支持这批属性：

| 属性 | 类型 | 范围 | 说明 |
|------|------|------|------|
| `margin` | String | [-99,99]px | 外边距：`"10px"` / `"4px 0"` / `"4px 0 4px 0"` |
| `padding` | String | [0,99]px | 内边距（容器组件可用） |
| `direction` | String | vertical / horizontal | 子组件排列方向 |
| `vertical_spacing` | String | small/medium/large/extra_large 或 [0,99]px | 子组件垂直间距 |
| `horizontal_spacing` | String | 同上 | 子组件水平间距 |
| `vertical_align` | String | top/center/bottom | 子组件垂直对齐 |
| `horizontal_align` | String | left/center/right | 子组件水平对齐 |
| `element_id` | String | 字母开头 ≤ 20 字符 | 唯一标识，用于流式/更新接口 |

**spacing 数值对应表**：small=4px, medium=8px, large=12px, extra_large=16px

### element_id 的后端用途：流式更新

为组件设置 `element_id` 后，可通过 **CardKit Open API** 在消息发送后动态更新单个元素，无需重发整张卡片，适用于 AI 流式输出、状态推进等场景：

```http
PATCH /open-apis/cardkit/v1/cards/:card_id/elements/:element_id
Content-Type: application/json

{
  "partial_update": true,           // true=增量追加，false=整体覆盖
  "element": {
    "tag": "markdown",
    "content": "新追加的内容片段"
  }
}
```

- `:card_id` 是消息发送后返回的卡片 ID（不是 `message_id`，需用 `/cardkit/v1/cards` 创建/绑定）
- `:element_id` 即组件 JSON 里声明的 `element_id` 值
- 配合 `config.streaming_mode: true` 和 `streaming_config.print_strategy` 实现打字机效果
- 详细 API 文档：`https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/cardkit-v1/card-element/patch`

---

## 颜色枚举

所有 `color` 字段（含 `<font>`、header.template、background_color 等）支持以下值：

**语义化**：
- `default`（主题默认）
- `neutral` / `grey` / `black` / `white`
- `blue` / `wathet`（浅蓝）/ `turquoise`（青）/ `indigo`（靛）
- `green` / `lime`
- `yellow` / `orange`
- `red` / `carmine`
- `purple` / `violet`

**RGBA**：`"rgba(255, 0, 0, 0.8)"` 或 `"rgba(255,0,0,1)"`

**深色模式**：通过 `config.style.color.cus-0.{light_mode, dark_mode}` 定义自定义色对。

**坑点**：**不接受 hex**（`#FF0000` 不行）。

---

## 标题组件 header

```json
{
  "header": {
    "title": { "tag": "plain_text", "content": "主标题（必填）" },
    "subtitle": { "tag": "plain_text", "content": "副标题" },
    "text_tag_list": [
      {
        "tag": "text_tag",
        "text": { "tag": "plain_text", "content": "紧急" },
        "color": "red"
      }
    ],
    "template": "blue",                // 13 种主题色 + default
    "icon": {
      "tag": "standard_icon",          // 或 custom_icon
      "token": "chat-forbidden_outlined",
      "color": "orange",
      "img_key": "img_v2_xxx"          // 仅 custom_icon
    },
    "padding": "12px 8px"
  }
}
```

**template 枚举（13 种）**：
`blue` / `wathet` / `turquoise` / `green` / `yellow` / `orange` / `red` / `carmine` / `violet` / `purple` / `indigo` / `grey` / `default`

**text_tag_list**：最多 3 个标签，超出不显示。color 支持的枚举同上。

**多语言**：用 `i18n_text_tag_list` 替代 text_tag_list，key 用语种代码（`zh_cn` / `en_us` / `ja_jp` / `zh_hk` / `zh_tw`）。

---

## 图标 token 来源

`icon.token` 来自飞书图标库：
- 官方文档：`https://open.feishu.cn/document/feishu-cards/enumerations-for-icons`
- 命名约定：末尾 `_outlined` 是线性图标，`_filled` 是面性图标
- 常用图标举例（仅示意，用前请在图标库确认）：`chat_outlined` / `info_filled` / `warning_filled` / `check-circle_filled` / `close-circle_filled` / `send_outlined` / `eye_outlined` / `down-small-ccm_outlined` / `up-small-ccm_outlined`

---

## 嵌套规则速查

| 容器 | 可嵌入 | 不可嵌入 |
|------|--------|---------|
| `body` | 所有组件 | — |
| `column_set > column` | 其它所有（含子 column_set） | form, table |
| `collapsible_panel` | 其它所有 | form, **table**（实测） |
| `form` | input / textarea / select / date_picker / button / markdown / div / column_set | form, table, chart |
| `interactive_container` | 所有组件 | form（建议）|

> **table 组件的特殊限制**：**只能**放在 `body.elements` 直接层或 `interactive_container` 里。
> 要在折叠面板/分栏里展示表格数据，用 markdown 的 `| 列 | 列 |` 表格语法替代。

**通用上限**：容器嵌套不超过 5 层；单卡 ≤ 200 组件；JSON ≤ 30KB。

---

## 进阶：自定义样式 与 多语言

- **自定义字号/颜色**：在 `config.style.text_size.cus-N` / `config.style.color.cus-N` 定义后，组件 `text_size`、`color` 字段引用 `cus-N` 即可全卡复用。颜色项支持 `light_mode` / `dark_mode` 双值，自动适配深色模式。
- **多语言**：v2 已废弃 v1 的 `i18n_elements`（全局多语言），改用**局部多语言**：单组件文本字段写成 `i18n_content: { "zh_cn": "...", "en_us": "...", "ja_jp": "..." }`；header 的 text_tag_list 对应 `i18n_text_tag_list`；可在 `config.locales` 数组里声明白名单语种。客户端按用户语言自动选取。
