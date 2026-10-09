# 评论工作流

支持 doc/docx/sheet/bitable/file/slides 评论的创建、列出、读取、回复、编辑回复、表情回应和解决状态。
创建富文本、局部（docx block / sheet 单元格 / slides 元素 / 多维表格记录）、云盘文件或 wiki URL 评论用
本文的 [`drive add-comment`](#创建富文本或局部评论drive-add-comment)；`comment add` 只能建纯文本全文评论。

```bash
# 列出评论（含正文与回复；--type 必填）；默认只取一页，has_more=true 时 stderr 提示续翻 page_token
feishu-cli comment list <file_token> --type docx
feishu-cli comment list <file_token> --type docx --solved-status false --page-all -o json   # 只看未解决，拉全量
feishu-cli comment list <file_token> --type docx --comment-scope partial                     # 只看局部（划词）评论
feishu-cli comment list <file_token> --type docx --page-token <page_token>                   # 续翻

# 读取单条 / 批量读取（batch_query，单次最多 100 个 ID；--type 默认 docx）
feishu-cli comment get <file_token> <comment_id> --type docx
feishu-cli comment batch-get <file_token> --comment-ids <id1>,<id2> --type docx -o json

feishu-cli comment add <file_token> --type docx --text "评论内容"
feishu-cli comment resolve <file_token> <comment_id> --type docx
feishu-cli comment unresolve <file_token> <comment_id> --type docx

# 回复
feishu-cli comment reply list <file_token> <comment_id> --type docx --page-all -o json
feishu-cli comment reply add <file_token> <comment_id> --type docx --text "回复内容"
feishu-cli comment reply update <file_token> <comment_id> <reply_id> --type docx --text "修改后的内容"
feishu-cli comment reply update <file_token> <comment_id> <reply_id> \
  --content '[{"type":"text","text":"请 "},{"type":"mention_user","mention_user":"ou_xxx"},{"type":"text","text":" 复核"}]'
feishu-cli comment reply react <file_token> <reply_id> --type docx --emoji THUMBSUP          # 取消: --action delete
feishu-cli comment reply delete <file_token> <comment_id> <reply_id> --type docx
```

## 创建富文本或局部评论（drive add-comment）

`drive add-comment` **必须 User Token**（无 `--as`，缺 Token 直接报错），需要 `docs:document.comment:create`、
`docs:document.comment:write_only`；wiki URL 还需 `wiki:node:read`，docx 局部评论还需 `docx:document:readonly`。
与 `comment add` 的区别：后者只建纯文本全文评论（默认 User 优先），`drive add-comment` 支持富文本、wiki 解析和局部/单元格/幻灯片/记录评论。

```bash
# 全文评论（裸 token 默认按 docx 处理）
feishu-cli drive add-comment --doc doxcnxxx --content '[{"type":"text","text":"需要修改标题"}]'

# wiki URL：自动解析到底层文档
feishu-cli drive add-comment --doc "https://xxx.feishu.cn/wiki/wikcnxxx" \
  --content '[{"type":"text","text":"收到"}]'

# 局部评论（锚定到 docx block；block_id 可用 doc blocks 获取）
feishu-cli drive add-comment --doc doxcnxxx --block-id doxcnblockxxx \
  --content '[{"type":"text","text":"这段重写"}]'

# 富文本：文本 + 提及用户 + 飞书文档链接
feishu-cli drive add-comment --doc doxcnxxx --content '[
  {"type":"text","text":"请 "},
  {"type":"mention_user","mention_user":"ou_xxx"},
  {"type":"text","text":" 查看 "},
  {"type":"link","link":"https://xxx.feishu.cn/docx/doxcnyyy"}
]'

# 电子表格单元格评论（--block-id <sheetId>!<cell>，必填）
feishu-cli drive add-comment --doc shtcnxxx --type sheet --block-id a281f9!D6 \
  --content '[{"type":"text","text":"这个数需要核对"}]'

# 多维表格记录评论（--block-id <table-id>!<record-id>!<view-id>，必填）
feishu-cli drive add-comment --doc "https://xxx.feishu.cn/base/bascnxxx" \
  --block-id tblxxx!recxxx!vewxxx --content '[{"type":"text","text":"请补充"}]'

# 幻灯片元素评论（--block-id <slide-block-type>!<xml-id>，必填）
feishu-cli drive add-comment --doc "https://xxx.feishu.cn/slides/sldxxx" \
  --block-id shape!bPq --content '[{"type":"text","text":"配色再调一下"}]'

# 云盘文件全文评论（服务端仅支持部分扩展名，如 .md/.txt/.json/.csv/.pptx/.png/.jpg/.zip）
feishu-cli drive add-comment --doc boxcnxxx --type file --content '[{"type":"text","text":"已阅"}]'
```

- 目标：裸 token 用 `--type`（docx/doc/sheet/slides/bitable/file，默认 docx）；URL 按路径识别
  （docx/doc/sheets/slides/base/file/wiki）；wiki 节点支持底层 obj_type 为 docx/doc/sheet/slides/bitable/file，mindnote 等报错。
- 锚点：docx 局部评论用 `--block-id <block_id>`；sheet/slides/bitable **必须**带对应格式的 `--block-id`（不支持 `--full`）；
  doc（旧版）与 file 只支持全文评论；`--full` 强制全文评论。
- 元素：`text`；`mention_user`（open_id 放 `mention_user` 或 `text` 字段）；`link`（URL 放 `link` 或 `text` 字段，
  建议只放飞书云文档链接）。所有 `text` 合计 ≤10000 字符（拆成多个元素不能绕过，超限返回 1069302），CLI 本地预检。
- 输出 `data.comment_id` / `data.reply_id`、`is_whole`、`resolved_by`。全文评论（`is_whole=true`）**不能再被回复**
  （`comment reply add` 返回 1069302，实测），需要后续讨论时创建局部评论。

## 输出结构

- `comment list/get/batch-get -o json` 输出评论数组（get 为单个对象）：`content` 是评论正文（根回复）的可读文本，
  `reply_list.replies[]` 原样保留服务端结构（`content.elements` 中的 `text_run` / `docs_link` / `person`，
  以及 `reply_id`、`user_id`、`create_time`、`extra`）。评论下回复过多时 `has_more=true`，用 `comment reply list` 续翻。
- `comment reply list -o json`：`content` 为可读文本（@人渲染为 `@<user_id>`，文档链接渲染为 URL），
  `elements` 原样保留服务端 `content.elements`。`comment reply add -o json` 返回同样结构（含 `reply_id`、`user_id`）。
- 文本模式渲染正文、发起人、划词原文与各条回复。

## 过滤与分页

| flag | 取值 | 说明 |
|---|---|---|
| `--solved-status` | `all`（默认）/ `false` / `true` | 未解决 / 已解决过滤（服务端 `is_solved`） |
| `--comment-scope` | `all`（默认）/ `whole` / `partial` | 全文评论 / 局部评论（服务端 `is_whole`） |
| `--page-size` | 1-100，默认 50 | 每页数量 |
| `--page-token` | 上次提示的 page_token | 手动续翻 |
| `--page-all` / `--page-limit` | 默认上限 50 页，0 = 不限 | 自动翻页；游标不前进时报错停止，达到上限时 stderr 提示 page_token |

## 身份

- `list/get/batch-get/add/resolve/unresolve/reply list`：默认优先 `auth login` 的 User Token，不可用时 stderr 告警后回退 App Token
  （以代码行为为准：`comment add --help` 里"默认以 App Token 创建"的说法已过时，实测默认以登录用户身份创建）。
- `reply add/update/react/delete`：默认 App/Bot 身份，仅显式 `--user-access-token`（或 `FEISHU_USER_ACCESS_TOKEN`）时以用户身份调用。
  **回复只能由作者身份修改/删除**（否则 1069303）：Bot 创建的回复用同一 App，用户创建的回复传该用户的 User Token。
- 个人文档（App 不是协作者）用 App 身份会得到 `1069303 forbidden`，此时登录后使用 User Token。

## 限制与排错

- 飞书不支持直接删除整条评论；`comment delete` 不执行任何删除，只打印替代方案并以退出码 1 结束——改用删除回复或 resolve。
- **全文评论（`is_whole=true`）不能回复**：`reply add` 返回 `1069302 The comment section does not allow replies`（实测），
  回复前先看 list/get 输出的 `is_whole`；需要讨论串时用 [`drive add-comment --block-id`](#创建富文本或局部评论drive-add-comment) 建局部评论。
- `reply update` 整体替换回复内容；修改根回复即修改评论正文。`link` 元素实测必须是飞书文档链接，普通网址返回 `1069302 param error`。
- `reply react` 的 `--emoji` 区分大小写（如 `THUMBSUP`、`HEART`、`DONE`、`OK`、`LGTM`、`ThumbsDown`），本地按平台枚举校验（服务端会把任意字符串存成无法显示的表情），非法值以退出码 2 拒绝。add/delete 均幂等，delete 只取消当前身份的回应。
