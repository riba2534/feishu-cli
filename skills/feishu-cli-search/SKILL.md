---
name: feishu-cli-search
description: 搜索消息和应用。当用户请求搜索飞书消息或应用时使用。重要：需要 User Access Token 才能使用。
argument-hint: <messages|apps> <query> [options]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书搜索技能

搜索飞书消息和应用。

## 重要说明：需要 User Access Token

**此功能需要 User Access Token（用户授权令牌），而非应用的 Access Token。**

### 获取 User Access Token 的方式

1. **在飞书开放平台创建应用**并配置重定向 URL
2. **引导用户访问授权页面**进行登录授权
3. **使用授权码换取 User Access Token**

详细文档：[用户授权](https://open.feishu.cn/document/ukTMukTMukTM/ukDNz4SO0MjL5QzM/auth-v3/auth/authorize-user-access-token)

### Token 提供方式

**方式一：命令行参数**
```bash
feishu-cli search messages "关键词" --user-access-token "u-xxx"
```

**方式二：环境变量（推荐）**
```bash
export FEISHU_USER_ACCESS_TOKEN="u-xxx"
feishu-cli search messages "关键词"
```

## 使用方法

```bash
/feishu-search messages "关键词"    # 搜索消息
/feishu-search apps "应用名"        # 搜索应用
```

## CLI 命令详解

### 1. 搜索消息

```bash
# 基本搜索
feishu-cli search messages "会议" --user-access-token <token>

# 使用环境变量
export FEISHU_USER_ACCESS_TOKEN="u-xxx"
feishu-cli search messages "会议"

# 搜索指定会话中的消息
feishu-cli search messages "会议" --chat-ids oc_xxx,oc_yyy

# 搜索图片类型的消息
feishu-cli search messages "图片" --message-type image

# 搜索指定时间范围内的消息
feishu-cli search messages "项目" \
  --start-time 1704067200 \
  --end-time 1704153600

# 搜索群聊消息
feishu-cli search messages "周报" --chat-type group_chat

# 搜索单聊消息
feishu-cli search messages "反馈" --chat-type p2p_chat

# 搜索用户发送的消息（排除机器人）
feishu-cli search messages "通知" --from-type user

# JSON 格式输出
feishu-cli search messages "会议" --output json
```

**参数说明**：
| 参数 | 说明 | 示例值 |
|------|------|--------|
| `--user-access-token` | User Access Token | `u-xxx` |
| `--chat-ids` | 会话 ID 列表（逗号分隔） | `oc_xxx,oc_yyy` |
| `--from-ids` | 发送者用户 ID 列表（逗号分隔） | `ou_xxx,ou_yyy` |
| `--at-chatter-ids` | @的用户 ID 列表（逗号分隔） | `ou_xxx` |
| `--message-type` | 消息类型 | `file`, `image`, `media` |
| `--chat-type` | 会话类型 | `group_chat`, `p2p_chat` |
| `--from-type` | 发送者类型 | `bot`, `user` |
| `--start-time` | 起始时间（Unix 时间戳，秒） | `1704067200` |
| `--end-time` | 结束时间（Unix 时间戳，秒） | `1704153600` |
| `--page-size` | 每页数量（默认 20） | `50` |
| `--page-token` | 分页 token | |
| `--user-id-type` | 用户 ID 类型 | `open_id`, `union_id`, `user_id` |
| `--output, -o` | 输出格式 | `json` |

**消息类型说明**：
| 类型 | 说明 |
|------|------|
| `file` | 文件消息 |
| `image` | 图片消息 |
| `media` | 媒体消息（视频、音频） |

**会话类型说明**：
| 类型 | 说明 |
|------|------|
| `group_chat` | 群聊 |
| `p2p_chat` | 单聊 |

### 2. 搜索应用

```bash
# 基本搜索
feishu-cli search apps "审批" --user-access-token <token>

# 使用环境变量
export FEISHU_USER_ACCESS_TOKEN="u-xxx"
feishu-cli search apps "审批"

# 分页获取更多结果
feishu-cli search apps "审批" --page-size 50

# JSON 格式输出
feishu-cli search apps "审批" --output json
```

**参数说明**：
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--user-access-token` | User Access Token | 无 |
| `--page-size` | 每页数量 | 20 |
| `--page-token` | 分页 token | 无 |
| `--user-id-type` | 用户 ID 类型 | `open_id` |
| `--output, -o` | 输出格式 | 文本 |

## 时间戳转换

搜索消息时使用 Unix 时间戳（秒）。常用转换：

**获取当前时间戳**：
```bash
date +%s
```

**指定时间转时间戳**：
```bash
# macOS
date -j -f "%Y-%m-%d %H:%M:%S" "2024-01-21 00:00:00" +%s

# Linux
date -d "2024-01-21 00:00:00" +%s
```

**时间戳转日期**：
```bash
# macOS
date -r 1704067200

# Linux
date -d @1704067200
```

## 典型工作流

### 搜索近 7 天的消息

```bash
# 计算 7 天前的时间戳
START_TIME=$(date -d "7 days ago" +%s)
END_TIME=$(date +%s)

# 搜索消息
feishu-cli search messages "项目进度" \
  --start-time $START_TIME \
  --end-time $END_TIME
```

### 搜索特定群聊中的文件

```bash
feishu-cli search messages "文档" \
  --chat-ids oc_xxx \
  --message-type file
```

### 搜索并导出结果

```bash
# 搜索并保存为 JSON
feishu-cli search messages "周报" --output json > search_results.json

# 使用 jq 处理结果
feishu-cli search messages "周报" --output json | jq '.items[].content'
```

### 搜索应用并获取详情

```bash
# 搜索应用
feishu-cli search apps "OKR"

# 获取更多结果
feishu-cli search apps "OKR" --page-size 50
```

## 注意事项

1. **Token 必需**：搜索功能必须提供 User Access Token
2. **Token 有效期**：User Access Token 有过期时间，需要及时刷新
3. **时间戳格式**：使用 Unix 时间戳（秒），不是毫秒
4. **权限范围**：只能搜索用户有权限访问的消息和应用
5. **环境变量优先**：推荐使用 `FEISHU_USER_ACCESS_TOKEN` 环境变量，避免在命令中暴露 token

## 常见问题

### Q: 如何获取 User Access Token？
A: 需要通过 OAuth 2.0 授权流程获取，参考[官方文档](https://open.feishu.cn/document/ukTMukTMukTM/ukDNz4SO0MjL5QzM/auth-v3/auth/authorize-user-access-token)。

### Q: 搜索返回空结果？
A: 检查以下几点：
1. Token 是否有效
2. 搜索关键词是否正确
3. 时间范围是否合理
4. 用户是否有权限访问相关消息

### Q: Token 过期怎么办？
A: User Access Token 有效期约 2 小时。使用 Refresh Token 刷新：

```bash
# 通过飞书开放平台 API 刷新 Token
curl -X POST "https://open.feishu.cn/open-apis/authen/v1/oidc/refresh_access_token" \
  -H "Content-Type: application/json" \
  -d '{
    "grant_type": "refresh_token",
    "refresh_token": "<your_refresh_token>",
    "app_id": "<app_id>",
    "app_secret": "<app_secret>"
  }'
```

Refresh Token 有效期 30 天，过期后需要用户重新授权。

## 搜索结果结构

消息搜索返回的每条结果包含：

| 字段 | 说明 |
|------|------|
| `message_id` | 消息唯一 ID |
| `content` | 消息内容 |
| `sender` | 发送者信息（user_id、name） |
| `chat_id` | 所在会话 ID |
| `create_time` | 发送时间（Unix 时间戳） |
| `message_type` | 消息类型（text/image/file 等） |

应用搜索返回的每条结果包含：

| 字段 | 说明 |
|------|------|
| `app_id` | 应用 ID |
| `name` | 应用名称 |
| `description` | 应用描述 |
