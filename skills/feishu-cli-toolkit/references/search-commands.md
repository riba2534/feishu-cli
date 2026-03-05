# 搜索命令详细参考

**重要**：搜索 API 需要 **User Access Token**，不能使用 App Access Token。

## User Access Token 获取方式

1. **命令行参数**：`--user-access-token <token>`
2. **环境变量**：`FEISHU_USER_ACCESS_TOKEN=<token>`

Token 有效期约 2 小时，Refresh Token 有效期 30 天。

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

## 搜索文档和 Wiki

```bash
feishu-cli search docs "关键词" \
  --user-access-token <token> \
  [--doc-types DOC,SHEET,WIKI] \
  [--folder-tokens fldcnxxxxxxxxxxxxxx] \
  [--space-ids space_xxxxxxxxxxxx] \
  [--creator-ids ou_xxx,ou_yyy] \
  [--only-title] \
  [--sort-type EditedTime|CreatedTime|OpenedTime] \
  [--page-size 20] \
  [--page-token <token>]
```

### 文档类型（必须大写）

| 类型 | 说明 |
|------|------|
| `DOC` | 飞书文档 |
| `SHEET` | 电子表格 |
| `BITABLE` | 多维表格 |
| `MINDNOTE` | 思维笔记 |
| `FILE` | 文件 |
| `WIKI` | 知识库 |
| `DOCX` | 新版文档 |
| `FOLDER` | 文件夹 |
| `CATALOG` | 目录 |
| `SLIDES` | 幻灯片 |
| `SHORTCUT` | 快捷方式 |

### 筛选参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `--doc-types` | string | 文档类型列表（逗号分隔，必须大写） |
| `--folder-tokens` | string | 限定文件夹范围（逗号分隔） |
| `--space-ids` | string | 限定 Wiki 空间（逗号分隔） |
| `--creator-ids` | string | 限定创建者（逗号分隔） |
| `--only-title` | flag | 仅搜索标题（不加此参数则搜索全文） |
| `--sort-type` | string | 排序方式：`EditedTime`（最后编辑）/`CreatedTime`（创建时间）/`OpenedTime`（最后打开） |

### 示例

```bash
# 基础搜索
feishu-cli search docs "产品需求" --user-access-token u-xxx

# 搜索特定类型的文档（注意：类型必须大写）
feishu-cli search docs "季度报告" \
  --user-access-token u-xxx \
  --doc-types DOC,SHEET

# 搜索特定文件夹下的文档
feishu-cli search docs "会议纪要" \
  --user-access-token u-xxx \
  --folder-tokens fldcnxxxxxxxxxxxxxx

# 仅搜索标题
feishu-cli search docs "技术方案" \
  --user-access-token u-xxx \
  --only-title

# 搜索 Wiki 空间中的文档
feishu-cli search docs "项目文档" \
  --user-access-token u-xxx \
  --doc-types WIKI \
  --space-ids space_xxxxxxxxxxxx

# 按最后编辑时间排序
feishu-cli search docs "文档" \
  --user-access-token u-xxx \
  --sort-type EditedTime

# 搜索特定创建者的文档
feishu-cli search docs "设计稿" \
  --user-access-token u-xxx \
  --creator-ids ou_xxx,ou_yyy

# 使用环境变量（推荐）
export FEISHU_USER_ACCESS_TOKEN="u-xxx"
feishu-cli search docs "产品需求"
```

### 输出格式

搜索结果包含以下信息：
- 高亮标题
- 文档类型
- 文档 URL
- 所有者名称
- 创建/更新时间
- 摘要（高亮显示匹配内容）

### 注意事项

1. **文档类型必须大写**：`DOC`、`SHEET`、`WIKI` 等，小写会报错
2. **搜索范围**：只能搜索用户有权访问的文档
3. **Wiki 搜索**：搜索 Wiki 时需要同时指定 `--doc-types WIKI` 和 `--space-ids`
4. **分页**：使用 `--page-token` 获取更多结果

