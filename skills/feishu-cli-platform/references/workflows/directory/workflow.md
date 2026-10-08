# 通讯录工作流

用于只读查询用户和部门。写通讯录不在当前 CLI 封装范围内；未封装端点先查 schema，再用 api。

## 常用命令

```bash
feishu-cli user info ou_xxx
feishu-cli user search --email user@example.com
feishu-cli user search --mobile '+8613800000000'
feishu-cli user list --department-id od_xxx
feishu-cli dept get od_xxx
feishu-cli dept children 0
```

用户 ID 类型包括 `open_id`、`union_id` 和 `user_id`；部门 ID 默认是 `open_department_id`。

`user search --email/--mobile` 走 `contact/v3/users/batch_get_id`（App 身份），按 `open_id`、`user_id`、
`union_id` 分别查询并填入同名字段（`user_id` 需应用有 `contact:user.employee_id:readonly`，否则为空）；
已登录时再用 `users/basic_batch` 按 open_id 精确补齐姓名。旧版本输出的 `user_id` 实为 open_id，
依赖旧值的脚本应改读 `open_id`。
根据输入实际类型设置 `--user-id-type` 或 `--department-id-type`，不要靠 token 前缀猜测邮箱或手机号。

查询前确认应用具备通讯录只读 scope。示例只能使用 `user@example.com` 等占位信息。
