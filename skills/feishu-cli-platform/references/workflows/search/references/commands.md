# 搜索命令参数映射与输出格式

本文件补充 `../workflow.md`：列出过滤参数到接口字段的映射与 JSON 输出结构。完整 flag 以
`feishu-cli search <docs|messages|apps> --help` 为准。

## search messages：参数映射

请求走 `POST /open-apis/im/v1/messages/search`，query 可省略。

| 参数 | 接口字段 / 说明 |
|------|------|
| `--chat-ids` | `filter.chat_ids`（逗号分隔） |
| `--from-ids` | `filter.from_ids`（逗号分隔） |
| `--at-chatter-ids` | `filter.at_chatter_ids`（逗号分隔） |
| `--chat-type` | `group_chat` / `p2p_chat`（也接受 `group` / `p2p`），映射为 `group` / `p2p` |
| `--from-type` / `--exclude-from-type` | `bot` / `user` |
| `--is-at-me` | 仅搜索 @我 的消息 |
| `--message-type` | `include_attachment_types`：`file` / `image` / `video` / `link`；`media` 映射为 `video` |
| `--start-time` / `--end-time` | `filter.time_range`；接受 RFC3339、`YYYY-MM-DD` 或 Unix 秒 |
| `--page-size` | 1–50，默认 20，越界报错 |
| `--page-all` / `--page-limit` | 最多 40 页；`--page-limit` 1–40，`0` 在 `--page-all` 时等于 40 |
| `--enrich` | 额外用 `GET /open-apis/im/v1/messages/mget`（每批最多 50）补全内容/发送者/群名/时间 |
| `--card-content-type` | 仅 `--enrich` 时生效：`user`（默认，提取 `card_texts`）/ `raw`（完整 cardDSL）/ `rendered`（OAPI 渲染版） |

### 输出

默认（无 `--enrich`），`-o json` 或 `--format json`：

```json
{
  "MessageIDs": ["om_xxx", "om_yyy"],
  "PageToken": "ea9dcb2f...",
  "HasMore": true
}
```

服务端返回提示时多一个 `notice` 字段，同时写 stderr。

`--enrich` 时返回对象数组：

```json
[
  {
    "message_id": "om_xxx",
    "msg_type": "text",
    "chat_id": "oc_xxx",
    "chat_name": "项目群",
    "sender_id": "ou_xxx",
    "sender_name": "张三",
    "create_time": "1704067200000",
    "time": "2024-01-01 08:00:00",
    "text": "今天上线"
  }
]
```

## search docs：参数与输出

底层与官方 CLI 一致使用 Search v2（`POST /open-apis/search/v2/doc_wiki/search`，与 `drive search` 同一端点），只返回当前用户
有权访问的文档，默认同时搜云盘与知识库。

| 参数 | 说明 |
|------|------|
| `--count` | 每页数量 1–20，默认 20（超过 20 按 20 处理并在 stderr 提示） |
| `--page-token` | 上一页输出的 `PageToken` |
| `--offset` | 已废弃：传大于 0 的值报用法错误，改用 `--page-token` |
| `--owner-ids` | 文件所有者 open_id（逗号分隔，映射 v2 `creator_ids`） |
| `--chat-ids` | 文件所在群 ID（逗号分隔） |
| `--docs-types` | 小写类型（逗号分隔）：`doc` 旧版文档、`docx` 新版文档、`sheet`、`slides`、`bitable`、`mindnote`、`file`、`wiki`、`shortcut`、`folder`、`catalog` |

`-o json` 输出：

```json
{
  "Total": 35,
  "HasMore": true,
  "PageToken": "<下一页游标>",
  "ResUnits": [
    {
      "DocsToken": "doc_token_xxx",
      "DocsType": "docx",
      "Title": "产品需求文档 - Q2",
      "OwnerID": "ou_xxx",
      "URL": "https://www.feishu.cn/docx/doc_token_xxx"
    }
  ]
}
```

## search apps：参数

| 参数 | 说明 |
|------|------|
| `--page-size` | 每页数量，默认 20 |
| `--page-token` | 分页 token |
| `--user-id-type` | `open_id`（默认）/ `union_id` / `user_id` |
