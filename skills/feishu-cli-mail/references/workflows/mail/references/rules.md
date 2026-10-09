# 收信规则（mail rule-*）

收信规则按执行顺序生效。CLI 用语义别名描述条件与动作，自动编码为服务端整数枚举，并在输出中给出中文描述。
全部 `rule-*` 命令（含只读的 `rule-list/rule-get`）都必须 User Token；读需要 `mail:user_mailbox.rule:read`，
写需要 `mail:user_mailbox.rule:write`（除 `rule-create` 外的写命令会先读取规则，两个 scope 都要）。

## 语法

条件语法 `field[:operator[:value]]`，动作语法 `kind[:folder_id=<ID>]`：

- 条件字段：`from`、`to`、`cc`、`to_or_cc`、`subject`、`body`、`attachment_name`、`attachment_type`、`any_address`；
  不带操作符的布尔条件：`all_mail`、`external`、`spam`、`not_spam`、`has_attachment`
- 操作符：`contains`、`not_contains`、`starts_with`、`ends_with`、`equals`、`not_equals`，以及不带值的 `contains_self`、`empty`；
  其余操作符缺值时报用法错误
- 动作：`archive`、`delete_mail`、`mark_read`、`move_spam`、`not_spam`、`star`、`mute_notification`、
  `move_folder:folder_id=<文件夹 ID>`（文件夹 ID 用 `mail triage --list-folders` 查看）；CLI 不提供自动转发、分享到会话、添加用户标签动作

条件也可用 `--conditions` 传 JSON 数组（每项为语法字符串、`{"field","operator","value"}` 或原始 `{"type","operator","input"}`），
动作用 `--actions`（语法字符串、`{"kind","folder_id"}` 或原始 `{"type","input"}`），两者都支持 `@文件`。
`--match all|any` 默认 `all`。

## 命令

```bash
feishu-cli mail rule-list                                   # 按执行顺序列出，含中文描述；-o json 为 {rules, count}
feishu-cli mail rule-get --rule-id <rule_id>                # 服务端无单条接口，CLI 从列表中查找
feishu-cli mail rule-create --name "老板邮件加旗标" \
  --condition from:contains:boss@example.com --action star --dry-run   # 先预览请求体与中文描述
feishu-cli mail rule-create --name "周报归档" --conditions @conds.json \
  --action mark_read --action move_folder:folder_id=<文件夹ID> --disable   # 先建为停用，确认后再启用
feishu-cli mail rule-update --rule-id <rule_id> --action mark_read --action star   # 未指定字段保持不变
feishu-cli mail rule-enable --rule-id <rule_id>
feishu-cli mail rule-disable --rule-id <rule_id>
feishu-cli mail rule-reorder --move-rule-id <rule_id> --to-top
feishu-cli mail rule-delete --rule-id <rule_id> --yes
```

## 决策与坑点

- `rule-update/enable/disable` 先读取当前规则，只覆盖显式传入的字段再整体写回（服务端为 PUT 全量语义，无乐观锁）；
  未传 `--match` 时保持原匹配方式。传入条件/动作时整体替换对应列表，不是追加。
- `--enable/--disable`、`--stop-after-match/--continue-after-match` 成对互斥。
- `rule-reorder` 二选一：`--rule-ids` 给完整顺序（必须恰好包含全部现有规则各一次），或 `--move-rule-id` 搭配
  `--before-rule-id/--after-rule-id/--to-top/--to-bottom` 之一。
- 所有写操作支持 `--dry-run`，且优先于确认。`rule-create`、`rule-reorder --rule-ids`、`rule-delete` 的预览给出完整请求；
  `rule-update/enable/disable` 与移动模式的 `rule-reorder` 预览只列出修改字段和读取/写回步骤，不联网、不校验规则是否存在。
- `rule-delete` 会先读取规则并在确认提示中展示描述；非交互环境必须带 `--yes`，否则以退出码 10 结束、不删除。
  `delete_mail`、`all_mail` 这类影响面大的规则，先向用户复述中文描述再创建。
