# 搜索命令详细参考

**重要**：搜索 API 需要 **User Access Token**，不能使用 App Access Token。

## User Access Token 获取方式

按优先级从高到低：

1. **`feishu-cli auth login`（推荐）**：一键 OAuth 登录，Token 自动保存并支持过期自动刷新，无需每次手动传 Token
2. **命令行参数**：`--user-access-token <token>`
3. **环境变量**：`FEISHU_USER_ACCESS_TOKEN=<token>`

Token 有效期约 2 小时，Refresh Token 有效期 30 天。使用 `auth login` 方式时，Access Token 过期后会自动用 Refresh Token 刷新。

## 搜索消息

```bash
feishu-cli search messages "关键词" \
  --user-access-token <token> \
  [--chat-ids oc_xxx,oc_yyy] \
  [--from-ids ou_xxx] \
  [--at-chatter-ids ou_xxx] \
  [--message-type file|image|media] \
  [--chat-type group_chat|p2p_chat] \
  [--from-type bot|user] \
  [--start-time 1704067200] \
  [--end-time 1704153600] \
  [--page-size 20] \
  [--page-token <token>]
```

### 筛选参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `--chat-ids` | string | 限定群聊范围（逗号分隔） |
| `--from-ids` | string | 限定发送者（逗号分隔） |
| `--at-chatter-ids` | string | 限定被@的用户（逗号分隔） |
| `--message-type` | string | 消息类型：`file`/`image`/`media` |
| `--chat-type` | string | 会话类型：`group_chat`（群聊）/`p2p_chat`（单聊） |
| `--from-type` | string | 发送者类型：`bot`（机器人）/`user`（用户） |
| `--start-time` | string | 起始时间（Unix 秒级时间戳） |
| `--end-time` | string | 结束时间（Unix 秒级时间戳） |

### 示例

```bash
# 搜索特定群里的文件消息
feishu-cli search messages "周报" \
  --user-access-token u-xxx \
  --chat-ids oc_xxx \
  --message-type file

# 搜索某时间段内的消息
feishu-cli search messages "上线" \
  --user-access-token u-xxx \
  --start-time 1704067200 \
  --end-time 1704153600

# 搜索机器人发送的消息
feishu-cli search messages "告警" \
  --user-access-token u-xxx \
  --from-type bot
```

## 搜索应用

```bash
feishu-cli search apps "应用名称" \
  --user-access-token <token> \
  [--page-size 20] \
  [--page-token <token>]
```

## 搜索云文档

使用 `/open-apis/suite/docs-api/search/object` 端点搜索当前用户可见的云文档。

```bash
feishu-cli search docs "关键词" \
  [--count 20] \
  [--offset 0] \
  [--owner-ids ou_xxx,ou_yyy] \
  [--chat-ids oc_xxx,oc_yyy] \
  [--docs-types doc,sheet,slides]
```

### 文档类型（小写）

| 类型 | 说明 |
|------|------|
| `doc` | 旧版飞书文档 |
| `docx` | 新版飞书文档 |
| `sheet` | 电子表格 |
| `slides` | 幻灯片 |
| `bitable` | 多维表格 |
| `mindnote` | 思维笔记 |
| `file` | 文件 |
| `wiki` | 知识库文档 |
| `shortcut` | 快捷方式 |

### 筛选参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `--count` | int | 返回数量（0-50，默认 20） |
| `--offset` | int | 偏移量（offset + count < 200） |
| `--owner-ids` | string | 文件所有者 Open ID 列表（逗号分隔） |
| `--chat-ids` | string | 文件所在群 ID 列表（逗号分隔） |
| `--docs-types` | string | 文档类型列表（逗号分隔，小写，可选值见上表） |

### 示例

```bash
# 先登录获取 Token（推荐）
feishu-cli auth login

# 基础搜索
feishu-cli search docs "产品需求"

# 搜索特定类型的文档
feishu-cli search docs "季度报告" --docs-types doc,sheet

# 指定返回数量和偏移
feishu-cli search docs "技术方案" --count 10 --offset 0

# 也可以手动指定 Token
feishu-cli search docs "产品需求" --user-access-token u-xxx
```

### 输出格式

搜索结果包含以下信息：
- 标题
- 文档类型
- 文档 Token
- 所有者 ID

### 注意事项

1. **文档类型使用小写**：`doc`、`sheet`、`wiki` 等（与 CLI `--docs-types` 帮助文本一致）
2. **搜索范围**：只能搜索用户有权访问的文档
3. **分页**：使用 `--offset` 和 `--count` 控制（offset + count < 200）

