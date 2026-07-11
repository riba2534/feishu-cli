# v2 vs v1 破坏性差异 & 迁移指南

> 本技能**只做 v2**。如果你遇到 v1 的历史卡片 JSON，用这张对照表快速迁移。

## 快速判别：是 v1 还是 v2

```
查顶层 "schema" 字段：
  有 "schema": "2.0"  → v2
  无或 "schema": "1.0" → v1

查结构：
  elements 直接在顶层 → v1
  body.elements        → v2
```

## 客户端兼容

| 版本 | 最低客户端 | 低于时行为 |
|------|-----------|-----------|
| v1 | 7.1+（chart 需要）| 基本都支持 |
| v2 | **7.20+** | 标题可显，内容显示"请升级客户端"兜底 |

## 破坏性变更（迁 v2 必改）

### 1. 顶层结构

```diff
  {
-   "schema": "1.0",               // 可省略
+   "schema": "2.0",
    "config": { ... },
    "header": { ... },
-   "elements": [ ... ]
+   "body": {
+     "elements": [ ... ]
+   }
  }
```

### 2. update_multi 默认值

- v1 默认 `false`（独享卡片）
- v2 默认 `true`，且**只支持 true**（共享卡片）

强行写 `update_multi: false` 在 v2 会报错。

### 3. 废弃组件

| 废弃组件 | v1 写法 | v2 替代 |
|---------|---------|---------|
| `note` 备注 | `{"tag":"note","elements":[...]}` | `{"tag":"markdown","text_size":"notation","content":"<font color='grey'>...</font>"}` |
| `action` 交互模块 | `{"tag":"action","actions":[btn,btn]}` | 按钮直接放 `body.elements`（水平排列用 column_set） |
| `fallback` 全局降级 | `"fallback": { ... }` | 暂不支持自定义全局降级 |
| `i18n_elements` 全局多语言 | `"i18n_elements": {"zh_cn":[...]}` | 用 `i18n_content` 做组件级多语言 |

### 4. 属性校验严格性

- **v1**：未知属性静默忽略 → 写错不会报错，但也不生效
- **v2**：未知属性**报错** → 写错会直接拒收

新增的 bug 来源：沿用 v1 写法误传 `wide_screen_mode` / `compact_width` / v1 独有的 `note` 等字段。

### 5. 废弃属性

| v1 属性 | v2 替代 |
|--------|---------|
| `config.wide_screen_mode: true` | `config.width_mode: "fill"` |
| `config.compact_width: true` | `config.width_mode: "compact"` |
| `img.size: "stretch_without_padding"` | 改用 `margin: "0 -12px"` 负边距 |
| `markdown href.urlVal + [text]($urlVal)` 差异化跳转 | `<link icon='' url='' pc_url='' ios_url='' android_url=''>text</link>` 标签 |

### 6. 组件属性变化

**header.icon 结构重组**：
```diff
  "header": {
    "title": { ... },
-   "icon": { "img_key": "img_v2_xxx" },
-   "ud_icon": { "token": "chat_outlined", "style": { "color": "red" } }
+   "icon": {
+     "tag": "standard_icon",
+     "token": "chat_outlined",
+     "color": "orange",
+     "img_key": "img_v2_xxx"
+   }
  }
```

**交互容器默认间距**：
- v1：`vertical_spacing: 12px, horizontal_spacing: 12px`
- v2：`vertical_spacing: 4px, horizontal_spacing: 8px`（且支持自定义）

**表单容器默认间距**：
- v1：`16px / 16px`
- v2：`12px / 12px`

**折叠面板 padding 行为**：
- 有边框或背景色时：v1 `8px` → v2 `4px 8px`（标题区）
- 无边框背景时：v1 `0 / 8px 8px` → v2 `0 / 8px 0 0 0`（标题 / 内容区）

### 7. column_set 语法

```diff
  {
    "tag": "column_set",
-   "columns": [{ "tag": "column", "width": 2, /* 数字权重 */ "elements": [...] }]
+   "columns": [{ "tag": "column", "width": "weighted", "weight": 2, "elements": [...] }]
  }
```
v1 的 column.width 可以直接写数字；v2 必须用 `"width": "weighted"` + `weight: 1-5`。

### 8. spacing 枚举扩展

- v1：`small(4) / medium(8) / large(16)`（三档）
- v2：`small(4) / medium(8) / large(12) / extra_large(16)`（四档，large 值改为 12px）

**注意**：v1 代码的 `large` 是 16px，迁 v2 后 `large` 变 12px。需要 16px 的地方改成 `extra_large`。

## v2 新能力（v1 没有）

1. **流式更新** `streaming_mode` + `summary`
2. **element_id** 组件级唯一标识（用于流式接口精准定位要更新的组件）
3. **统一布局属性**：所有组件支持 `margin/padding/direction/spacing/align`
4. **完整 CommonMark**：标准 Markdown + 表格 + 引用 + 嵌套列表
5. **更多 HTML 标签**：`<person>` / `<local_datetime>` / `<number_tag>` / `<link>` / 嵌套 `<font>`
6. **自定义字号/颜色**：`config.style.text_size` / `config.style.color` 按浅色/深色主题分别定义
7. **宽度模式**：`width_mode: "fill"` 撑满聊天窗口
8. **locales 白名单**：`config.locales: ["en_us"]` 限定生效语言

## 迁移检查清单

迁移前跑一遍：

- [ ] 顶层加 `"schema": "2.0"`
- [ ] 把 `elements` 包进 `body.elements`
- [ ] 删除 `note` 组件，替换成 markdown + grey font
- [ ] 删除 `action` 模块，按钮直接放 body 或用 column_set 排列
- [ ] 删除 `wide_screen_mode` / `compact_width`，改成 `width_mode`
- [ ] 删除 `i18n_elements`
- [ ] 删除 `fallback`
- [ ] column.width 数字 → `"weighted"` + weight
- [ ] `update_multi: false` → 删掉或改 `true`
- [ ] header.icon 按新结构组织
- [ ] markdown 里 `[text]($urlVal)` → `<link>` 标签
- [ ] large spacing：如果是 16px 语义，改 `extra_large`
- [ ] 跑一次 `python -m json.tool` 确认 JSON 合法
- [ ] 发到飞书 7.20+ 客户端验证渲染
