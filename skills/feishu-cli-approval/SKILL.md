---
name: feishu-cli-approval
description: >-
  飞书审批操作（查询 + 写入）。读：definition detail / task query。
  写：instance {create,cancel,cc} + task {approve,reject}（v1.23+ 新增）。
  优先 User Token（自动从 token.json 读，可 --user-access-token 覆盖）；
  fallback Tenant Token（部分写命令仅 user 身份合法）。
  当用户请求"提交审批"、"审批通过/拒绝"、"撤回审批"、"抄送"、"审批查询"时使用。
argument-hint: instance create/cancel/cc | task approve/reject | definition detail | task query
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书审批技能（查询 + 写入）

通过 feishu-cli 完成审批全生命周期：查询审批定义 / 待办任务，发起 / 撤回 / 抄送审批实例，通过 / 拒绝审批任务。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 核心概念

飞书审批由四级对象组成，命令按对象分组：

| 对象 | 说明 | 唯一 ID | CLI 子命令 |
|------|------|---------|-----------|
| **definition** | 审批定义（审批流模板，行政/财务后台配置） | `approval_code` | `approval get` |
| **instance** | 审批实例（一次具体的发起，绑定一个 definition + 一份 form） | `instance_code` | `approval instance {create,cancel,cc}` |
| **task** | 审批任务（实例分发到每个审批节点上的待办） | `task_id` | `approval task {query,approve,reject}` |
| **cc** | 抄送（把实例送到其他用户阅知，非审批节点） | — | `approval instance cc` |

**生命周期**：`get definition` → 提交 form 触发 `instance create` → 节点用户 `task approve/reject` → 发起人可中途 `instance cancel` 或 `instance cc` 抄送他人。

## 身份说明（Token 策略）

所有写命令（instance create/cancel/cc、task approve/reject）默认走 `resolveOptionalUserTokenWithFallback`：

1. `--user-access-token` 参数（最高优先级）
2. `FEISHU_USER_ACCESS_TOKEN` 环境变量
3. `~/.feishu-cli/token.json`（OAuth 登录态，过期自动用 refresh_token 刷新）
4. 都没有时回退 Tenant Token（App 身份）

**重要**：审批写 API 在飞书侧大多只接受 **User Token**（用户必须是审批流可见人 / 发起人 / 节点审批人）。Tenant Token 调用 `task approve` 会被服务端拒绝。建议执行写操作前先 `feishu-cli auth login`。

`approval get` 用 Tenant Token 即可。`approval task query` 通过 `resolveCurrentAuthedUserID` 自动从登录态推断 `open_id`，必须先 login。

### 所需 scope

| 命令 | scope | Token 类型 |
|------|-------|-----------|
| `approval get` | `approval:approval:readonly` | Tenant 即可 |
| `approval task query` | `approval:task` | **User Token 必需** |
| `approval instance {create,cancel,cc}` | `approval:approval`（含 `approval:instance:write`） | **User Token 必需** |
| `approval task {approve,reject}` | `approval:approval`（含 `approval:task:write`） | **User Token 必需** |

```bash
feishu-cli auth check --scope "approval:approval approval:task"
feishu-cli auth login --scope "approval:approval approval:task offline_access"
```

## 命令速查

### 读

```bash
# 查审批定义（拿表单结构 / 节点列表，发起前必看）
feishu-cli approval get <approval_code>
feishu-cli approval get <approval_code> --output raw-json   # 原始 API

# 查我的审批任务
feishu-cli approval task query --topic todo                 # 待我审批
feishu-cli approval task query --topic done                 # 我已审批
feishu-cli approval task query --topic started              # 我发起的
feishu-cli approval task query --topic cc-unread            # 抄送给我（未读）
feishu-cli approval task query --topic cc-read              # 抄送给我（已读）
```

### 写（v1.23+ 新增）

```bash
# 发起审批实例（form 必须是 JSON 数组）
feishu-cli approval instance create \
  --approval-code <code> \
  --user-id ou_xxx \
  --form-file form.json
#   或：--form '[{"id":"widget_1","type":"input","value":"内容"}]'

# 撤回（取消）已发起的审批实例（只有发起人能撤）
feishu-cli approval instance cancel \
  --approval-code <code> \
  --instance-code <ic> \
  --user-id ou_xxx

# 抄送实例给其他用户（逗号分隔，自动去重保留首次顺序）
feishu-cli approval instance cc \
  --approval-code <code> \
  --instance-code <ic> \
  --user-id ou_xxx \
  --cc-user-ids ou_a,ou_b \
  --comment "请知悉"

# 通过 / 拒绝审批任务
feishu-cli approval task approve \
  --approval-code <code> --instance-code <ic> --task-id <task> --user-id ou_xxx \
  --comment "同意"

feishu-cli approval task reject \
  --approval-code <code> --instance-code <ic> --task-id <task> --user-id ou_xxx \
  --comment "金额超预算"
```

## 关键 flag

### `--approval-code`（所有写命令必填）

审批定义 code，可从 `approval get` 输出或飞书后台审批管理页 URL 拿到。所有命令都先用 `isValidToken` 校验格式。

### `--user-id` + `--user-id-type`

操作人用户 ID。默认 `--user-id-type open_id`（`ou_xxx`），也支持 `user_id` / `union_id`。

- `instance create`：发起人 ID
- `instance cancel`：必须是发起人（飞书侧校验）
- `instance cc`：发起抄送的用户（通常仍是发起人）
- `task approve/reject`：节点上的审批人 ID（不是发起人！）

### `--form` 与 `--form-file`（`instance create` 二选一）

**form 必须是 JSON 数组**，否则 CLI 在客户端先报 "表单数据必须是 JSON 数组，解析失败"。每个元素对应审批定义中的一个 widget：

```json
[
  {"id": "widget_1", "type": "input",    "value": "差旅报销 1500"},
  {"id": "widget_2", "type": "number",   "value": 1500},
  {"id": "widget_3", "type": "textarea", "value": "上海出差 3 天"}
]
```

widget 的 `id` / `type` 从 `approval get --output raw-json` 的 form 字段读，**不要手编**。常见 type：`input` / `textarea` / `number` / `radio` / `checkbox` / `date` / `attachmentV2` / `fieldList`（明细控件，value 是数组的数组）。

### `--cc-user-ids`（`instance cc` 必填）

逗号分隔列表，例如 `ou_a,ou_b,ou_a`。CLI 通过 `parseCommaSeparatedIDs` 自动 trim + 去重，保留首次出现顺序，所以重复传同一个 ID 只会抄送一次。空字符串会被过滤。

### `--comment`（可选，approve/reject/cc 共用）

审批意见 / 抄送备注。`task reject` 建议必填拒绝原因；`task approve` 通常可省。

### `--open-chat-id`（仅 `instance create`）

审批结果推送到的群 ID，发起后自动在该群发卡片更新审批状态。可选。

### `--user-access-token`

覆盖 token 解析链最顶端。多数情况无需指定，自动从 `~/.feishu-cli/token.json` 读已登录态。

## 完整用例

### 例 1：从零到通过一条报销审批

```bash
# 1. 登录并预检 scope
feishu-cli auth login --scope "approval:approval approval:task offline_access"

# 2. 查审批定义拿 widget 结构
feishu-cli approval get 7AB12C... --output raw-json | jq '.form'

# 3. 准备 form.json
cat > /tmp/form.json <<'EOF'
[
  {"id": "widget_1", "type": "input",  "value": "差旅报销"},
  {"id": "widget_2", "type": "number", "value": 1500}
]
EOF

# 4. 发起实例（拿到 instance_code）
feishu-cli approval instance create \
  --approval-code 7AB12C... \
  --user-id ou_self \
  --form-file /tmp/form.json
# → 输出：审批实例已创建  instance_code: 8XY99Z...

# 5. 节点审批人通过任务（先在节点审批人电脑/账号上 auth login）
feishu-cli approval task query --topic todo   # 拿到 task_id
feishu-cli approval task approve \
  --approval-code 7AB12C... \
  --instance-code 8XY99Z... \
  --task-id 99TASK... \
  --user-id ou_approver \
  --comment "同意"
```

### 例 2：发起后撤回 + 抄送

```bash
# 发起人撤回
feishu-cli approval instance cancel \
  --approval-code <code> --instance-code <ic> --user-id ou_self

# 抄送给 2 个同事（重复 ID 会去重）
feishu-cli approval instance cc \
  --approval-code <code> --instance-code <ic> \
  --user-id ou_self --cc-user-ids ou_a,ou_b,ou_a \
  --comment "供参考"
```

## 踩坑

| 问题 | 原因 | 解决 |
|------|------|------|
| `表单数据必须是 JSON 数组，解析失败` | `--form` 传了 `{...}` 对象 | 包成数组 `[{...}]`，飞书 form 顶层永远是数组 |
| `--cc-user-ids` 重复 ID 抄送多次 | 不会，CLI 已去重（`parseCommaSeparatedIDs`） | 如需多次提示，多次执行 `instance cc` |
| `task approve` 返回 forbidden | 用了 Tenant Token / 操作人不是节点审批人 / 实例已结束 | `auth login` 用节点审批人账号；先 `task query --topic todo` 确认 task 还在 |
| `instance cancel` 失败 | 操作人不是发起人 / 实例已审批完成 | 只有发起人能撤；已通过/拒绝的实例无法撤 |
| `widget id` 找不到 | 手编 ID，没对上后台定义 | 先 `approval get --output raw-json` 看 `form` 字段 |
| `auth login` 没传 `offline_access` | token 1h 后过期不能自动刷新 | 重新 login 显式加 `--scope "... offline_access"`，Device Flow 已自动注入但确认下 |
| 写命令默认走 token.json，没登录态时静默回退 Tenant | 调用立即 403 | 写之前先 `feishu-cli auth status` 看登录情况 |

## 输出格式

写命令默认输出单行成功摘要：

```
审批实例已创建
  instance_code: 8XY99Z...
```

读命令 `approval get` / `task query` 支持：

- 不传 `--output`：人类可读文本摘要
- `--output json`：CLI 归一化 JSON
- `--output raw-json`：飞书 API 原始响应（拿 widget 结构 / debug 必备）

写命令暂未实现 `--output json`，需要拿 `instance_code` 程序消费的话从 stdout 抓字符串。

## 不在本技能范围

| 需求 | 走哪里 |
|------|--------|
| 审批流可视化设计（拖拽节点、配置审批人、设置可见范围） | 飞书后台「审批管理」Web UI，CLI 不覆盖 |
| 转交 / 退回 / 加签 / 催办（`tasks/transfer` / `rollback` / `add_sign` / `remind`） | 暂未实现，按需 PR |
| 审批回调订阅 / Webhook 处理 | 不属于 CLI 职责，走开放平台事件订阅 |
| 审批结果通知到群（消息卡片） | `instance create --open-chat-id <chat>` 内置；或 `feishu-cli-msg` 发自定义卡片 |
| 给审批文档评论 / 加权限 | `feishu-cli-perm` / 评论命令；审批本身不挂在云文档体系 |

## 相关 skill

- `/feishu-cli-auth` — OAuth 登录、scope 预检、token 状态
- `/feishu-cli-toolkit` — 综合查询入口（也有 approval get / task query 速查段）
- `/feishu-cli-msg` — 审批结果二次通知到群 / 个人
