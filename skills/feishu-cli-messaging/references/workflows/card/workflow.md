# 飞书 Card JSON 2.0 构造工作流

把用户意图转换为真实、可读、可校验的 Card JSON 2.0。默认产物是卡片 JSON；只有用户明确
要求发送、并确认接收者后，才衔接 `msg send`。本工作流不负责实现按钮回调服务。

所有相对路径均相对本文件所在目录解析。

## 不可跳过的原则

1. 只生成 Card JSON 2.0：顶层 `"schema": "2.0"`，组件位于 `body.elements`。
2. 不编造数字、人员、ID、URL、图片 token、数据源或行动能力。
3. 场景模板和视觉风格分别选择；模板是骨架，不是必须填满的表单。
4. 草稿可以有显式 TODO，但发送候选不允许占位符、示例 ID 或 `example.com`。
5. 没有真实动作就不放按钮；没有回调处理器就不生成 callback 交互。
6. 发送前必须先跑项目自带离线 linter，再按风险预览 PC 与移动端。

内容事实与审查规则见 `references/content-quality.md`，视觉风格路由见
`references/styles.md`。

## 1. 先选场景骨架

| 用户意图 | 起始模板 | 默认 header.template |
|---|---|---|
| 通知、公告、信息同步 | `templates/notification.json` | `blue` |
| 操作完成、构建成功、交付 | `templates/success-report.json` | `green` |
| 告警、故障、错误 | `templates/alert.json` | `red` |
| 审批请求、待办、确认 | `templates/approval.json` | `orange` |
| 报表、指标、复盘 | `templates/data-dashboard.json` | `purple` |
| 长文或文章摘要 | `templates/article-summary.json` | `blue` |
| AI 流式输出 | `templates/llm-streaming.json` | `blue` |

场景不匹配时使用最小骨架从零构造，不要硬套 `notification.json`。用户只给了一段短通知时，
`header + 一段 markdown` 往往已经足够。

然后独立选择视觉风格：

- 未明确指定：`default-semantic`；
- 明确要求 dashboard：从 6 个 `dashboard-*` 子风格中选择；
- 明确要求现代、极简、克制：`modern-minimal`；
- 提供品牌、参考图、专题方向或点名主题：从 11 个 Custom Visual `style_id` 中选择。

可运行 `python3 scripts/style_assets.py styles` 查看全部 19 个预设；素材库包含 5 张语义头图、
153 个 doodle 和 48 个 phosphor 图标。

## 2. 最小骨架

```json
{
  "schema": "2.0",
  "header": {
    "template": "blue",
    "title": {
      "tag": "plain_text",
      "content": "标题"
    }
  },
  "body": {
    "direction": "vertical",
    "vertical_spacing": "medium",
    "elements": [
      {
        "tag": "markdown",
        "content": "正文"
      }
    ]
  }
}
```

`config`、`subtitle`、图表、表格和按钮都按需添加。V2 的 `update_multi` 只能为 `true`；
不需要改变默认行为时直接省略。`width_mode` 可用 `default`、`compact`、`fill`。

## 3. 八步工作流

### 步骤一：建立事实账本

区分用户事实、已授权数据源事实、安全推断和缺失信息。先列 TODO，再开始写 JSON。任何会改变
结论或行动的缺失信息都不能静默猜测。

### 步骤二：选模板和风格

读取一个最接近的 `templates/*.json` 作为结构参考；再按 `references/styles.md` 调整信息层级。
删除没有事实支撑的模块，不为保留模板结构而制造内容。

用户点名视觉风格时，先查看预设并按需生成起稿：

```bash
python3 scripts/style_assets.py show handdrawn-dark
python3 scripts/style_assets.py starter handdrawn-dark \
  --title "真实标题" \
  --summary "真实结论" \
  --detail "真实补充" \
  --output /tmp/card.json
```

Dashboard、Modern Minimal 和 Custom Visual 的完整选择边界、组件配方及素材命令统一见
`references/styles.md`，再按其中路由渐进读取对应子文件。

### 步骤三：组织信息

推荐顺序：

```text
header → 首屏结论 → 关键证据 → 必要详情 → 真实行动 → 来源/时间
```

把次要细节移入折叠面板。每张卡片只保留一个主要视觉焦点和至多一个 primary 行动。
折叠面板的标题条属于导航 surface，不是第二个 header：背景保持 `white`、`default` 或
`*-50/*-100` 浅色，语义色放在标题文字、图标或细边框。不要使用 `blue`、`purple`、
`red` 等饱和色铺满整条标题栏。

### 步骤四：构造组件

常用选择：

| 目标 | 组件 | 关键约束 |
|---|---|---|
| 富文本结论 | `markdown` | `text_align` 可省略，默认左对齐 |
| 2×2 关键字段 | `div.fields` | 避免超过 6 项 |
| 分栏 | `column_set` | 移动端阅读顺序必须成立 |
| 图表 | `chart` | 字段必须存在于 `chart_spec.data.values` |
| 表格 | `table` | 只能直接放在 `body.elements` |
| 单人 / 人员列表 | `person` / `person_list` | 前者用 `user_id`；后者每项用 `id` |
| 单行 / 多行输入 | `input` | 多行仍是 `input`，设 `input_type: "multiline_text"` |
| 表单 | `form` | 只能直接放 body，且必须有 submit 按钮 |
| 跳转 / 回调 | `button` | 目标或回调处理器必须真实存在 |

完整字段见 `references/components.md`；VChart 见 `references/vchart-quickref.md`；迁移历史 V1
卡片时再读 `references/v2-vs-v1.md`。

### 步骤五：处理图片

- `img` 必须提供有意义的 `alt`；`img_key` 可使用当前 App 已上传的 key，或配合
  `--upload-images` 使用存在的本地图片路径。
- Markdown 中需要发送的本地图片可写本地路径，发送时加 `--upload-images`。
- `img_combination.img_list[].img_key` 同样支持本地路径，数量仍需严格匹配布局模式。
- 不得复制模板、其他租户或外部 Skill 中的 `img_key`。
- URL 图片不能直接充当 `img_key`。
- `media upload` 返回的是文档素材 `file_token`，不可混用。
- 内置素材从 `assets/` 选择；运行 `python3 scripts/style_assets.py assets ...` 检索，
  `copy <asset_id> --output ...` 可原样复制到业务目录。

### 步骤六：离线校验

先将 JSON 写到临时文件，例如 `/tmp/alert-card.json`。
以下命令以仓库根目录为工作目录；Skill 安装到其它位置时，用本文件已解析出的实际目录替换
`skills/feishu-cli-messaging/references/workflows/card` 前缀。

草稿允许明确占位符：

```bash
python3 skills/feishu-cli-messaging/references/workflows/card/scripts/lint_card.py \
  --allow-placeholders /tmp/alert-card.json
```

准备发送时必须使用严格的发送候选检查：

```bash
python3 skills/feishu-cli-messaging/references/workflows/card/scripts/lint_card.py \
  --strict /tmp/alert-card.json
```

需要机器可读报告：

```bash
python3 skills/feishu-cli-messaging/references/workflows/card/scripts/lint_card.py \
  --strict --json /tmp/alert-card.json
```

发送候选包含任意本地图片时（Markdown、`img.img_key` 或 `img_combination`）：

```bash
python3 skills/feishu-cli-messaging/references/workflows/card/scripts/lint_card.py \
  --strict --upload-images /tmp/alert-card.json
```

该脚本是项目的保守离线检查器，不冒充飞书服务端完整 Schema。它会检查 JSON 2.0 根结构、
30 KB / 200 组件等常见限制、V1 残留、非法嵌套、重复 `element_id`、占位符、人员字段、
表单提交、图表字段、无动作按钮、深色/饱和折叠标题条和本地图片上传提示；对 `audio`、
`video`、`avatar`、静态/人员/图片选择器及日期时间选择器额外检查高置信度必填字段、枚举、
默认值引用与格式。它还会确认声明的本地素材真实存在。
警告也必须人工处理；只有确知可接受时才保留。

### 步骤七：内容与视觉验收

按 `references/content-quality.md` 的双清单检查：

- 首屏能否回答“发生了什么、是否需要我处理”；
- 每个数字、ID、链接和人名是否有来源；
- 按钮是否真的能兑现；
- 去掉颜色后层级是否仍清楚；
- 折叠标题条是否为白色/浅色，并通过文字或图标颜色表达语义；
- 窄屏折行后阅读顺序是否合理；
- 图表、表格或图片是否确实提升理解。

复杂卡片在测试会话或官方卡片搭建工具中预览 PC、移动端、浅色和深色模式。离线 linter
无法证明客户端渲染效果或回调链路可用。

### 步骤八：按用户授权发送

发送前确认 `receive-id-type` 与 ID 类型匹配，并重新运行 `--strict` 且不带
`--allow-placeholders` 的检查。

```bash
feishu-cli msg send \
  --receive-id-type chat_id \
  --receive-id oc_xxx \
  --msg-type interactive \
  --content-file /tmp/alert-card.json
```

卡片中含本地图片路径时追加：

```bash
feishu-cli msg send \
  --receive-id-type chat_id \
  --receive-id oc_xxx \
  --msg-type interactive \
  --content-file /tmp/alert-card.json \
  --upload-images
```

图片逐张上传；任一文件缺失、格式非法或上传失败都会中止，不会把本地路径继续发送给飞书。
修复素材后使用同一幂等键重试。
成功标志以 CLI 返回的消息 ID 为准。

## 4. V2 禁区

| 禁止 | 替代 |
|---|---|
| 顶层 `elements` | `body.elements` |
| `tag: "note"` | notation 字号的灰色 `markdown` |
| `tag: "action"` | 直接使用 `button`；并排时放入 `column_set` |
| `tag: "textarea"` | `input_type: "multiline_text"` 的 `input` |
| `config.wide_screen_mode` | `config.width_mode: "fill"` |
| `config.update_multi: false` | 删除或设为 `true` |
| table 嵌在任何容器中 | table 直接放 `body.elements` |
| form 嵌在其它容器中 | form 直接放 `body.elements` |
| form 内再放 form / table / chart | 重组为根级组件 |
| `person.user_id_type` | 删除；只传 `person.user_id` |
| `person_list.persons[].user_id` | 改为 `persons[].id` |
| hex 颜色 | 官方颜色枚举或组件允许的 `rgba(...)` |
| 图片 URL 写入 `img_key` | 上传后使用真实 `img_key` |

## 5. 资源索引

| 文件 | 何时读取 |
|---|---|
| `references/content-quality.md` | 事实、TODO、来源、行动真实性与发送前审查 |
| `references/styles.md` | 19 个预设的总路由与素材操作 |
| `references/styles/dashboard.md` | 6 个 Dashboard 子风格 |
| `references/styles/modern-minimal/` | token、原子组件、蓝图与 5 张头图 |
| `references/styles/custom-visual/` | 11 个命名主题与 4 个结构原型 |
| `references/components.md` | 组件字段、嵌套与颜色枚举 |
| `references/design.md` | 配色、间距、图标和详细布局 |
| `references/vchart-quickref.md` | 构造图表 |
| `references/v2-vs-v1.md` | 迁移 V1 历史卡片 |
| `templates/*.json` | 七种场景骨架 |
| `scripts/lint_card.py` | 离线发送闸门 |
| `scripts/style_assets.py` | 列出预设、检索/复制素材、生成 token 和起稿 |
| `assets/` | 5 张头图、201 个图标与主题 manifest |

规范发生冲突时，以当前飞书官方文档和服务端响应为准，随后修正本项目文档与 linter：

- Card JSON 2.0 结构：`https://open.feishu.cn/document/feishu-cards/card-json-v2-structure`
- V2 组件总览：`https://open.feishu.cn/document/feishu-cards/card-json-v2-components`
- 飞书卡片搭建工具说明：`https://open.feishu.cn/document/uAjLw4CM/ukzMukzMukzM/feishu-cards/feishu-card-cardkit/feishu-cardkit-overview`

## 与消息工作流的边界

本工作流负责构造、审查和校验卡片。用户要求发送时，再读取
`../msg/workflow.md`，由消息工作流负责接收者、身份、上传与实际发送。只要求设计或生成 JSON
时，不应产生外部消息。
