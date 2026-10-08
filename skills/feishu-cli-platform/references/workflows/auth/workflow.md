# 飞书认证

本项目默认使用 App Token。只有搜索、消息历史/互动、审批任务、会议/妙记、邮箱等用户身份场景需要 User Access Token。

## 目录

- [推荐流程](#推荐流程人工--交互终端)
- [AI Agent 两步授权](#ai-agent-授权两步模式-)
- [授权结果判读](#授权结果判读)
- [常用命令](#常用命令)
- [Token 解析策略](#token-解析策略)
- [业务域登录](#业务域登录)
- [排错](#排错)
- [环境诊断（doctor）](#环境诊断doctor)
- [多 App Profile](#多-app-profileprofile)
- [技能与 CLI 版本](#技能与-cli-版本skills--update)
- [Agent 约定](#agent-约定)

## 推荐流程（人工 / 交互终端）

```bash
# 1. 预检：缺什么 scope 一目了然（auth check 返回 missing 时先到开放平台开通）
feishu-cli auth check --scope "search:docs:read search:message"

# 2a. 按业务域登录，自动带该域推荐 scope
feishu-cli auth login --domain search --recommend

# 2b. 一次授全：对全部业务域申请推荐 scope（单独用 --recommend，不带 --domain）
feishu-cli auth login --recommend

# 2c. 精确控制：显式指定 scope（空格或逗号分隔均可，线上统一转为空格分隔）
feishu-cli auth login --scope "search:docs:read,search:message"

# 2d. 叠加与排除：业务域 + 额外 scope，再剔除不想申请的
feishu-cli auth login --domain docs --scope "drive:file:download" --exclude docx:document:write_only
```

`--recommend` 三种用法：单独用 = 全部业务域的推荐 scope；配 `--domain X` = 仅该域推荐 scope；都不传且在交互终端下 = 弹出选择提示。**非交互环境（无 tty）必须显式指定范围**，否则报用法错误（退出码 2）。

`--scope`、`--domain`、`--recommend` 可以叠加（取并集）；`--exclude`（可重复或逗号分隔）在最终集合上剔除，写错的 scope（不在所选范围内）报用法错误。批量申请（`--domain` / `--recommend`）一律不含 `im:message.send_as_user`（部分租户需管理员审核，会卡住整次授权），确需时用 `--scope` 显式申请。

> 💡 `auth login` 是**增量授权**：多次登录申请的 scope 在飞书服务端累积，补授新 scope 不会丢掉之前已授的。（本地 `token.json` 虽被新 token 覆盖，但其 `scope` 是服务端返回的累积值。）

## AI Agent 授权（两步模式 ⭐）

AI Agent 的 harness 通常**只把最终回复发给用户、且单轮有 timeout**，不适合在同一轮里阻塞等授权。用两步模式：第一步拿链接发给用户并结束本轮，用户授权后下一轮再续轮询。

```bash
# 第一步：只取 device_code + 授权链接，立即返回不轮询
feishu-cli auth login --recommend --no-wait --json
# → 输出一行 device_authorization 事件，把 verification_uri_complete 发给用户后结束本轮

# 第二步（用户回复已授权后）：用 device_code 续上轮询
feishu-cli auth login --device-code <device_code> --json
```

要点：
- **scope 自动恢复**：第二步从 device_code 缓存读回第一步申请的 scope，**不用也不能**再传 `--scope/--domain/--recommend`（重传会报错）。
- **device_code 有效期以服务端返回的 `expires_in` 为准（第一步缺省兜底 240s）**；第二步续轮询本地最多等 600s，由服务端 `expired_token` 决定何时结束。过期需从第一步重来；每次重新跑第一步都会作废上一个链接。
- **不要用短 timeout 反复重试**第一步——每次重启都会让上一个授权链接失效。

若 harness 支持后台任务，也可一步阻塞 + 后台运行：

```bash
# run_in_background 跑：阻塞轮询到授权完成或 device_code 过期（以服务端返回的 expires_in 为准，CLI 兜底 240s），stdout 逐行出 JSON 事件
feishu-cli auth login --recommend --json
```

阻塞模式务必 `run_in_background`，或把单命令 timeout 设到 ≥ 600s。

### JSON 事件 schema

`--json` 模式按 JSONL 逐行输出事件，Agent 解析这两个即可：

**`device_authorization`**（第一步 / 阻塞模式开头）：

```json
{"event":"device_authorization","verification_uri_complete":"https://accounts.feishu.cn/oauth/v1/device/verify?flow_id=...&user_code=XXXX-XXXX","user_code":"XXXX-XXXX","device_code":"...","expires_in":240,"interval":5,"requested_scopes":["..."]}
```

→ 把 `verification_uri_complete`（已含 user_code，可直接点开）**原样**发给用户，不要做任何 URL 编码/改写。

**`authorization_complete`**（授权成功，token 已落盘）：

```json
{"event":"authorization_complete","expires_at":"...","scope":"...","refresh_token_present":true,"granted_scopes":["..."],"missing_scopes":["..."],"requested_scopes":["..."]}
```

→ 上述 6 个字段常驻（`scope` 即落盘 `token.json` 的累积 scope 值；时间字段为带真实时区的 RFC3339）；`refresh_expires_at`（拿到 refresh token 时）、`warnings`/`hints`（refresh 缺失或 scope 未授予时）、`status_message`（服务端附带的提示，如部分 scope 被裁剪）为条件字段，无对应情况时 key 不出现——解析勿假设必存。

## 授权结果判读

收到 `authorization_complete` 即代表**登录成功、token 已写入 `token.json`**——但还要看这几个字段：

| 字段 | 含义 | 处理 |
|---|---|---|
| `refresh_token_present: false` | 没拿到 refresh token | 应用未开通 `offline_access`，开通后 `auth logout && auth login` |
| `missing_scopes` 非空 | 部分申请的 scope 未授予（**warning，不是失败**） | 这些 scope 没在开放平台开通；其余 `granted_scopes` 照常可用 |
| `granted_scopes` | 实际拿到的 scope | 以此为准，可 `auth check` 复核 |

**补授权（增量）**：`missing_scopes` 非空时，先到飞书开放平台开通这些 scope，再照 CLI 的 hint 执行 `auth login --scope "<缺失的>"` 即可。多次 login 的 scope 在服务端累积，补授只需带缺失的那几个，**不会丢掉**之前已授的，无需重跑全量。

## 常用命令

```bash
feishu-cli auth status
feishu-cli auth status -o json --verify
feishu-cli auth check --scope "REQ_SCOPES"
feishu-cli auth scopes --scope "REQ_SCOPES" -o json   # 应用侧是否开通（区分 99991672 / 99991679）
feishu-cli auth login --domain <domain> --recommend
feishu-cli auth refresh
feishu-cli auth logout
feishu-cli auth token --as user|bot|auto    # v1.29+ 导出 token 给 curl/Python 用
feishu-cli auth token --bind-legacy-app --as user  # 把旧版未绑定 app_id 的 token.json 绑到当前应用
feishu-cli config init
printf '%s' "$SECRET" | feishu-cli config init --app-id cli_xxx --app-secret-stdin --probe
feishu-cli config get app_id
feishu-cli config create-app --save
feishu-cli doctor --json
feishu-cli profile current
feishu-cli skills install --dry-run
feishu-cli update --check
```

### `auth token` 导出 Token 给外部工具用（v1.29+）

让 curl / Python requests / 任何 HTTP 工具复用本 CLI 的 Token 全生命周期管理
（Device Flow 登录、2 小时自动刷新、多 profile），不再各自实现 OAuth 流程。

| `--as` | 输出 | 适用场景 |
|---|---|---|
| `user` | User Access Token（`eyJhbGc...`，自动刷新） | 真人身份调 API，含 `auth login` 授权过的 scope |
| `bot` | Tenant Access Token（`t-g10...`，2h 有效；Accounts OAuth v3 `client_credentials`） | App 身份，调 tenant scope API |
| `auto`（默认） | 优先 user，没有再回退 bot | 兼容兜底 |

`--as bot` 与 `--user-access-token` 不能同时出现（会报错，而不是静默覆盖）。`--bind-legacy-app` 也不可与 `--as bot` 或 `--user-access-token` 同时使用。
`auth token --as auto` 仅在从未配置 User Token 时回退 Bot；token.json 损坏、App mismatch、未绑定或刷新失败都会非 0 退出。

旧版 `token.json` 没有 `app_id` 时，**即使 access 仍有效也不能用**，必须先：

```bash
feishu-cli auth token --bind-legacy-app --as user
```

或重新 `auth login`。绑定不会改 token 本身，也不会把另一套 App 的 token 偷偷接到当前应用。

```bash
# 给 curl 用
TOKEN=$(feishu-cli auth token --as user)
curl -H "Authorization: Bearer $TOKEN" \
  https://open.feishu.cn/open-apis/authen/v1/user_info

# 给 Python 用
TOKEN=$(feishu-cli auth token --as bot)
python3 -c "import requests; print(requests.get('https://open.feishu.cn/open-apis/im/v1/chats', headers={'Authorization': f'Bearer $TOKEN'}).json())"
```

> 想直接调任意 OpenAPI 而不写 curl？用 `feishu-cli api <method> <path>`（详见 `feishu-cli-platform` skill）。

### Agent 判读：auth check / auth status 的 JSON 契约

**`auth check --scope "..."`** —— 执行业务前预检某组 scope 够不够。退出码 `0`=满足、`3`=缺失或未登录（鉴权类）；stdout 出 JSON：

| 字段 | 说明 |
|---|---|
| `ok` | `true` 表示所有 required scope 都已授权 |
| `granted` / `missing` | 已有 / 缺失的 scope 列表 |
| `error` | 仅失败时出现：`not_logged_in`（没登录）/ `token_expired`（access + refresh 都失效） |
| `suggestion` | `ok=false` 时给出的修复命令（已拼好 `auth login --scope`） |

```bash
# 满足才往下执行业务命令
feishu-cli auth check --scope "search:docs:read" && feishu-cli search docs --query "..."
```

**`auth status -o json`** —— 看本地 token 现状（默认不连服务端，加 `--verify` 才在线核验）。JSON 字段按字母序输出，已登录时关键字段（按实际输出顺序）：

| 字段 | 说明 |
|---|---|
| `access_token` | 脱敏后的 access token（前 6 + 末 6） |
| `access_token_valid` | access token 是否还在有效期内 |
| `expires_at` | access token 过期时间（RFC3339，带真实时区） |
| `health` | `healthy` / `missing_refresh_token`（没拿到 refresh，对应未开 `offline_access`）/ `needs_relogin`（refresh 也过期） |
| `identity` | `user`（User Token 可用）/ `bot`（仅剩 App Token 可用） |
| `logged_in` | 是否登录 |
| `refresh_expires_at` | refresh token 过期时间（有 refresh 时出现） |
| `refresh_token_present` | 是否拿到 refresh token |
| `refresh_token_valid` | refresh token 是否还在有效期内（有 `refresh_failure` 时恒为 false） |
| `refresh_failure` | 仅当上次刷新被服务端判定 refresh_token 终态失效（20026/20037/20064/20073）时出现：`code`/`error`/`description`/`at`。CLI 不会再自动刷新，需重新 `auth login`（新 token 不带该字段） |
| `scope` | 当前 token 已授权 scope 列表（空格分隔） |
| `token_status` | `valid` / `needs_refresh`（access 过期但 refresh 可用，下次调用自动刷新）/ `expired` |
| `cached_user.open_id` / `.name` | **当前登录的是谁**——需要本人 open_id（发消息给自己、查自己任务）时从这里取（仅当本地有 user cache 时出现） |
| `note` | 健康度提示文案（如 `missing_refresh_token` / `expired` 场景出现） |
| `verified` / `verify_error` | 仅 `--verify`：在线调 `user_info` 核验 token 是否仍被服务端接受。access 过期时与业务命令同样走跨进程加锁刷新，并先校验 token 已绑定当前 App（未绑定的旧 token 直接校验失败，不会被静默绑定）；输出展示刷新后的状态 |
| `profile` / `profile_source` | 当前选中的 profile 及其选择来源（flag/env/pointer/fallback/legacy/none） |
| `app_id` | 叠加 `--bot-app-*` / `FEISHU_APP_*` 后的生效 App ID |
| `token_from_profile` | `token.json` 的候选目录（**不是**本次实际用的 token，见下） |
| `env_overrides` / `flag_overrides` | 当前命令有哪些 App/Profile 覆盖来源 |
| `profile_error` | 当前生效那套的配置/token 读取问题（配置损坏时仍会输出，便于排查） |
| `hint` | App 凭证被覆盖时的说明 |
| `catalog` | OpenAPI catalog：`source`（embedded/cache/runtime）、版本、service/method 数 |

未登录时返回 `{"logged_in": false, "identity": "bot", "note": "..."}`。

> 判断"任务能不能干"优先用 `auth check`（按 scope 精确判定）；`auth status` 看整体健康度和当前身份。下面「排错」表的状态值即来自这两个命令（`auth check` 的 `error` / `auth status` 的 `health`）。

## Token 解析策略

先按任务选择身份，再做与身份一致的预检。完整分类、默认值和失败行为见
[身份选择](references/identity.md)；这里不重复维护命令表。

`auth check --scope` 读取本次 profile 的本地 User Token，不验证 Tenant Token、App 权限，
也不验证 `--user-access-token` / `FEISHU_USER_ACCESS_TOKEN` 的授权范围。
应用侧开通情况用 `auth scopes`：以应用身份读取开放平台已开通的 scope（区分 User / Tenant），
`--scope` 逐项给出 `ok` / `user_not_granted` / `not_logged_in` / `app_not_enabled` / `tenant_only`，
据此区分"应用没开通"（99991672，需开发者后台开通并发布）与"用户没授权"（99991679，需 `auth login --scope`）。
`auth scopes` 只做诊断，退出码恒为 0（接口失败除外）。
Bot-only 任务不能因为没有 User 登录而被阻断；先用 `doctor --only bot_identity` 检查应用身份，
再根据目标 API 的实际结果判断资源权限。`--dry-run` 只验证本地请求，不证明线上权限。

## 业务域登录

`--domain` 可重复或逗号分隔。可选域：`approval apps attendance bitable calendar chat contact doc_access docs drive event im mail markdown minutes okr search sheets slides task vc whiteboard wiki`，或 `all`（运行时 catalog 缓存可能追加个别域，以 `auth login --help` 为准）。`okr` 用于 `okr --as user`，`apps` 为妙搭（spark），`markdown` 为云盘原生 .md 文件。

```bash
feishu-cli auth login --domain search --recommend                # 单域
feishu-cli auth login --domain vc --domain minutes --recommend   # 多域
feishu-cli auth login --recommend                                # 全部域（等价 --domain all --recommend）
```

`--scope` 与 `--domain/--recommend` 叠加；`--exclude` 剔除；最终以 `auth check --scope` 结果为准。

## 排错

| 现象 | 处理 |
|---|---|
| `not_logged_in` | 执行 `auth login --scope "..." --json` |
| `token_expired` 且 refresh 可用 | 业务命令会自动刷新；也可重新登录 |
| `needs_relogin` | refresh token 已过期，重新登录 |
| `missing_refresh_token` | 开通 `offline_access` 后 `auth logout && auth login` |
| `99991663` | access token 无效（通用码）：先确认本次实际身份；User 用 `auth status --verify` 或重新登录，Bot 用 `doctor --only bot_identity` 检查 App 凭证 |
| `99991668` + `not support` | 该接口不支持 User Token（如 OKR `cycle list`），改用 `--as bot` |
| `99991668` + `Invalid access token` | User Access Token 无效或已过期：`auth status --verify` 或重新登录 |
| `99991672` | **应用**未开通所需 scope：重新登录修不好，按错误附带的开放平台链接为应用开通并发布版本（`auth scopes --scope "<scope>"` 可确认） |
| `99991679` | **用户**未授权所需 scope：应用侧已开通后执行 `auth login --scope "<错误提示的 scope>"` 增量补授 |
| 刷新报 `20026/20037/20064/20073`，或 `auth status` 出现 `refresh_failure` | refresh_token 已终态失效（旧格式/过期/吊销/已被使用）。CLI 已在 token.json 记录标记、不再重复刷新；重新 `auth login` |
| 刷新报 `20050` | 刷新服务临时错误，CLI 已自动重试一次；稍后重试 |
| `refresh_token 可能已被服务端消耗` | 刷新请求发出后断网/响应损坏（退出码 4）。网络恢复后重试；若随后报 20073/20064 再重新登录 |
| `invalid_client`（`code=20002` secret 错误 / `20048` 应用不存在） | App 凭证错误（退出码 3）：检查 `--bot-app-*`、`FEISHU_APP_*` 或 config.yaml |
| `解析 token 文件失败 ... 可能已损坏` | `auth login` 重新登录覆盖，或 `auth logout` 清理后再登录 |
| 退出码 `3` / `4` / `10` | `3` 鉴权或权限问题（按上几行处理）；`4` 网络错误（可重试）；`10` 危险操作需确认，获得用户同意后追加 `--yes` |
| `token.json 未绑定 app_id` | 执行 `auth token --bind-legacy-app` 或重新 `auth login`，不要手改 token 文件把别的 App 填进去 |
| `token.json 绑定的 app_id 与当前应用不一致` | 切回匹配的 `--profile` / `--bot-app-id`，或对该应用重新登录 |
| `拒绝自定义远端 host` / `拒绝非 loopback 的 HTTP` | 默认只允许官方 HTTPS。确认不是配错 `base_url` 之后，才设 `FEISHU_ALLOW_CUSTOM_BASE_URL=1`（远端 HTTP 另需 `FEISHU_ALLOW_INSECURE_HTTP=1`） |

## 环境诊断（doctor）

用户报“feishu-cli 不工作”“突然连不上”“配置有问题”时，先用 doctor 缩小问题面：

```bash
feishu-cli doctor
feishu-cli doctor --json
feishu-cli doctor --offline
feishu-cli doctor --only user_token
feishu-cli doctor --only proxy
feishu-cli doctor --only user_token,endpoint_open    # 多值，逗号分隔
```

`doctor` 共 10 项检查；`--only` 支持单个值或逗号分隔多个值（typo 会被本地校验拒收）：

| 检查名 | 含义 |
|---|---|
| `config_file` | 配置文件存在性、字段完整性 |
| `user_token` | `~/.feishu-cli/token.json` 是否存在 / 过期 / refresh 可用 |
| `user_identity` | 用户身份就绪度：token.json 存在且 access_token 有效或可自动刷新 |
| `bot_identity` | 应用身份就绪度：用 app_id/app_secret 在线换取 `tenant_access_token`（需联网；`--offline` 时仅确认凭证已配置并跳过换取） |
| `endpoint_open` | `open.feishu.cn` 可达性 |
| `endpoint_larksuite` | `open.larksuite.com` 可达性（海外站） |
| `proxy` | `HTTPS_PROXY` / `NO_PROXY` 是否会拦截 OpenAPI 域名 |
| `dependencies` | 当前 Go 版本与编译依赖中的 Lark SDK 版本 |
| `catalog` | OpenAPI catalog 来源（embedded/cache/runtime）、版本、service/method 数；`--offline` 时不拉 overlay |
| `skills` | 本地技能目录（`--skills-dir` > `FEISHU_CLI_SKILLS_DIR` > `~/.claude/skills`）与当前 CLI 内嵌技能是否一致；只读本地文件，不联网 |

`user_identity` 与 `bot_identity` 分别回答"用户态命令能否直接跑"和"应用态（`--as bot` / 无人值守）能否直接跑"：
`bot_identity` 通过说明 app_id/app_secret 正确且应用已启用；`user_identity` 为 `warn`（未登录）或 `fail`（过期）时，按提示 `feishu-cli auth login`。

每项状态为 `pass` / `warn` / `fail` / `skip`。整体退出码：全部 `pass`/`skip` 或仅 `warn` → `0`；任一 `fail` → `1`。

`--json` 输出 schema：

```json
{
  "ok": true,
  "checks": [
    {"name": "user_token", "status": "pass", "message": "...", "hint": "..."}
  ]
}
```

CI 用法：`feishu-cli doctor --offline --json | jq -e '.ok == true'`。

常见命中：`proxy` 检查发现 `HTTPS_PROXY` 拦截 → 把 `.feishu.cn,.larkoffice.com,.larksuite.com` 加入 `NO_PROXY`；`HTTPS_PROXY` 中的 userinfo（`user:pass@host`）会被 redact 后再上报，无需担心日志泄漏。

已经明确是 scope 缺失时，不需要 doctor，直接 `auth check --scope "..."`。

> **v1.27.1 新增**：使用 `--config <path>` 显式覆盖配置时，CLI 会在 stderr 打印 warning：`--config` **只换 yaml，token 仍读当前 profile**。完整隔离请用 `--profile <name>` 或 `FEISHU_PROFILE`。此时 `profile list --json` / `profile current --json` 的 `effective`（app_id、base_url、has_secret）反映的是 `--config` 那份文件，而 `profiles[]` 仍是各 profile 目录里的原值，`token_from_profile` 不受影响。

## 多 App Profile（profile）

用户需要在多个飞书租户、多个 App ID 或工作/个人账号之间切换时，用 profile 管理独立的 `config.yaml` 和 `token.json`。

**发现入口（Agent 必跑）**：`feishu-cli profile list --json`。这是 Bot 清单，不是目录清单。即使还没 `profile add`，也会给出 `effective`（环境变量 / 旧布局正在用的那一套），不要把空的 `profiles[]` 理解成「没有 Bot」。

```bash
printf '%s' "$SECRET" | feishu-cli profile add work --app-id cli_xxx --app-secret-stdin --probe --use  # 推荐：secret 不进 shell 历史/ps
feishu-cli profile add work --app-id cli_xxx --app-secret secret_xxx --use
feishu-cli profile list --json
feishu-cli profile current --json
feishu-cli --profile work msg send ...     # 单次指定，不改指针（推荐）
feishu-cli --bot-app-id cli_xxx --bot-app-secret xxx auth status -o json  # 仅本次覆盖 App 凭证
feishu-cli profile use work                # 改默认指针；仅当用户要求「以后都用这个」
feishu-cli profile use -                    # toggle 到上一个 profile（previous-profile 指针）
feishu-cli profile rename old-name new-name
feishu-cli profile remove old-name
feishu-cli profile migrate --name work
```

`profile list --json` 关键字段：

- `mode`: `profile` 或 `legacy`
- `active` / `active_source`: `flag` / `env` / `pointer` / `fallback` / `legacy` / `none`
- `env_overrides.app_id`: `true` 表示 `FEISHU_APP_ID` 会盖住所有 profile 的 app_id
- `flag_overrides.app_id/app_secret`: `true` 表示对应 `--bot-app-*` 正在覆盖 App 凭证
- `token_from_profile`: `token.json` 的**候选目录**；App 凭证覆盖不会改变它
- `user_token_override`: `--user-access-token` / `FEISHU_USER_ACCESS_TOKEN` / 空。非空表示存在压过 `token.json` 的显式 User Token
- `has_config_user_token`: 生效的 `config.yaml` 里是否配了静态 `user_access_token`（解析链最后一级，不输出明文）
- `effective_error` / `profiles[].config_error` / `token_error` / `cache_error`: 各文件的读取问题；单个 profile 损坏不会让整张清单失败
- `hint`: App 凭证与 User Token 来自不同解析链时的显式提醒
- `effective`: 叠加环境变量后真正会拿去调 API 的那一套（含 `app_id`、`token_status`、`select_with`）
- `profiles[]`: 磁盘上每一套；`app_id` 是文件里的值。`select_with` 形如 `--profile alert`

两条解析链彼此正交：

1. **目录 / User Token**：`--profile` > `FEISHU_PROFILE` > `~/.feishu-cli/active-profile` 指针 > 旧布局。指针缺失或失效且没有旧布局时，回退到 `profiles/` 下字典序第一个。
2. **App 凭证**：`--bot-app-id` / `--bot-app-secret` > `FEISHU_APP_ID` / `FEISHU_APP_SECRET` > 上一条选中目录的 `config.yaml`。

**不要把这些字段当成「本次命令实际用了哪个 token」**：本项目按命令分四类 token helper（读类 User 优先 Tenant 兜底、写类默认 Bot、必须 User Token、`--as` 显式切换），`--as bot`、默认 Bot 身份的写命令、只认 flag 的 `vc bot meeting-join`、固定读本地文件的 `auth status` 各走各的路径。CLI 只陈述可离线证明的事实（设置了哪些覆盖、候选目录是谁、文件在不在），实际身份要看目标命令自己的文档与 `--dry-run` 输出。

`--profile` 不会压过 `FEISHU_APP_ID/FEISHU_APP_SECRET`，App 凭证覆盖也不会切换 `token.json`。`hint` 和 `token_from_profile` 任何时候都描述这组正交来源；stderr 只在**真错配**时出声，两种情形：

- 覆盖来的 `app_id` 与所选目录 `config.yaml` 里的不是同一个应用（User Token 仍读该目录的 `token.json`）；
- `app_id` 与 `app_secret` 落在不同层（例如只传了 `--bot-app-secret`），两半凭证可能不属于同一应用。

单应用 `export FEISHU_APP_ID/FEISHU_APP_SECRET`（没有 profile，或与所选 profile 是同一个应用）不会有任何 stderr 输出——不要把「没有警告」理解成「没在用环境变量」，要判断覆盖关系一律读 `profile list --json` 的 `env_overrides` / `effective`。

`--app-secret-stdin` 从 stdin 读第一行（终端下不回显），与 `--app-secret` 互斥；`--probe` 在写盘前换一次
tenant_access_token：服务端明确拒绝（secret 错误、应用不存在）时退出码 3 且不创建 profile，网络/临时错误只告警
（`--json` 输出 `probe: ok|skipped`）。`config init` 同样支持 `--app-id` / `--app-secret-stdin` / `--base-url` / `--probe`。

`config create-app`：注册请求始终在飞书端发起，`--brand lark` 只决定确认页域名；扫码用户属于 Lark 租户时，
CLI 按服务端 `tenant_brand` 自动切换轮询域，`--save` 写入 `base_url: https://open.larksuite.com`，JSON 的 `brand` 为实际品牌。

踩坑（违反直觉、可能丢数据，逐条留意）：

1. **`profile add` 不会自动迁移**旧配置；已有配置接入 profile 系统必须显式运行 `profile migrate`，避免静默改变当前环境。
2. **`profile migrate` 不可逆且不删旧文件**：`~/.feishu-cli/config.yaml` / `token.json` 仍留在原位，需要时手动清理；不要把 migrate 当成"备份+迁移"用。
3. **`profile use <name>` 不会自动登录**：切到一个新建的 profile 后，必须再 `auth login` 才有 User Token。
4. **进程内锁仅 `sync.Mutex`，不跨进程**：并发 `profile use` / `profile rename` 在不同 shell 里同时跑会有 race。
5. **`profile use -` 在没有上一个 profile 时直接报错**：CLI 不会自动回退到字典序首位；先 `profile list` 确认目标，再 `profile use <name>` 显式切换。

profile 名校验规则 `[A-Za-z0-9_-]{1,64}`（禁止 `.` / `..` / `profiles` / `cache` 等保留名），违反时 CLI 自身会报错。`profile rename` 会自动同步 active 与 previous 指针，不需要手动改文件。

`auth logout` 会先调用飞书吊销端点使服务端 token 失效（优先吊销 refresh_token，失败仅告警不阻断），再清理当前 profile 的 token 和用户 profile 缓存。`--no-revoke` 可跳过服务端吊销、只删本地文件（缺 app_id/app_secret 时也会自动跳过吊销）。

## 技能与 CLI 版本（skills / update）

技能内容在编译期内嵌进二进制，与 CLI 版本严格配套。本地技能目录可能落后于 CLI（或反之），
出现“技能里写的命令/参数在当前 CLI 不存在”时，先检查漂移：

```bash
feishu-cli doctor --only skills                      # 只读本地文件，不联网
feishu-cli skills list                               # 内嵌技能、文件数、内容哈希、工作流
feishu-cli skills read feishu-cli-docs               # 输出内嵌 SKILL.md（与当前 CLI 版本一致）
feishu-cli skills read feishu-cli-platform/references/workflows/auth/references/identity.md
feishu-cli skills install --dry-run                  # 预览：新建/更新/无变化/冲突
feishu-cli skills install                            # 写入 ~/.claude/skills 并记录 .feishu-cli-skills.json
feishu-cli skills install --dir /tmp/skills-test     # 指定目录（FEISHU_CLI_SKILLS_DIR 同效）
```

- 技能目录优先级：`--dir` > `FEISHU_CLI_SKILLS_DIR` > `~/.claude/skills`。写入前解析符号链接并打印真实路径；
  `~/.agents/skills` 指向 `~/.claude/skills` 时两者是同一份数据，只需安装一次，不能当作冗余副本删除。
- 只覆盖上次由 `skills install` 写入且未被本地修改的文件；有本地修改，或目录没有安装记录（npx/手动复制）
  且内容不同时，不带 `--force` 整体拒绝。`--force` 只覆盖技能文件，额外文件（笔记、旧 `evals/`）保留。
- 技能目录本身是符号链接（例如指向仓库开发目录）时视为外部管理，任何参数下都不写入、不删除。
- 旧版 29 个技能目录默认只列出；`--prune-legacy` 仅删除 SKILL.md `name` 与旧技能名一致的普通目录。
- 未安装技能的环境可直接用 `skills read` 读取当前版本的工作流，不必先安装。

CLI 自身升级：

```bash
feishu-cli update --check                            # 当前版本 vs GitHub 最新 release
feishu-cli update --dry-run                          # 预览下载地址、校验文件、目标路径与写权限
feishu-cli update                                    # 下载 → sha256 校验 → 新二进制 --version 自检 → 原子替换
```

`update` 只在显式运行时联网；release 缺少 `checksums.txt` 或校验不匹配时拒绝替换；目标目录不可写时给出
`sudo` / `install.sh` / `--target` 建议，不自动提权。更新后运行 `feishu-cli skills install` 同步技能。

## Agent 约定

1. 先根据目标命令选择身份；仅本地 User Token 路径使用 `auth check --scope` 预检，Bot 路径参照 [身份选择](references/identity.md)。
2. 登录优先用**两步模式**（`--no-wait --json` 拿链接 → 用户授权后 `--device-code --json` 续轮询）；能开后台任务时也可 `--json` 阻塞 + `run_in_background`。把 `verification_uri_complete` 原样发给用户，不改写 URL。
3. 授权成功后读 `authorization_complete` 的 `missing_scopes`：非空只是 warning，开通后按需补授（增量授权，补缺失的几个即可，不会丢已授 scope）。
4. 不把 `user_access_token` / `device_code` 写入文档、代码或日志。
5. scope/token 错误按实际身份排查：本地 User 用 `auth check`，Bot 检查应用 scope，显式 User Token 按接口错误排查该 Token；错误不明确（"突然不工作"/网络异常）时用 `doctor` 缩小问题面。
6. 用户可能有多个飞书 Bot、或没指明用哪个时，先 `feishu-cli profile list --json`。看 `effective` 和 `env_overrides`，不要猜。
7. 单次指定用 `feishu-cli --profile <name> <cmd>`（或 `FEISHU_PROFILE=<name>`）。不要为了跑一条业务去 `profile use`——会改全局指针。
8. `--as bot` 只选身份，不选 App。环境变量仍覆盖 profile 的 App 凭证；需要该 profile 原配置时，仅在本次命令进程移除对应覆盖或显式指定 App 凭证，不更改全局环境，也不重复询问已经确定的 App 选择。不要把 `--as bot` 和 `--user-access-token` 一起传。
9. `app_id` 可以出现在回复里；`app_secret` / 裸 token / `device_code` 禁止写入回复或文件。
10. 自定义 `base_url`、明文 HTTP、带 body 的跨源重定向默认拒绝。未得到用户明确授权不要设置 `FEISHU_ALLOW_*` opt-in。
