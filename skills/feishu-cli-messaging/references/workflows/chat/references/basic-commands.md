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
