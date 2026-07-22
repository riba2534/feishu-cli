# 飞书 OpenAPI 裸调技能

`feishu-cli api` 直接调用任意飞书 OpenAPI 接口，覆盖尚未封装成专用命令的接口，是单工具栈下的兜底能力。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

---

## 用法

```bash
feishu-cli api <METHOD> <path> [flags]
```

- `METHOD`：`GET` | `POST` | `PUT` | `DELETE` | `PATCH`（大小写不敏感）
- `path`：API 路径，如 `/open-apis/im/v1/messages`（前导斜杠可省略）。完整 URL 只支持
  `open.feishu.cn`、`open.larksuite.com`、`open.larkoffice.com` 三类 OpenAPI host；租户文档 URL
  （如 `https://tenant.feishu.cn/...`）不会被正确规范化，必须手动提取 `/open-apis/...`。

URL 中可以内嵌 query，但不要携带 `#fragment`：当前实现先解析 query，再处理 fragment，标准的
`?foo=bar#hash` 会把 `#hash` 留在参数值中。

### Flags

| Flag | 说明 |
|------|------|
| `--params '<json>'` | query 参数（JSON 对象），如 `'{"page_size":10}'` |
| `--data '<json>'` / `--data-file <file>` | 请求体：`--data` 传 JSON 字符串，或 `--data-file` 从文件读（`-` 表示 stdin）；二者互斥 |
| `--as auto\|user\|bot` | 身份：auto（User 优先 Tenant 兜底，默认）/ user（强制 User Token，需先 `auth login`）/ bot（强制 Tenant/应用 Token） |
| `--user-access-token` | 显式传 User Access Token（`--as user/auto` 时用） |
| `--dry-run` | 只打印将发送的请求（method/path/query/body/identity），不实际调用 |
| `-o <file>` | 写原始响应体到文件（binary-safe，适合下载类接口） |
| `--raw` | 原样输出响应 body，不做 pretty JSON |
| `--include-headers` | 在 stderr 打印响应状态码和响应头 |
| `--timeout <seconds>` | 单次请求超时，默认 30 秒 |
| `--format json\|pretty\|table\|ndjson\|csv` | 响应渲染格式（指定后走内置渲染，覆盖默认 pretty；仅适用于 JSON 响应） |
| `--jq '<expr>'` | 用内置 gojq 过滤响应（无需外部 jq；仅适用于 JSON 响应） |

> **`-o` 二进制下载 与 `--format/--jq` 互斥**：默认 / `--raw` / 纯 `-o` 走原样写文件路径（binary-safe）；一旦带上 `--format` 或 `--jq`，响应会先按 JSON 解析再渲染，二进制响应会 decode 失败并报错「响应不是合法 JSON，无法用 --format/--jq 渲染（去掉这两个 flag 可用 --raw 原样输出）」。下载媒体/文件时只用 `-o`，不要叠加 `--format/--jq`。

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
feishu-cli api POST /open-apis/bitable/v1/apps/xxx/tables --data-file body.json

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
