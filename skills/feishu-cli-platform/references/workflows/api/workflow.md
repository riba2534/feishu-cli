# 飞书 OpenAPI 透传（api）

`feishu-cli api` 直接调用任意飞书 OpenAPI 端点，覆盖尚未封装成专用命令的接口，复用本地 Token
（含自动刷新）与错误诊断。高频场景优先用专用命令（参数校验、分页、输出更完善）；不知道 path 时先走
schema 工作流查 path / 参数 / 身份 / scope。

## 用法

```bash
feishu-cli api <METHOD> <path> [flags]
```

- `METHOD`：`GET` | `POST` | `PUT` | `DELETE` | `PATCH`（大小写不敏感，其他方法报用法错误）。
- `path`：`/open-apis/...` 短路径，前导斜杠和 `/open-apis/` 前缀都可省略。完整 URL 只接受 `https` 且 host 为
  `open.feishu.cn` / `open.larksuite.com` / `open.larkoffice.com`；`http://` 或租户文档 URL（如
  `https://xxx.feishu.cn/...`）直接报用法错误，必须手动提取 `/open-apis/...`。
- URL 内嵌的 query 会拆进请求参数，同名参数以 `--params` 为准；fragment（`#` 之后）先于 query 被丢弃，
  `?a=1#frag?b=2` 只会留下 `a=1`。

## 身份

| `--as` | 行为 |
|---|---|
| `auto`（默认） | 已配置 User Token 时用 User；从未配置时回退 Tenant（Bot）；已配置但解析/刷新失败直接报错，不静默切 Bot |
| `user` | 强制 User Token，缺失时报错（需先 `auth login`） |
| `bot` | 强制 Tenant Token，不读取任何 User Token |

`--user-access-token` 只在 auto/user 路径生效。身份要与 schema 的 `Identity` 一致：只支持 `tenant (bot)` 的接口用
`--as bot`，只支持 `user` 的接口用 `--as user`。User 身份预检用 `auth check --scope`，Bot 身份看 `auth scopes`
的 `tenant_enabled` 与实际返回（见 [身份选择](../auth/references/identity.md)）。

## 关键约束

完整 flag 见 `feishu-cli api --help`，这里只列容易出错的点：

- `--params` 必须是**单个** JSON 对象（值会转成字符串）；尾部多余内容（如 `'{"a":1} {"b":2}'`）报错而非静默只取前半。
- `--data` 与 `--data-file`（`-` 表示 stdin）互斥，内容必须是合法 JSON，否则发网前报用法错误。
  数字按原始字面量发送，19 位 ID 等大整数不会经 float64 舍入。
- `--data-file` 拒绝读取 `~/.ssh`、`~/.aws`、`~/.feishu-cli`、`/etc` 等敏感目录；`-o` 拒绝写入敏感目录，
  也拒绝含 `..` 段的相对路径（越出当前目录）。均为用法错误（退出码 2）。
- `--dry-run` 不发请求、不刷新 token，只打印 `method`、`path`、`query`、`body`、`supported_tokens`、
  `will_use_user_tok`、`page_all`、`page_limit`；成功不代表服务端会接受。
- 响应 JSON 用 `UseNumber` 解析，`message_id` / `chat_id` 等大整数不丢精度。
- `-o <file>` 原样写响应体（binary-safe、原子写入）。下载媒体/文件时只用 `-o`，不要叠加 `--format/--jq`：
  带上它们后响应会先按 JSON 解析，二进制响应会报"响应不是合法 JSON，无法用 --format/--jq 渲染"。
- `--page-all` 只识别 `data.has_more`（容忍 bool/数字/字符串）+ `page_token`/`next_page_token`；
  `has_more=true` 但游标为空或重复时停止并报错。`--page-limit` 默认 10，`0` 表示不限；`--page-delay` 默认 200ms。
  `-o` 与 `--page-all` 同用时必须带 `--format` 或 `--jq`，否则报用法错误。
- `--timeout` 为单次请求秒数，默认 30。`--include-headers` 把状态码和响应头写 stderr。

## 输出与错误

- 成功：默认 pretty JSON；`--raw` 原样输出；`--format json|pretty|table|ndjson|csv` 与 `--jq` 用内置渲染。
- **业务错误不进 stdout**：响应 `code != 0`（即使随 HTTP 400 下发）或 HTTP 非 2xx 时 stdout 为空，
  `--jq/--format/-o` 不处理错误体；stderr 输出 `飞书业务错误: code=..., msg=...`，并附 `log_id`、所需 scope、
  字段校验（`field_violations`）与修复建议。需要原始错误体调试时加 `--raw`（原样写 stdout / `-o`，退出码仍非 0）。
- 退出码：`0` 成功，`1` 业务错误（含资源不存在、资源级无权限），`2` 用法错误，`3` 鉴权/权限（token 无效、
  99991672 应用未开通 scope、99991679/99991676 用户未授权、App 凭证错误），`4` 网络错误（可重试）。

常见错误码的提示：

| 错误码 | 含义与处理 |
|---|---|
| 99991672 | 应用未开通 scope：重新登录修不好，按提示的开放平台链接开通并发布；User 调用时开通后还需 `auth login --scope` |
| 99991679 / 99991676 | User Token 未授权该 scope：先 `auth scopes --scope` 确认应用侧已开通，再 `auth login --scope "<scope>"` |
| 99991668 | msg 含 `not support` 时接口不收 User Token，改 `--as bot`；否则 User Token 无效，重新登录 |
| 99991661 / 99991663 / 99991677 | Token 缺失、无效或过期；按实际身份排查（见 auth 工作流排错表） |
| 99991400 / 230020 | 限流，降低频率后重试 |
| 230001 | 请求参数无效，对照 `feishu-cli schema` 检查参数名、取值与格式 |
| 230002 / 232011 | Bot 或用户不在该群 |
| 232006 | chat_id 无效 |
| 232025 | App 未启用机器人能力 |
| 232033 | 外部群权限不足：需开启「对外共享能力」的 App 且其 Bot 已入群（见 feishu-cli-messaging 的 chat 工作流） |

## 示例

```bash
# GET + query + jq 过滤
feishu-cli api GET /open-apis/wiki/v2/spaces --params '{"page_size":10}' --jq '.data.items[].name'

# POST 发消息（先 dry-run 预览，用户确认后去掉 --dry-run）
feishu-cli api POST /open-apis/im/v1/messages \
  --params '{"receive_id_type":"chat_id"}' \
  --data '{"receive_id":"oc_xxx","msg_type":"text","content":"{\"text\":\"hi\"}"}' --dry-run

# 请求体从文件读
feishu-cli api POST /open-apis/bitable/v1/apps/xxx/tables --data-file body.json --dry-run

# 下载二进制到文件
feishu-cli api GET /open-apis/drive/v1/medias/<token>/download -o file.bin

# 强制用户身份（访问用户私有资源）
feishu-cli api GET /open-apis/calendar/v4/calendars --as user

# 表格输出
feishu-cli api GET /open-apis/wiki/v2/spaces --jq '.data.items' --format table

# 安全翻页（空/重复 cursor 会停止并报错）
feishu-cli api GET /open-apis/im/v1/chats --page-all --page-limit 10 --as user
```

官方文档站未收录、但官方开源工程在调用的接口，调研方法见 `references/embedded-api-discovery.md`。
