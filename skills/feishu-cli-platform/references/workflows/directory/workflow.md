# 通讯录工作流

用于只读查询用户、机器人和部门。写通讯录不在当前 CLI 封装范围内；未封装端点先查 schema，再用 api。

## 常用命令

```bash
feishu-cli user info ou_xxx                                        # 默认 App 身份，字段最全
feishu-cli user info ou_xxx --as user                              # 应用通讯录权限不足时，以用户身份只查姓名
feishu-cli user search --email user@example.com
feishu-cli user search --mobile '+8613800000000'
feishu-cli user search --query "张三" --has-chatted -o json        # 关键词搜索（必须 User Token）
feishu-cli user search-bot --query "告警" --chat-ids oc_xxx        # 搜索机器人（必须 User Token，scope search:bot）
feishu-cli user list --department-id od_xxx
feishu-cli dept get od_xxx
feishu-cli dept children 0
```

## 身份与字段

| 命令 | 身份 | 说明 |
|---|---|---|
| `user info` | `--as bot`（默认）/ `user` / `auto` | bot 走 `contact/v3/users/:id`，字段最全，需应用通讯录权限；user 走 `basic_batch`，只返回 `open_id`、`name`、`en_name`，适合应用通讯录范围外的同事 |
| `user search --email/--mobile` | App 身份 | 走 `contact/v3/users/batch_get_id`，按 `open_id`、`user_id`、`union_id` 分别查询并填入同名字段（`user_id` 需应用有 `contact:user.employee_id:readonly`，否则为空）；已登录时再用 `basic_batch` 补全姓名 |
| `user search --query` 及过滤条件 | 必须 User | 走 `contact/v3/users/search`，见下文 |
| `user search-bot` | 必须 User（`search:bot`） | 走 `bot/v4/bot/search` |
| `user list`、`dept get/children` | App 身份 | 需应用通讯录范围覆盖目标部门 |

`user search --email/--mobile` 旧版本输出的 `user_id` 实为 open_id，现已改为真实 user_id；依赖旧值的脚本应改读 `open_id`。

**关键词搜索**（`user search --query`）：关键词 ≤ 50 字，可叠加 `--has-chatted`、`--exclude-external-users`、
`--left-organization`、`--has-enterprise-email`、`--user-ids`（≤ 100）、`--lang` 过滤；过滤条件也可不带 `--query` 单独使用。
每页 1–30（`--page-size` 默认 20）。结果 `users[]` 含 `open_id`、`name`、`email`、`enterprise_email`、`department`、
单聊 `p2p_chat_id`、`has_chatted`、`is_cross_tenant` 等，**不返回 `user_id`**；需要 user_id 时用 `--email/--mobile`。

**机器人搜索**（`user search-bot`）：`--query` 必填（≤ 50 字），`--chat-ids`（≤ 100）只在指定群里找，
`--has-chatted` 只看聊过天的机器人，每页 1–30。结果 `bots[]` 的 `open_id` 可直接用于 @机器人 或 `chat member add`。

两种搜索在服务端返回 `notice`（如查询词被截断）时都会写到 stderr，JSON 输出同时带 `notice` 字段。

## 注意事项

- 用户 ID 类型包括 `open_id`、`union_id`、`user_id`；部门 ID 默认是 `open_department_id`。
  根据输入实际类型设置 `--user-id-type` 或 `--department-id-type`，不要靠前缀猜测 ID 类型。
- 应用通讯录权限不足时 `dept`/`user list` 返回业务错误（如 40004 no dept authority，退出码 1），
  需在开放平台调整应用的通讯录可见范围；只需姓名时可改用 `user info --as user`。
- 结果默认只读取；用户要求后续发消息、拉群等写操作时切换到对应领域技能。
- 示例只能使用 `user@example.com`、`ou_xxx` 等占位信息。
