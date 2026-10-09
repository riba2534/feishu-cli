# 飞书外部群操作完整指南

> 凡是用户/Agent 碰到外部群（external=true）相关的 232033 错误，**必读本文档**。

## 一句话核心

飞书外部群的「群信息 / 群成员 / 群配置 / 群标签页 / 群公告」类 API 默认 232033 拒绝，
**只有「开启了对外共享能力的 App」且「该 App 的 Bot 已加入该群」时**才能调。
切到正确的 App + Bot 已在群里，就能拉到完整成员列表（含群昵称）。

## 受影响的 API 全集

下列 API 在外部群上默认被 232033 拒绝：

| API 路径 | 用途 |
|---|---|
| `GET /im/v1/chats/{id}` | 群基础信息 |
| `PUT /im/v1/chats/{id}` | 更新群信息 |
| `DELETE /im/v1/chats/{id}` | 解散群 |
| `GET /im/v1/chats/{id}/members`、`/members/list`（`chat member list` 使用后者） | **群成员列表（含 name = 群昵称）** |
| `POST /im/v1/chats/{id}/members` | 加成员 |
| `DELETE /im/v1/chats/{id}/members` | 移除成员 |
| `GET /im/v1/chats/{id}/members/is_in_chat` | 查我/Bot 是否在群 |
| `POST /im/v1/chats/{id}/link` | 获取群分享链接 |
| `GET /im/v1/chats/{id}/moderation` | 群发言权限 |
| `GET /im/v1/chats/{id}/announcement` | 群公告 |
| `POST /im/v1/chats/{id}/chat_tabs` | 群标签页 |
| 等所有「群内信息/配置」类 | |

**不受影响**（外部群也能调）：
- `msg history` / `msg list` / `msg get` —— 读消息历史可以（调用者需在群内）
- `msg search-chats`、`chat list` —— 搜群、列群可以（含外部群）
- `msg send/reply/forward` —— 发消息可以
- `chat create` —— 创建群（不涉及外部群读取）

## 解锁条件

需要**同时**满足两个条件：

### 条件 1：App 开启「对外共享能力」

去 [飞书开放平台](https://open.feishu.cn/app) → 选你的 App → **凭证与基础信息**
找「应用市场分发能力」/「对外共享」开关，开启并按提示提交（可能需要审核）。

> 这是 App 级别的配置，不是 scope。开启后整个 App 的"外部能力"被解锁。

### 条件 2：Bot 实际加入此群

被群管理员邀请进群。或群成员通过分享链接邀请（如果群允许）。

> Bot 在不在群里：`feishu-cli chat member list <oc_xxx> --as user --member-types bot`（看 `bots[].app_id` 里有没有你的 App），
> 或 `feishu-cli chat member list <oc_xxx> --as bot` 能否成功
> 失败码不同：232033 = App 没对外共享；232011 = Bot 不在群里

## 实操工作流

### 场景 A：你已经有「开了对外共享能力」的 App

直接切换 App 调用即可，优先用不写盘、不改当前指针的方式：

```bash
# 方式 1：单次覆盖 App 凭证（不写盘；secret 会进入 shell 历史与 ps 输出，共享机器慎用）
feishu-cli --bot-app-id cli_xxx --bot-app-secret xxx chat member list oc_yyy --as bot

# 方式 2：环境变量单次生效
FEISHU_APP_ID=cli_xxx FEISHU_APP_SECRET=xxx feishu-cli chat member list oc_yyy --as bot

# 方式 3：长期使用时建独立 profile，按命令指定（不切换全局指针）
printf '%s' "$EXT_APP_SECRET" | feishu-cli profile add ext-bot --app-id cli_xxx --app-secret-stdin --probe
feishu-cli --profile ext-bot chat member list oc_yyy --as bot
```

新建 profile 会写入本机配置，先征得用户同意；不要直接改默认 `config.yaml`。

### 场景 B：你没有对外共享 App

需要去开发者后台开启对外共享能力（见上文条件 1），等审核通过后让 Bot 加群再调。

或者**临时替代方案**：用 `msg history` 拿"发过言的"外部商家列表（覆盖率有限，没发言的潜水商家拿不到），具体见 [chat workflow 的名字反解策略](../workflow.md#名字反解策略)。

### 场景 C：你不知道 chat_id 是不是外部群

```bash
# chat list 的每一项都有布尔 external 字段
feishu-cli chat list --page-all -o json | jq '.items[] | select(.chat_id=="oc_xxx") | .external'

# 或按群名搜：外部群会出现 "external": true，内部群省略该字段
feishu-cli msg search-chats --query "群名关键词" -o json
```

## 命令支持

`feishu-cli chat member list/add/remove` 命令都已经支持 `--as bot|user|auto`：

```bash
# 默认 auto（User 优先，回退 Bot）
feishu-cli chat member list oc_xxx

# 强制 Bot 身份（外部群推荐）
feishu-cli chat member list oc_xxx --as bot

# 强制 User 身份（如果你是群管理员且 App 已对外共享）
feishu-cli chat member list oc_xxx --as user
```

`chat get/update/delete/link/list/member` 收到 232033 / 232011 / 232006 时会自动打印中文解决方案。

## 输出示例

调对外共享 App + Bot 在群里时（字段结构，值为示意）：

```bash
$ feishu-cli --bot-app-id cli_xxx --bot-app-secret xxx chat member list oc_xxx --as bot
{
  "users": [
    {"member_id": "ou_aaa", "member_id_type": "open_id", "name": "张三", "tenant_key": "tk_aaa"},
    {"member_id": "ou_bbb", "member_id_type": "open_id", "name": "李四", "tenant_key": "tk_bbb"}
  ],
  "items": ["…与 users 相同…"],
  "bots": [],
  "truncations": [],
  "user_total": 2,
  "bot_total": 0,
  "has_more": false,
  "page_token": ""
}
```

`truncations` 非空表示服务端因群安全设置截断了名单（不完整），stderr 也会告警。

## name 字段的含义

API 返回的 `name` 字段：
- **优先**：用户在该群设置的「群昵称」（如果设置了）
- **回落**：用户全局展示名（如果没设群昵称）

所以 `chat member list` 拿到的 name **就是该群里看到的显示名**，可直接拿来做名字规范检查。

## 排错速查

| 错误码 | 含义 | 解决 |
|---|---|---|
| 232033 | App 没对外共享 / Bot 不在群 | 切对外共享 App + 确认 Bot 在群 |
| 232011 | 操作者（Bot/User）不在群里 | 让群管理员邀请进群 |
| 232006 | chat_id 无效 | 用 `msg search-chats` 重查 |
| 232025 | App 未启用机器人能力 | 飞书开放平台 → 应用 → 应用能力 → 添加机器人 |
| 41050 | 跨企业用户 user info 不可见 | 正常，外部用户的 contact 默认不开放 |

## 排查原则

碰到 232033，第一反应是"当前 App 是否开了对外共享、Bot 是否在群里"，而不是"飞书不支持"；
不要绕路用 `api` 命令、浏览器或 `msg history` 硬凑成员名单。

---

## 重大陷阱：外部群里 sender_id 和 member_id 是不同 namespace

**结论先行**：`msg history` 拿到的 `sender.id`（open_id）和 `chat member list` 拿到的 `member_id`（open_id），**即使用同一个 App 同一个 Bot Token 调，在外部群下也是完全独立的两套 ID 系统**。**0 交集**。无法用 member 反查 sender 的名字。

### 实测证据

```
样本群: oc_xxx (external=true)
环境:   FEISHU_APP_ID=对外共享App, FEISHU_APP_SECRET=xxx, --as bot

chat member list 拿到 member_id 数: 100
msg history     拿到 user sender_id 数: 22

两者 open_id 交集:       0
两者 tenant_key 交集:    0
```

### 为什么

飞书对外部群成员信息实施了**两套独立 ID 系统**：
- **"群通讯录视图"**（`chat member list` API 走的）：返回 100 人，是群里实际成员的"对外身份"
- **"消息系统视图"**（`msg history` API 走的）：返回的 sender_id 是消息系统**给外部用户分配的另一套 ID**，跟通讯录视图独立

推测目的：**保护外部商家身份隐私 + 防止跨群追踪**。商家在不同群里、对不同 App 表现的 ID 都不同，App 无法把"群 A 发言者 ou_xxx" 关联到"群 B 发言者 ou_yyy"。

### 这意味着什么

| 想做的事 | 能不能做 |
|---|---|
| 拉群完整成员名单（含群昵称） | ✅ `chat member list` 100% 拿到 |
| 知道某个发言者叫什么 | ⚠ 靠 `sender_names` / `items[].sender_name`（服务端回填，含外部用户）+ `mentions[].name`；仍可能有解不出的 |
| 用 member.member_id 反查 sender 名字 | ❌ **完全做不到**（不同 namespace） |
| 把消息按"群里实际的人"分组 | ❌ 做不到（同上） |

### 在 feishu-cli 中的处理

`feishu-cli msg history -o json` 在 `oc_` 会话上**会自动拉一次成员列表**（成功的话），把结果**单独输出**到顶层 `chat_members` 字段，而不是混进 `sender_names` 字典，并附带 `chat_members_note` 提示，避免误用（文本模式不拉成员）：

```json
{
  "items": [...],
  "sender_names": {"ou_xx": "李四"},
  "chat_members": [
    {"member_id": "ou_yy", "name": "张三", "tenant_key": "xxx"},
    ...100 人
  ],
  "chat_members_note": "chat_members 是该群完整成员名单（含群昵称），但因为飞书外部群的 ID 隔离机制，无法直接通过 member_id lookup 到 sender_id。两者请独立使用。"
}
```

**用法指引**：
- 想知道"群里都有哪些人，名字规不规范" → 看 `chat_members` 字段
- 想知道"这条消息是谁发的" → 看 `items[].sender_name` / `sender_names`
- **不要**试图用 `chat_members[*].member_id` 去匹配 `items[].sender.id`，永远匹配不上

### 何时 chat_members 字段为空

- 当前 App 没开「对外共享能力」→ 成员接口返回 232033 → 静默降级，字段不输出
- Bot / 当前用户不在该群 → 同理
- 文本模式（没有 `-o json`）或容器是话题 `omt_xxx` → 不拉成员
- 群聊、话题群、私聊（`oc_` 会话）在 JSON 模式下都会尝试拉取

要让 chat_members 有数据，切到对外共享 App + `--as bot` 即可。
