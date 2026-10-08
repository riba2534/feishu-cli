# 收信规则（mail rule-*）

收信规则按执行顺序生效。CLI 用语义别名描述条件与动作，自动编码为服务端整数枚举，并在输出中给出中文描述。
`update / enable / disable` 先读取当前规则，只覆盖显式传入的字段再整体写回（服务端为 PUT 全量语义，无乐观锁）；
`delete` 走确认门禁（非交互需 `--yes`）；所有写操作支持 `--dry-run`（优先于确认）。

条件语法 `field[:operator[:value]]`，动作语法 `kind[:folder_id=<ID>]`：

- 条件字段：`from`、`to`、`cc`、`to_or_cc`、`subject`、`body`、`attachment_name`、`attachment_type`、`any_address`；无操作符的布尔条件：`all_mail`、`external`、`spam`、`not_spam`、`has_attachment`
- 操作符：`contains`、`not_contains`、`starts_with`、`ends_with`、`equals`、`not_equals`、`contains_self`（不带值）、`empty`（不带值）
- 动作：`archive`、`delete_mail`、`mark_read`、`move_spam`、`not_spam`、`star`、`mute_notification`、`move_folder:folder_id=<文件夹 ID>`（自动转发/分享到会话/添加用户标签服务端暂不支持）

```bash
feishu-cli mail rule-list                                   # 按执行顺序列出，含中文描述
feishu-cli mail rule-create --name "老板邮件加旗标" \
  --condition from:contains:boss@example.com --action star --dry-run   # 先预览请求体
feishu-cli mail rule-update --rule-id 701xxx --action mark_read --action star   # 未指定字段保持不变
feishu-cli mail rule-disable --rule-id 701xxx
feishu-cli mail rule-reorder --move-rule-id 701xxx --to-top
feishu-cli mail rule-delete --rule-id 701xxx --yes
```

条件也可用 `--conditions` 传 JSON 数组（每项为语法字符串、`{"field","operator","value"}` 或原始 `{"type","operator","input"}`），
动作用 `--actions`（语法字符串、`{"kind","folder_id"}` 或原始 `{"type","input"}`），支持 `@文件`。
`rule-update` 传入条件/动作时整体替换对应列表；`--enable/--disable`、`--stop-after-match/--continue-after-match` 成对互斥。
`rule-reorder` 用 `--rule-ids` 给完整顺序（必须恰好包含全部现有规则各一次），或用 `--move-rule-id` 搭配
`--before-rule-id/--after-rule-id/--to-top/--to-bottom` 之一。
