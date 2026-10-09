# 评论工作流

支持 doc/docx/sheet/bitable/file/slides 评论的列出、读取、回复、编辑回复、表情回应和解决状态。
创建富文本、局部（docx block / sheet 单元格 / slides 元素 / 多维表格记录）、云盘文件或 wiki URL 评论用
`../drive/workflow.md` 的 `drive add-comment`；`comment add` 只能建纯文本全文评论。

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
  回复前先看 list/get 输出的 `is_whole`；需要讨论串时用 `drive add-comment --block-id` 建局部评论。
- `reply update` 整体替换回复内容；修改根回复即修改评论正文。`link` 元素实测必须是飞书文档链接，普通网址返回 `1069302 param error`。
- `reply react` 的 `--emoji` 区分大小写（如 `THUMBSUP`、`HEART`、`DONE`、`OK`、`LGTM`、`ThumbsDown`），本地按平台枚举校验（服务端会把任意字符串存成无法显示的表情），非法值以退出码 2 拒绝。add/delete 均幂等，delete 只取消当前身份的回应。
