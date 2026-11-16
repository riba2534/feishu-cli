# 群聊管理详细参考

## 群聊 CRUD

### 创建群聊

```bash
feishu-cli chat create \
  --name "项目讨论群" \
  [--description "群描述"] \
  [--owner-id ou_xxx] \
  [--user-ids ou_xxx,ou_yyy] \
  [--bots cli_xxx] \
  [--chat-type private|public] \
  [--chat-mode group|topic] \
  [-o json]
```

- 只走应用身份（Bot），不能切 User Token；不传 `--owner-id` 时 Bot 为群主。
- 约束：`--name` ≤60 字（公开群至少 2 字）、`--description` ≤100 字、`--user-ids` 最多 50 个 open_id、
  `--bots` 最多 5 个 app_id；`--chat-mode topic` 创建话题群。
- `-o json` 输出 `{chat_id, name, chat_type, chat_mode, owner_id, external, share_link}`；分享链接获取失败不影响建群。

### 从私聊原生建群并同步（实验性）

`chat create-sync` 调用未公开的网页端 `PutChatRequest`（`fromChatId` + `initMessageIds`），
从双人私聊创建普通私有群并原生同步消息，**不是转发或合并转发**。新群包含私聊双方，群主是网页登录用户。
这是实验性网页接口，以当前 `chat create-sync --help` 为契约，不视为公开 OpenAPI 的稳定能力。

- `--name`、`--from-chat-id`、`--idempotency-key` 必填；群名最多 60 字，请求键最多 50 字。
- 网页 session 默认 `~/.larkx/credentials.json`，可用 `--web-session-file` 指定已有文件；不自动登录或更新凭据，不支持 OAuth Token。
  文件必须是普通 JSON、非符号链接、最多 1 MB，Unix 权限为 `0600`。格式为 `{"region":"feishu","user_id":"123456789","cookies":{"session":"xxx"},"web_app_id":"161471"}`，所有值均为占位，需复用本人真实已有登录态；国际版 `region` 为 `lark`。
- 会话、消息及计划中的成员 ID 都是网页端数字 native ID，与 `oc_` / `om_` / `ou_` 等 OpenAPI ID 不同，不可互换。下面数字仅为占位示例，不可用于真实操作。
- `--last-messages N`（1 到 50）与 `--message-ids`（逗号分隔，最多 50 个）必须且只能选一个。
  最近 N 条目前只从最近 100 个位置中筛选非系统、非空消息，数量不足时拒绝建群，不自动扩大范围。
- `--dry-run` 只校验、预览参数，不读取登录态、不联网；`--preview` 只读联网核对实际双方和消息并输出 JSON，不建群。
  写操作先展示计划并确认，非交互未带 `--yes` 时以退出码 10 拒绝执行；只有获得用户批准后才追加 `--yes`。
- 同一操作重试必须复用 `--idempotency-key`，同键重复执行只核验已有群、不重复创建。
  同键的参数、身份或源消息变化时拒绝执行；使用最近 N 条时，新消息会改变选择范围，不能自动换键重建。
  网络结果未知或进程异常退出时拒绝自动重建，先检查群列表与回执，不换键绕过。
- Bot 建群后另用现有 `chat member add <open_chat_id> --id-list cli_xxx --member-id-type app_id` 加入，先确认群与 Bot。
  建群回执的 `native_chat_id` 不能直接当 OpenAPI `chat_id`，需另行核验目标群的 open ID。

以下示例均离线，不需要网页登录态：

```bash
feishu-cli chat create-sync --help
feishu-cli chat create-sync --name "消息同步测试" --from-chat-id 123456789 \
  --last-messages 3 --idempotency-key sync-example --dry-run
feishu-cli chat create-sync --name "消息同步测试" --from-chat-id 123456789 \
  --message-ids 111111111,222222222 --idempotency-key sync-explicit-example --dry-run
feishu-cli chat member add --help
```

只读核对时，先将占位 ID 替换为已确认的真实数字 ID，再把 `--dry-run` 改为 `--preview`。
实际建群需去掉预览参数并确认计划，不能把离线参数预览当成成员、消息或建群结果已验证。

协议字段参考 [LarkAgentX 的公开定义（固定版本）](https://github.com/cv-cat/LarkAgentX/blob/97ad48e69e34bd8c78ee30f795b7c3759dd81659/larkx/proto/lark_all_pb2.py)。实现只封装本功能需要的 protobuf 字段，不引入该项目的执行代码。

### 获取群聊信息

```bash
feishu-cli chat get <chat_id> [--as bot|user|auto]
```

`chat get/update/delete` 支持 `--as`：默认 auto（已登录用 User Token，未登录回退 Bot）；`--as bot` 需 Bot 在群内。

### 更新群聊信息

```bash
feishu-cli chat update <chat_id> \
  [--name "新群名"] \
  [--description "新描述"] \
  [--owner-id <new_owner_id>] \
  [--as bot|user|auto]
```

至少需要指定一个参数。

### 解散群聊

```bash
feishu-cli chat delete <chat_id> [--yes] [--as bot|user|auto]
```

操作不可逆，会有确认提示；非交互环境（Agent/脚本）未带 `--yes` 时不执行并以退出码 10 失败，获得用户同意后追加 `--yes` 重试。

### 获取群分享链接

```bash
feishu-cli chat link <chat_id> [--validity-period week|year|permanently]
```

固定应用身份（Bot 需在群内）。

| validity-period | 说明 |
|----------------|------|
| `week` | 一周有效（默认） |
| `year` | 一年有效 |
| `permanently` | 永久有效 |

## 群成员管理

### 列出群成员

```bash
feishu-cli chat member list <chat_id> \
  [--member-id-type open_id|user_id|union_id] \
  [--member-types user|bot|user,bot] \
  [--page-size 20] \
  [--page-token <token>] [--page-all] [--as bot|user|auto]
```

始终输出 JSON：`users[]`（= 旧字段 `items[]`，仅用户）、`bots[]`（群内机器人，含 `app_id`）、`truncations[]`
（服务端截断名单时非空）、`user_total` / `bot_total`。不接受 `-o`。

### 添加群成员

```bash
feishu-cli chat member add <chat_id> \
  --id-list id1,id2,id3 \
  [--member-id-type open_id|user_id|union_id|app_id] \
  [--as bot|user|auto]
```

### 移除群成员

```bash
feishu-cli chat member remove <chat_id> \
  --id-list id1,id2 \
  [--member-id-type open_id|user_id|union_id|app_id] \
  [--as bot|user|auto]
```

加人、移人没有确认门禁，执行前确认群和名单。

## 成员 ID 类型

| 值 | 说明 |
|---|------|
| `open_id` | Open ID（默认） |
| `user_id` | User ID |
| `union_id` | Union ID |
| `app_id` | 应用 ID（仅用于添加机器人） |

## 权限要求

任一满足即可（以 `feishu-cli schema im.chats.<method>` / `im.chat.members.<method>` 输出为准）：

| 操作 | scope |
|------|------|
| 建群 | `im:chat` / `im:chat:create` |
| 读群信息 | `im:chat` / `im:chat:read` / `im:chat:readonly` |
| 改群信息 | `im:chat` / `im:chat:update` |
| 读成员 | `im:chat` / `im:chat:readonly` / `im:chat.members:read` |
| 加人、移人 | `im:chat` / `im:chat.members:write_only` |

## 示例场景

### 创建项目群并添加成员

```bash
# 1. 创建群聊
feishu-cli chat create --name "Q1 项目组" --description "Q1 季度项目讨论"

# 2. 获取群 ID（从创建结果中）
# chat_id: oc_xxxx

# 3. 添加成员
feishu-cli chat member add oc_xxxx --id-list ou_aaa,ou_bbb,ou_ccc

# 4. 获取群分享链接
feishu-cli chat link oc_xxxx --validity-period year
```
