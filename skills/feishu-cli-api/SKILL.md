---
name: feishu-cli-api
description: >-
  飞书 OpenAPI 裸调。api GET/POST/PUT/DELETE/PATCH <path> 直接调用任意飞书 OpenAPI 接口，
  覆盖 feishu-cli 尚未封装的接口（对齐 lark-cli 的 api 能力）。支持 --params(query)/--data(body 或 @file)/
  --as auto|user|bot 身份/--dry-run 预览/-o 二进制下载/--format/--jq。
  当用户请求"调用 X API"、"裸调飞书接口"、"feishu-cli 没封装的接口怎么调"、"raw api"、
  "用 api 命令发请求"、"下载飞书媒体/文件 binary"时使用。
  不适用：仅查 schema 不调用（用 feishu-cli schema）；已有专用命令的高频场景（用对应 feishu-cli <模块>）。
argument-hint: <METHOD> <path>
user-invocable: true
allowed-tools: Bash(feishu-cli api:*), Bash(feishu-cli schema:*), Read
---

# 飞书 OpenAPI 裸调技能

`feishu-cli api` 直接调用任意飞书 OpenAPI 接口，覆盖尚未封装成专用命令的接口。是单工具栈下替代 `lark-cli api` 的兜底能力。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

---

## 用法

```bash
feishu-cli api <METHOD> <path> [flags]
```

- `METHOD`：`GET` | `POST` | `PUT` | `DELETE` | `PATCH`（大小写不敏感）
- `path`：完整 API 路径，如 `/open-apis/im/v1/messages`（前导斜杠可省略；误传完整 URL 会自动剥掉 host）

### Flags

| Flag | 说明 |
|------|------|
| `--params '<json>'` | query 参数（JSON 对象），如 `'{"page_size":10}'` |
| `--data '<json>'` / `--data @file` | 请求体（JSON 字符串或 `@文件路径`） |
| `--as auto\|user\|bot` | 身份：auto（User 优先 Tenant 兜底，默认）/ user（强制 User Token，需先 `auth login`）/ bot（强制 Tenant/应用 Token） |
| `--user-access-token` | 显式传 User Access Token（`--as user/auto` 时用） |
| `--dry-run` | 只打印将发送的请求（method/path/query/body/identity），不实际调用 |
| `-o <file>` | 写原始响应体到文件（binary-safe，适合下载类接口） |
| `--format json\|pretty\|table\|ndjson\|csv` | 响应渲染格式（默认 json） |
| `--jq '<expr>'` | 用内置 gojq 过滤响应（无需外部 jq） |

> 大整数精度：响应用 `UseNumber` 解析，飞书 19 位 `message_id`/`chat_id` 等不会被降级丢精度。

---

## 三步调研法（不知道 path 时）

```bash
feishu-cli schema <service>                 # 1. 列出该 service 的 resource.method
feishu-cli schema <service>.<resource>.<method>   # 2. 查 path / 参数 / scope
feishu-cli api <METHOD> <path> ...          # 3. 裸调
```

---

## 示例

```bash
# GET + query + jq 过滤
feishu-cli api GET /open-apis/wiki/v2/spaces --params '{"page_size":10}' --jq '.data.items[].name'

# POST 发消息（先 dry-run 预览）
feishu-cli api POST /open-apis/im/v1/messages \
  --params '{"receive_id_type":"chat_id"}' \
  --data '{"receive_id":"oc_xxx","msg_type":"text","content":"{\"text\":\"hi\"}"}' --dry-run

# 请求体从文件读
feishu-cli api POST /open-apis/bitable/v1/apps/xxx/tables --data @body.json

# 下载二进制到文件
feishu-cli api GET /open-apis/drive/v1/medias/<token>/download -o /tmp/file.bin

# 强制用户身份（访问用户私有资源）
feishu-cli api GET /open-apis/calendar/v4/calendars --as user

# 表格输出
feishu-cli api GET /open-apis/wiki/v2/spaces --jq '.data.items' --format table
```

---

## 何时用专用命令而非 api

`api` 是兜底。高频场景优先用封装好的专用命令（错误处理/参数校验/便捷 flag 更完善）：消息→`msg`、文档→`doc`、多维表格→`bitable`、表格→`sheet`、日历→`calendar` 等。仅当某接口没有对应专用命令时用 `api` 裸调。
