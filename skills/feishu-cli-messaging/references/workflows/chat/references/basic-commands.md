# 群聊管理详细参考

## 群聊 CRUD

### 创建群聊

```bash
feishu-cli chat create \
  --name "项目讨论群" \
  [--description "群描述"] \
  [--owner-id <user_id>] \
  [--user-ids id1,id2,id3] \
  [--chat-type private|public]
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--name` | string | 必填 | 群名称 |
| `--description` | string | — | 群描述 |
| `--owner-id` | string | — | 群主 ID |
| `--user-ids` | string | — | 邀请成员 ID（逗号分隔） |
| `--chat-type` | string | `private` | 群类型：private（私有）/ public（公开） |

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
  [--owner-id <new_owner_id>]
```

至少需要指定一个参数。

### 解散群聊

```bash
feishu-cli chat delete <chat_id>
```

操作不可逆，会有确认提示；非交互环境（Agent/脚本）未带 `--yes` 时不执行并以退出码 10 失败，获得用户同意后追加 `--yes` 重试。

### 获取群分享链接

```bash
feishu-cli chat link <chat_id> [--validity-period week|year|permanently]
```

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

输出 `users[]`（= 旧字段 `items[]`，仅用户）、`bots[]`（群内机器人，含 `app_id`）、`truncations[]`
（服务端截断名单时非空）、`user_total` / `bot_total`。

### 添加群成员

```bash
feishu-cli chat member add <chat_id> \
  --id-list id1,id2,id3 \
  [--member-id-type open_id|user_id|union_id|app_id]
```

### 移除群成员

```bash
feishu-cli chat member remove <chat_id> \
  --id-list id1,id2 \
  [--member-id-type open_id|user_id|union_id|app_id]
```

## 成员 ID 类型

| 值 | 说明 |
|---|------|
| `open_id` | Open ID（默认） |
| `user_id` | User ID |
| `union_id` | Union ID |
| `app_id` | 应用 ID（仅用于添加机器人） |

## 权限要求

| 权限 | 说明 |
|------|------|
| `im:chat` | 群聊创建/修改/删除 |
| `im:chat:read` | 群聊信息读取 |
| `im:chat:member` | 群成员添加/移除 |

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
