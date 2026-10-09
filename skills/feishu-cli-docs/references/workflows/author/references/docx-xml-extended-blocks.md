<!-- 内容改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.） -->
# DocxXML 扩展块补充说明

常用标签与通用规则见 [`docx-xml.md`](docx-xml.md)。标注"实测"的写法已写入测试文档并回读确认；
其余依赖真实业务资源（任务、群、OKR、知识库），未在本项目实测，写入后用 `doc read --engine docs_ai --detail full` 回读确认。

## 扩展标签

- `<bookmark name="示例站点" href="https://example.com"></bookmark>`：网页书签卡片（实测）。
- `<button action="OpenLink" src="https://example.com" background-color="blue">操作按钮</button>`：行内按钮（实测 `OpenLink`）。
  `action` 还可为 `DuplicatePage`、`FollowPage`；可选 `background-color`、`src`。
- `<time expire-time="1775916000000" notify-time="1775912400000" should-notify="false">提醒</time>`：日期提醒，使用毫秒时间戳（实测；
  标签内的文字回读时作为普通文本跟在提醒之后，提醒本身显示日期）。
- `<sheet type="blank"/>`：在文档中新建空白电子表格（实测）；`<sheet sheet-id="SHEET_ID" token="SPREADSHEET_TOKEN"/>` 复制已有表格（未实测）。
  需要写入单元格时按 `feishu-cli-data` 的 sheet 工作流操作。实测 `type="blank"` 会生成一个独立的电子表格（回读 `<sheet token>`），
  删除文档后它仍然存在，清理测试文档时需另行 `feishu-cli file delete <token> --type sheet`。
- `<task task-id="TASK_GUID"/>`：挂载任务，`task-id` 为任务 GUID（未实测）。
- `<chat_card chat-id="CHAT_ID"/>`：挂载群聊卡片（未实测）。
- `<sub-page-list/>`：子页面列表块，仅知识库文档可插入（未实测）。

## HTML 组件（html5-block）

把完整的单文件 HTML 存为本地 `.html` 文件（建议放在草稿工作区），XML 中写 `<html5-block path="@./widget.html"/>`；
`doc create --doc-format xml` / `doc content-update --doc-format xml` 会读取文件并转为 `reference_map` 写入。
标签体必须为空，HTML 不能直接写在 `<html5-block>` 与 `</html5-block>` 之间；`data` 属性保留给 CLI 内部使用。
读取时 `<html5-block data-ref="html5_1"></html5-block>` 只是占位，HTML 内容在响应的 `reference_map` 中。

妙笔 BOX（`feishu-cli doc htmlbox create <document_id> --html-file ./widget.html`，规范见 `feishu-cli-visual` 的 htmlbox 工作流）
是另一种块类型：实测回读为 `<readonly-block type="isv">`，不是 `html5-block`，适合给已有文档单独追加动态组件。
`html5-block` 的 HTML 文件格式：

```html
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="use-iframe" content="true">
  <meta name="html-box-height-mode" content="auto">
  <meta name="description" content="内容摘要，帮助理解该组件的用途">
  <title></title>
</head>
<body>
  ...
</body>
</html>
```

- `html-box-height-mode` 必须在 `<head>` 中显式声明（CLI 原样写入、不做校验）。高度模式只用 `auto`（正文在文档中完整展开，普通文档流，根容器不设固定高度或 `overflow: hidden`）或
  `viewport`（`100vh` + 内部滚动 / 切页 / 缩放，适合游戏、幻灯片、Dashboard、canvas）。
- 文档常见可用宽度约 820 px；根容器用 `width: 100%`、`max-width: 100%`、`box-sizing: border-box`。
- 页面加载后再追加或展开的内容不会触发高度刷新，不要臆造相关 flag。
- HTML 总长度控制在 500KB 内；不要内联大图片、Base64、字体、长 JSON/CSV 或大量 mock 数据。

## OKR 块

`<okr cycle-id="CYCLE_ID"></okr>`：创建时只支持 root-only，挂载已有周期的 OKR（未实测）。先按 `feishu-cli-work` 的 OKR 工作流
确认可用周期；不要构造 Objective / KR / Progress 子树。读取时的结构示例：

```xml
<okr cycle-id="" cycle-name="CYCLE_NAME" user-name="USER_NAME">
  <okr-objective objective-id="OBJECTIVE_ID" status="normal" percent="80" score="75">
    <p>O 描述</p>
    <okr-progress>
      <p>O 进展</p>
      <checkbox done="true">事项</checkbox>
    </okr-progress>
    <okr-key-result key-result-id="KEY_RESULT_ID" status="risk" percent="60" score="80">
      <p>KR 描述</p>
      <okr-progress><p>KR 进展</p></okr-progress>
    </okr-key-result>
  </okr-objective>
</okr>
```

- `cycle-name`、`user-name` 只读；`objective-id`、`key-result-id` 为只读业务 ID，更新已有 OKR 时保持不变。
- `okr-objective` / `okr-key-result` 可更新 `status`（`unset` / `normal` / `risk` / `extended`）、`percent`、`score`（0-100），不可更新描述。
- `okr-progress` 承载进展，直接子节点支持 `p`、`checkbox`、`grid`、`img`、`source`、`ol`、`ul`、`h1`-`h9`。
