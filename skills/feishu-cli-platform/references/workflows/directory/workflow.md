# 通讯录工作流

用于只读查询用户和部门。写通讯录不在当前 CLI 封装范围内；未封装端点先查 schema，再用 api。

## 常用命令

```bash
feishu-cli user info ou_xxx
feishu-cli user search --email user@example.com
feishu-cli user search --mobile '+8613800000000'
feishu-cli user search --query "张三" --has-chatted -o json      # 关键词搜索（User Token）
feishu-cli user search-bot --query "告警" [--chat-ids oc_xxx]     # 搜索机器人（User Token，scope search:bot）
feishu-cli user info ou_xxx --as user                             # 应用通讯录权限不足时，以用户身份只查姓名
feishu-cli user list --department-id od_xxx
feishu-cli dept get od_xxx
feishu-cli dept children 0
```

用户 ID 类型包括 `open_id`、`union_id` 和 `user_id`；部门 ID 默认是 `open_department_id`。

`user search --email/--mobile` 走 `contact/v3/users/batch_get_id`（App 身份），按 `open_id`、`user_id`、
`union_id` 分别查询并填入同名字段（`user_id` 需应用有 `contact:user.employee_id:readonly`，否则为空）；
已登录时再用 `users/basic_batch` 按 open_id 精确补齐姓名。旧版本输出的 `user_id` 实为 open_id，
依赖旧值的脚本应改读 `open_id`。
`user search --query` 走 `contact/v3/users/search`（必须 User Token），关键词 ≤50 字，可叠加
`--has-chatted`、`--exclude-external-users`、`--left-organization`、`--has-enterprise-email`、`--user-ids`
过滤（过滤条件也可不带 `--query` 单独使用），每页 1–30。结果含 `open_id`、姓名、邮箱、部门、单聊
`p2p_chat_id`、是否外部联系人；**不再返回 `user_id`**，需要 user_id 时用 `--email/--mobile`。
结果过多时服务端会给出 `notice`（写到 stderr），按提示加过滤条件缩小范围。
`user info` 默认 `--as bot`（`contact/v3/users/:id`，字段最全，需应用通讯录权限）；`--as user` 走
`basic_batch`，只返回姓名类字段，适合应用通讯录范围外的同事。
根据输入实际类型设置 `--user-id-type` 或 `--department-id-type`，不要靠 token 前缀猜测邮箱或手机号。

查询前确认应用具备通讯录只读 scope。示例只能使用 `user@example.com` 等占位信息。
