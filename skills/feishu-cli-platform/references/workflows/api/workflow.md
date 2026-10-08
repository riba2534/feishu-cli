# 飞书 OpenAPI 裸调技能

`feishu-cli api` 直接调用任意飞书 OpenAPI 接口，覆盖尚未封装成专用命令的接口，是单工具栈下的兜底能力。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

---

## 用法

```bash
feishu-cli api <METHOD> <path> [flags]
```

- `METHOD`：`GET` | `POST` | `PUT` | `DELETE` | `PATCH`（大小写不敏感）
- `path`：API 路径，如 `/open-apis/im/v1/messages`（前导斜杠可省略）。完整 URL 必须是 `https`，且只支持
  `open.feishu.cn`、`open.larksuite.com`、`open.larkoffice.com` 三类 OpenAPI host；`http://` 或租户文档 URL
  （如 `https://tenant.feishu.cn/...`）会被拒绝，必须手动提取 `/open-apis/...`。

URL 中可以内嵌 query。fragment（`#` 之后）会被丢弃且**不会**进入 query。完整 URL 只接受官方
OpenAPI host：`open.feishu.cn` / `open.larksuite.com` / `open.larkoffice.com`；租户文档 URL
必须先抽出 `/open-apis/...` 短 path。

### Flags

| Flag | 说明 |
|------|------|
| `--params '<json>'` | query 参数（**单个** JSON 对象），如 `'{"page_size":10}'`；尾部有多余内容（如 `'{"a":1} {"b":2}'`）会报错而非静默只取前半 |
| `--data '<json>'` / `--data-file <file>` | 请求体：`--data` 传 JSON 字符串，或 `--data-file` 从文件读（`-` 表示 stdin）；二者互斥 |
| `--as auto\|user\|bot` | 身份：auto（未配置 User 时回退 Tenant；已配置但不可用时失败，默认）/ user（强制 User Token，需先 `auth login`）/ bot（强制 Tenant/应用 Token） |
| `--user-access-token` | 显式 User Token，仅 auto/user 路径解析；`--as bot` 强制应用身份，不用 User Token |
| `--dry-run` | 只打印将发送的请求（method/path/query/body/identity），不实际调用 |
| `-o <file>` | 写原始响应体到文件（binary-safe，适合下载类接口） |
| `--raw` | 原样输出响应 body，不做 pretty JSON |
| `--include-headers` | 在 stderr 打印响应状态码和响应头 |
| `--timeout <seconds>` | 单次请求超时，默认 30 秒 |
| `--format json\|pretty\|table\|ndjson\|csv` | 响应渲染格式（指定后走内置渲染，覆盖默认 pretty；仅适用于 JSON 响应） |
| `--jq '<expr>'` | 用内置 gojq 过滤响应（无需外部 jq；仅适用于 JSON 响应） |
| `--page-all` | 自动翻页：识别 `data.has_more`（容忍 bool/数字/字符串写法）+ `page_token`/`next_page_token`（前者为空自动回落后者）；两者皆空或重复 cursor 停止并报错 |
| `--page-limit` | 配合 `--page-all` 的最大页数（默认 10，`0`=不限） |
| `--page-delay` | 翻页间隔毫秒（默认 200） |

> **`-o` 二进制下载 与 `--format/--jq` 互斥**：默认 / `--raw` / 纯 `-o` 走原样写文件路径（binary-safe）；一旦带上 `--format` 或 `--jq`，响应会先按 JSON 解析再渲染，二进制响应会 decode 失败并报错「响应不是合法 JSON，无法用 --format/--jq 渲染（去掉这两个 flag 可用 --raw 原样输出）」。下载媒体/文件时只用 `-o`，不要叠加 `--format/--jq`。

> 大整数精度：响应用 `UseNumber` 解析，飞书 19 位 `message_id`/`chat_id` 等不会被降级丢精度。

> **业务错误不进 stdout**：响应 `code != 0`（即使随 HTTP 400 下发）或 HTTP 非 2xx 时，stdout 为空、`--jq/--format/-o` 不处理错误体；
> stderr 输出 `飞书业务错误: code=..., msg=...`，并附 `log_id`、所需 scope、字段校验与修复建议。需要原始错误体调试时加 `--raw`（原样写 stdout / `-o`，退出码仍非 0）。
> 退出码：`1` 业务错误，`2` 用法错误，`3` 鉴权/权限（99991672 应用未开通 scope、99991679 用户未授权、token 失效等），`4` 网络错误。

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

# 安全翻页（空/重复 cursor 会停止并报错）
feishu-cli api GET /open-apis/im/v1/chats --page-all --page-limit 10 --as user
```

---

## 何时用专用命令而非 api

`api` 是兜底。高频场景优先用封装好的专用命令（错误处理/参数校验/便捷 flag 更完善）：消息→`msg`、文档→`doc`、多维表格→`bitable`、表格→`sheet`、日历→`calendar` 等。仅当某接口没有对应专用命令时用 `api` 裸调。
