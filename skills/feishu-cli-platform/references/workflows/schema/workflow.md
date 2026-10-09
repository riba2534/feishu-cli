# OpenAPI schema 查询

`feishu-cli schema` 查询 OpenAPI 方法的 path、HTTP 动词、参数、请求/响应体、支持的身份、scope 与文档链接，
无需 Token。它只负责发现接口；执行请求用 api 工作流的 `feishu-cli api`。

典型流程：`schema list` 发现 → `schema <service.resource.method>` 看参数与身份 → 按执行身份预检 → `feishu-cli api` 调用。

## 路径格式：`<service>.<resource>.<method>`

| 段 | 含义 | 示例 |
|----|------|------|
| service | 业务域 | im / drive / calendar / vc / approval / sheets / task / ... |
| resource | 资源（可含 `.`，按最长前缀匹配） | messages / events / chat.members |
| method | 动作 | create / get / list / update / delete / patch |

按路径深度自动分发：

- `schema` 或 `schema list` → 列出所有 service（pretty 含 Catalog 头）
- `schema status` → catalog 来源 / 版本 / service+method 数
- `schema <service>` 或 `schema list --service <service>` → 该 service 下所有 resource.method
- `schema <service>.<resource>` → resource 下的所有 method
- `schema <service>.<resource>.<method>` → method 详情

## 数据来源

编译期内嵌的 catalog 是离线 baseline（12 个 service / 152 个 method）。运行时默认从官方 public
`api_definition?protocol=meta` 拉取 overlay（无凭证、10MB 上限、24h TTL、原子写缓存
`~/.feishu-cli/cache/remote_meta.json`），失败静默回退内嵌版本；`FEISHU_CLI_REMOTE_META=off` 可关闭。
overlay 生效后为 15 个 service / 250 个 method。首次运行会在短预算内同步拉取
（`FEISHU_CLI_META_FIRST_SYNC_MS` 可调，`0` 表示只走后台刷新），之后命中缓存。

判断 overlay 是否生效看 `schema status --format json` 的 `source`：`runtime`（本次刚拉取）/ `cache`（命中缓存）/
`embedded`（未生效，检查网络或 `FEISHU_CLI_REMOTE_META`）。`doctor --only catalog` 与 `auth status -o json`
的 `catalog` 字段同样可读。

## 命令速查

```bash
feishu-cli schema list                                   # 所有 service：name | version | resources 数 | title
feishu-cli schema list --service im                      # im 域全部 resource.method（按 resource 分组）
feishu-cli schema list --service im --format json        # 扁平列表，适合 Agent 二次处理
feishu-cli schema im.chat.members                        # 含点号的 resource（最长前缀匹配）
feishu-cli schema im.messages.delete                     # method 详情
feishu-cli schema im.messages.delete --format json
feishu-cli schema status --format json
```

`schema list --service <s> --format json` 输出 `[{service, resource, method, path, httpMethod, description}]`。

method 详情（pretty）包含：

- HTTP 动词 + 完整 path（如 `DELETE /open-apis/im/v1/messages/{message_id}`）与描述（常含身份约束说明）
- Parameters（`path` / `query` / `required` 标记、类型、描述、示例、枚举值）
- Request Body（有请求体的方法）与 Response Body，含嵌套字段
- `Identity`：`tenant (bot)` / `user`，表示接口接受哪种 Token
- `Scopes`：调用所需权限点（列出多个时通常任一即可，以错误提示为准）
- `Docs`：开放平台文档链接

`--format json` 额外给出 `accessTokens`、`danger`（写/危险操作标记）、`docUrl` 等字段，JSON 不转义 `<` / `>` / `&`。

## schema 查 → api 调

```bash
# 1. 查 path、身份与 scope
feishu-cli schema im.chats.create
# 输出: POST /open-apis/im/v1/chats
#       Identity: tenant (bot)
#       Scopes:   im:chat, im:chat:create, im:chat:create_by_user

# 2. 本例只支持 Bot：确认 App 凭证可用、应用已开通 scope（User 登录不能替代应用授权）
feishu-cli doctor --only bot_identity
feishu-cli auth scopes --scope "im:chat:create" -o json

# 3. 本地预览；用户已授权创建且参数核对后去掉 --dry-run 执行
feishu-cli api POST /open-apis/im/v1/chats \
  --data '{"name":"测试群","description":"by feishu-cli api"}' \
  --as bot --dry-run
```

接口只支持 `user` 时改用 `--as user`，并先 `auth check --scope` 预检本地 User Token。`api` 的参数约束、
输出与错误码见 `../api/workflow.md`。

## 踩坑

1. **路径过深会报错**：`schema im.messages.delete.foo` → `路径过深: ...（多余片段: foo）`，多写一层不会被静默吞掉。
2. **路径不存在分级提示**：未知 service / resource / method 都会列出该层的可用候选名，便于纠正。
3. **resource 含点号用最长前缀匹配**：`im.chat.members.create` 会匹配 resource = `chat.members`、method = `create`。
4. **查询不需要 token**：schema 走本地/缓存 catalog；overlay 是无凭证的 public meta 请求，失败不影响命令。
5. **覆盖范围有限**：catalog 只覆盖开放平台的一小部分。未收录的接口查开放平台文档或
   [OpenAPI Explorer](https://open.feishu.cn/api-explorer)，文档站也没有时按 `../api/references/embedded-api-discovery.md` 调研。

## 何时转其他工作流或技能

| 需求 | 去处 |
|------|------|
| 调用没有专用命令的接口 | 本技能 api 工作流（`feishu-cli api <METHOD> <path>`） |
| 本项目已有对应业务命令 | 对应领域技能（msg / doc / bitable / drive 等专用命令体验更好） |
| 申请 scope / 登录拿 User Token | auth 工作流（`auth check --scope` 预检、`auth login --domain <域> --recommend`） |
| 发消息、写文档、发卡片等具体业务 | `feishu-cli-messaging` / `feishu-cli-docs` 等领域技能 |
