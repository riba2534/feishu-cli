# 卡片消息发送与历史排障

飞书卡片消息（interactive）是最灵活的消息类型，支持丰富的布局和交互组件。本文档只维护 `msg send` 侧的发送格式、`template_id` / `card_id` 用法和历史排障。新增卡片 JSON 的设计与模板请使用 `../feishu-cli-card/SKILL.md`。

## 三种发送方式

| 方式 | 适用场景 | 灵活性 |
|------|---------|--------|
| 完整 Card JSON | 代码动态生成卡片 | 最高 |
| template_id | 使用飞书卡片搭建工具创建的模板 | 中等（模板变量） |
| card_id | 引用已保存的卡片实例 | 最低（固定内容） |

**推荐**：Agent 发送现成卡片时使用完整 Card JSON，最灵活且无需预先创建模板。卡片 JSON 本身由 `feishu-cli-card` 生成。

---

## Card JSON 结构

### v1 格式（历史兼容，新增卡片不要使用）

```json
{
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "卡片标题"}
  },
  "elements": [
    {"tag": "markdown", "content": "内容"},
    {"tag": "hr"},
    {"tag": "note", "elements": [{"tag": "plain_text", "content": "备注"}]}
  ]
}
```

### v2 格式（支持高级组件）

```json
{
  "schema": "2.0",
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "卡片标题"}
  },
  "body": {
    "direction": "vertical",
    "elements": [
      {"tag": "markdown", "content": "内容"}
    ]
  }
}
```

**v1 vs v2 区别**：

| 特性 | v1 | v2 |
|------|----|----|
| 顶层容器 | `elements` 数组 | `body.elements` |
| 表格组件 | 不支持 | `table` |
| 图表组件 | 不支持 | `chart` |
| 表单容器 | 不支持 | `form_container` |
| 多列布局 | `column_set`（有限） | `column_set`（完整） |

**选择建议**：新增卡片默认使用 v2（`schema: "2.0"`）。v1 只用于历史卡片排障或迁移说明，不再作为新卡片模板。

---

## header 配置

### 颜色模板

| template 值 | 色系 | 推荐场景 |
|-------------|------|---------|
| `blue` | 蓝色 | 通用通知、信息提示 |
| `wathet` | 浅蓝 | 轻量提示、次要通知 |
| `turquoise` | 青色 | 进行中、处理中状态 |
| `green` | 绿色 | 成功、完成、通过 |
| `yellow` | 黄色 | 注意、提醒 |
| `orange` | 橙色 | 警告、需关注 |
| `red` | 红色 | 错误、失败、紧急 |
| `carmine` | 深红 | 严重告警、危险 |
| `violet` | 紫罗兰 | 特殊标记 |
| `purple` | 紫色 | 自定义分类 |
| `indigo` | 靛蓝 | 深色主题 |
| `grey` | 灰色 | 已处理、归档、历史 |

### 语义化颜色速查

```
成功/完成 → green
通用通知 → blue
警告/注意 → orange
错误/紧急 → red
进行中   → turquoise
已处理   → grey
```

### header 完整结构

```json
{
  "header": {
    "template": "blue",
    "title": {
      "tag": "plain_text",
      "content": "卡片标题"
    },
    "subtitle": {
      "tag": "plain_text",
      "content": "副标题（可选）"
    }
  }
}
```

---

## 组件速查

本节是历史排障参考，保留 v1/v2 混合组件用于读懂旧卡片。新增卡片以 `feishu-cli-card/references/components.md` 为准，不要使用 v1 的 `note` / `action`。

### 内容组件

#### markdown（最常用）

```json
{
  "tag": "markdown",
  "content": "**加粗** *斜体* ~~删除线~~ `代码`\n[链接](url)\n<font color='green'>绿色</font>"
}
```

#### hr（分割线）

```json
{"tag": "hr"}
```

#### note（v1 底部备注，新增不要使用）

灰色小字，通常放在卡片底部。v2 新卡片用 `markdown` + `<font color='grey'>...` + `text_size: "notation"` 替代。

```json
{
  "tag": "note",
  "elements": [
    {"tag": "plain_text", "content": "备注内容"}
  ]
}
```

note 的 elements 支持 `plain_text`、`lark_md`、`img` 类型。

#### img（图片）

```json
{
  "tag": "img",
  "img_key": "img_v2_xxx",
  "alt": {"tag": "plain_text", "content": "图片描述"},
  "mode": "fit_horizontal"
}
```

mode 可选值：`crop_center`（居中裁剪）、`fit_horizontal`（适应宽度）、`large`（大图）、`medium`（中图）、`small`（小图）、`tiny`（超小图）。

### 布局组件

#### div（文本块 + fields 多列）

基础文本块：

```json
{
  "tag": "div",
  "text": {"tag": "lark_md", "content": "一段文本"}
}
```

带 fields（多列布局）：

```json
{
  "tag": "div",
  "fields": [
    {"is_short": true, "text": {"tag": "lark_md", "content": "**标签1**\n值1"}},
    {"is_short": true, "text": {"tag": "lark_md", "content": "**标签2**\n值2"}},
    {"is_short": true, "text": {"tag": "lark_md", "content": "**标签3**\n值3"}},
    {"is_short": true, "text": {"tag": "lark_md", "content": "**标签4**\n值4"}}
  ]
}
```

- `is_short: true`：半宽排列（一行放两个）
- `is_short: false`：全宽排列（独占一行）

带 extra（右侧附加元素）：

```json
{
  "tag": "div",
  "text": {"tag": "lark_md", "content": "左侧文本"},
  "extra": {
    "tag": "button",
    "text": {"tag": "plain_text", "content": "操作"},
    "type": "primary",
    "url": "https://example.com"
  }
}
```

#### column_set（多列分栏，v2）

```json
{
  "tag": "column_set",
  "flex_mode": "none",
  "background_style": "default",
  "columns": [
    {
      "tag": "column",
      "width": "weighted",
      "weight": 1,
      "elements": [
        {"tag": "markdown", "content": "**左栏内容**\n详细描述..."}
      ]
    },
    {
      "tag": "column",
      "width": "weighted",
      "weight": 1,
      "elements": [
        {"tag": "markdown", "content": "**右栏内容**\n详细描述..."}
      ]
    }
  ]
}
```

`flex_mode` 可选值：`none`（不换行）、`flow`（自动换行）、`bisect`（二等分）、`trisect`（三等分）。

### 交互组件

#### action（v1 按钮容器，新增不要使用）

```json
{
  "tag": "action",
  "actions": [
    {
      "tag": "button",
      "text": {"tag": "plain_text", "content": "主要按钮"},
      "type": "primary",
      "url": "https://example.com"
    },
    {
      "tag": "button",
      "text": {"tag": "plain_text", "content": "危险按钮"},
      "type": "danger"
    },
    {
      "tag": "button",
      "text": {"tag": "plain_text", "content": "默认按钮"},
      "type": "default"
    }
  ]
}
```

v2 新卡片中按钮直接放入 `body.elements` 或 `column_set` 内，通过 spacing / columns 控制布局。

按钮类型：
- `primary`：蓝色主按钮
- `danger`：红色危险按钮
- `default`：灰色普通按钮

**注意**：`url` 属性实现跳转链接，无需服务端支持；`value` 属性触发回调，需要应用服务端处理。CLI 场景下推荐使用 `url`。

#### select_static（下拉选择）

```json
{
  "tag": "select_static",
  "placeholder": {"tag": "plain_text", "content": "请选择"},
  "options": [
    {"text": {"tag": "plain_text", "content": "选项 1"}, "value": "opt1"},
    {"text": {"tag": "plain_text", "content": "选项 2"}, "value": "opt2"}
  ]
}
```

#### date_picker（日期选择）

```json
{
  "tag": "date_picker",
  "placeholder": {"tag": "plain_text", "content": "请选择日期"}
}
```

#### overflow（折叠菜单）

```json
{
  "tag": "overflow",
  "options": [
    {"text": {"tag": "plain_text", "content": "操作 1"}, "value": "action1"},
    {"text": {"tag": "plain_text", "content": "操作 2"}, "value": "action2"}
  ]
}
```

### v2 独有组件

#### table（表格，v2）

```json
{
  "tag": "table",
  "page_size": 5,
  "row_height": "low",
  "header_style": {"text_align": "center", "text_size": "normal", "background_style": "grey", "text_color": "default", "bold": true},
  "columns": [
    {"name": "name", "display_name": "姓名", "width": "auto", "data_type": "text"},
    {"name": "status", "display_name": "状态", "width": "auto", "data_type": "text"}
  ],
  "rows": [
    {"name": "张三", "status": "已完成"},
    {"name": "李四", "status": "进行中"}
  ]
}
```

#### chart（图表，v2）

```json
{
  "tag": "chart",
  "chart_spec": {
    "type": "bar",
    "title": {"text": "数据统计"},
    "data": {
      "values": [
        {"category": "A", "value": 10},
        {"category": "B", "value": 20}
      ]
    }
  }
}
```

---

## 卡片 Markdown 语法（lark_md）

卡片内的 `markdown` 组件使用 `lark_md` 语法，与标准 Markdown 有差异。

### 支持的语法

| 语法 | 效果 |
|------|------|
| `**文本**` | 加粗 |
| `*文本*` | 斜体 |
| `~~文本~~` | 删除线 |
| `` `代码` `` | 行内代码 |
| `[文本](url)` | 超链接 |
| `![描述](img_key)` | 图片（需要 img_key） |

### 特有语法

#### 彩色文字

```markdown
<font color='green'>成功</font>
<font color='red'>失败</font>
<font color='grey'>已处理</font>
```

仅支持 `green`、`red`、`grey` 三种颜色。

#### @用户

```markdown
<at id=ou_xxx></at>
<at id=all></at>
```

**注意**：卡片中 @用户的语法是 `<at id=xxx>`（无引号），而 text/post 消息中是 `<at user_id="xxx">`（有引号），两者不同。

### 不支持的语法

- 标题（`#`、`##` 等）
- 有序/无序列表（`-`、`1.`）→ 使用 `\n- ` 或 `\n1. ` 模拟
- 代码块（` ``` `）→ 卡片中显示效果有限
- 表格 → 使用 v2 的 `table` 组件代替

---

## 新卡片构造入口

新增卡片不要从本文复制模板。先用 `feishu-cli-card` 参考 `references/design.md` 与 `references/components.md` 生成 v2 JSON，再回到本命令发送：

```bash
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/my-card.json
```

最小 v2 结构如下，仅用于理解 `content-file` 里应该放什么：

```json
{
  "schema": "2.0",
  "config": {"update_multi": true, "width_mode": "fill"},
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "快速通知"}
  },
  "body": {
    "direction": "vertical",
    "elements": [
      {"tag": "markdown", "content": "任务已完成。"}
    ]
  }
}
```

## CLI 发送示例

### 从文件发送卡片

```bash
# 将 v2 卡片 JSON 写入文件
cat > /tmp/card.json << 'CARD_EOF'
{
  "schema": "2.0",
  "config": {"update_multi": true, "width_mode": "fill"},
  "header": {
    "template": "green",
    "title": {"tag": "plain_text", "content": "任务完成"}
  },
  "body": {
    "direction": "vertical",
    "elements": [
      {"tag": "markdown", "content": "所有子任务已完成，可以发布。"},
      {"tag": "markdown", "content": "<font color='grey'>自动通知</font>"}
    ]
  }
}
CARD_EOF

# 发送
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/card.json
```

### 使用 template_id

```bash
cat > /tmp/tpl.json << 'EOF'
{
  "type": "template",
  "data": {
    "template_id": "AAqk1xxxxxx",
    "template_variable": {
      "title": "部署通知",
      "env": "production",
      "version": "v1.2.3"
    }
  }
}
EOF

feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/tpl.json
```

### 内联 JSON 发送简单卡片

```bash
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content '{"schema":"2.0","config":{"update_multi":true},"header":{"template":"blue","title":{"tag":"plain_text","content":"快速通知"}},"body":{"direction":"vertical","elements":[{"tag":"markdown","content":"任务已完成"}]}}'
```

---

## 注意事项

1. **大小限制**：卡片 JSON 最大 30 KB，超出时精简内容或拆分多条消息
2. **按钮回调**：`url` 属性可直接跳转（无需服务端）；`value` 属性需要应用服务端处理回调事件
3. **图片引用**：卡片中的 `img_key` 需要通过飞书 API 上传获取，不能直接使用外部 URL
4. **Markdown 差异**：卡片 Markdown（lark_md）不支持标题、列表等常见语法，仅支持加粗/斜体/删除线/链接/代码/颜色/@人
5. **v1 vs v2**：新增卡片优先用 v2；v1 仅用于历史兼容排查
6. **颜色语义**：header 颜色应与消息语义匹配（绿=成功、红=错误、橙=警告、蓝=通知）
