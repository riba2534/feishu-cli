---
name: feishu-cli-profile
description: >-
  feishu-cli 多 App 配置切换。profile add/list/remove/rename/use/current/migrate 管理
  ~/.feishu-cli/profiles/&lt;name&gt;/{config,token}.json 多套 App ID + Token 配置。
  active-profile 指针记录当前激活 profile。向后兼容旧版无 profile 时直接读
  ~/.feishu-cli/{config,token}.json。
  当用户在多个飞书租户 / 多个 App ID 间切换时使用，避免反复重新登录。
argument-hint: add | list | use | current | rename | remove | migrate
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书 CLI 多 App Profile 管理

`feishu-cli profile` 让一台机器在多个飞书账号 / 应用之间快速切换，免去手动备份 / 恢复 `~/.feishu-cli/{config.yaml,token.json}` 的麻烦。常见场景：work / personal 双账号、feishu.cn / larksuite.com 双端、多个 tenant 协同测试。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

---

## 核心概念

### 目录布局

```
~/.feishu-cli/
  config.yaml                # 旧布局：profile 系统未启用时仍读这里（无感升级）
  token.json                 # 旧布局
  active-profile             # 一行文本：当前 profile 名
  previous-profile           # 一行文本：上一个 profile 名（支持 use -）
  profiles/
    work/
      config.yaml            # 该 profile 自己的 app_id / app_secret / base_url
      token.json             # 该 profile 的 User Access Token + Refresh Token
      user_profile.json      # 缓存的当前登录用户信息
    personal/
      ...
```

### Active profile 解析优先级

由 `internal/profile.ActiveDir()` 决定，所有 feishu-cli 命令（auth login / doc import / msg send 等）自动从这里读 config + token，无需任何额外参数：

1. **环境变量 `FEISHU_PROFILE=<name>`**（强制覆盖，不写指针文件；profile 必须已存在，否则报错）
2. **`~/.feishu-cli/active-profile`** 指针文件指向的 profile
3. **指针缺失或指向不存在的 profile** → 回退到 `profiles/` 下字典序第一个
4. **未启用 profile 系统（`profiles/` 不存在）** → 返回旧布局 `~/.feishu-cli/`，老用户零感知

### 向后兼容（零迁移升级）

- 没有任何 profile 时，`internal/config` 和 `internal/auth` 仍走旧路径 `~/.feishu-cli/{config.yaml,token.json}`
- 第一次执行 `profile add` **不会**自动迁移旧文件——避免静默丢数据
- 要把已有 `config.yaml + token.json` 接入 profile 系统，**必须显式** `profile migrate`

---

## 子命令速查

```bash
# 新建
feishu-cli profile add <name> [--app-id ... --app-secret ... --base-url ... --use] [--json]

# 列表（标注 active）
feishu-cli profile list [--json]                                # alias: ls

# 切换 active
feishu-cli profile use <name> [--json]                          # alias: switch / checkout
feishu-cli profile use -                                        # 切回上一个 profile

# 显示当前 active
feishu-cli profile current

# 重命名（自动同步 active / previous 指针）
feishu-cli profile rename <old> <new> [--json]                  # alias: mv

# 删除（默认二次确认）
feishu-cli profile remove <name> [--force] [--json]             # alias: rm / delete

# 把旧布局迁移到 profile 系统（不可逆见下方踩坑）
feishu-cli profile migrate [--name <target>] [--force] [--json]
```

### `profile add`

创建 `~/.feishu-cli/profiles/<name>/` 目录并写入初始 `config.yaml`。**不会**自动迁移旧布局——如需迁移用 `profile migrate`。

| 参数 | 默认 | 说明 |
|------|------|------|
| `--app-id` | `""` | 飞书应用 app_id，可后续手动写 config.yaml |
| `--app-secret` | `""` | 飞书应用 app_secret |
| `--base-url` | `https://open.feishu.cn` | 飞书 OpenAPI base URL（larksuite 用 `https://open.larksuite.com`） |
| `--use` | `false` | 创建后立即切换为 active profile |
| `--json` | `false` | JSON 输出，适合脚本 / AI Agent |

```bash
feishu-cli profile add work --app-id cli_xxx --app-secret xxx --use
feishu-cli profile add personal --base-url https://open.larksuite.com
feishu-cli profile add temp                                     # 留空待手动填
```

### `profile list`

```bash
feishu-cli profile list
# ACTIVE  NAME      CONFIG  TOKEN  PATH
# *       work      yes     yes    /Users/me/.feishu-cli/profiles/work
#         personal  yes     no     /Users/me/.feishu-cli/profiles/personal
```

- `ACTIVE` 列星号 `*` 标当前激活
- `CONFIG / TOKEN` 表示该 profile 是否已有 config.yaml / token.json（用来判断是否已 `auth login`）
- `--json` 返回 `{"active": "work", "profiles": [{"name":..., "path":..., "active":..., "has_config":..., "has_token":...}]}`

### `profile use`

切换 active，把 `~/.feishu-cli/active-profile` 指针写为 `<name>`。

- 切换前的当前 active 会被记入 `previous-profile`，支持 `use -` toggle 切回
- 已经在目标 profile 时打印「无需切换」并返回 0
- profile 不存在时报错 `ErrNotFound`

```bash
feishu-cli profile use personal
feishu-cli profile use -        # 切回上一个
```

### `profile current`

显示当前激活 profile 名 + 目录（制表符分隔）。未启用 profile 系统时输出 `(未启用 profile 系统，使用旧布局 ~/.feishu-cli/)`。

### `profile rename`

把 `profiles/<old>/` 整目录 rename 到 `profiles/<new>/`，自动同步 `active-profile` / `previous-profile` 指针文件。

### `profile remove`

删除 `profiles/<name>/` 整个目录（含 config / token / 用户信息缓存）。

- 默认在 TTY 下二次确认（管道 / 重定向自动跳过）
- `--force` 跳过提示
- 若删的是当前 active，`active-profile` 指针被清空，下次访问回退到字典序第一个 profile

### `profile migrate`

把旧布局 `~/.feishu-cli/{config.yaml,token.json,user_profile.json}` 拷贝到 `profiles/<target>/`，并把 active 指针指向 target。

| 参数 | 默认 | 说明 |
|------|------|------|
| `--name` | `default` | 迁移目标 profile 名 |
| `--force` | `false` | 目标 profile 已存在时覆盖 |
| `--json` | `false` | JSON 输出 |

**原文件不会被删除**——用户自己确认无误后可手动 `rm ~/.feishu-cli/{config.yaml,token.json,user_profile.json}`。

---

## 关键参数：`FEISHU_PROFILE` 环境变量

临时覆盖当前 active profile，**不修改指针文件**，适合 CI / 一次性切换：

```bash
FEISHU_PROFILE=work feishu-cli msg send ...           # 仅本次命令用 work
FEISHU_PROFILE=personal feishu-cli auth status        # 检查 personal 的登录态
```

- 优先级最高（覆盖 active-profile 指针）
- profile 必须已存在，否则报错 `ErrNotFound`
- 名字非法（含 `/` `..` 等）时报错 `ErrInvalidName`

---

## profile 名校验规则

仅允许 `[A-Za-z0-9_-]{1,64}`：

- 禁止 `.` / `..` / `/` 等路径注入字符
- 保留名 `profiles` / `cache` 不可作为 profile 名
- 最大长度 64 字符

非法名直接报 `ErrInvalidName`，所有命令在动手前会先校验。

---

## 典型工作流

### 场景 A：老用户首次启用 profile 系统

```bash
# 已经有 ~/.feishu-cli/config.yaml + token.json
feishu-cli profile migrate                              # → profiles/default/，指针指 default
feishu-cli profile list                                 # 确认 default 已是 active
feishu-cli profile add personal --use --app-id cli_yyy  # 新建 personal 并切过去
feishu-cli auth login                                   # 给 personal 做 OAuth
feishu-cli profile use -                                # 切回 default
```

### 场景 B：从零开始多账号

```bash
feishu-cli profile add work --app-id cli_work --app-secret xxx --use
feishu-cli auth login                                   # work 登录
feishu-cli profile add personal --base-url https://open.larksuite.com
feishu-cli profile use personal
feishu-cli auth login                                   # personal 登录
feishu-cli profile list                                 # 看双账号状态
```

### 场景 C：CI / 临时切换

```bash
FEISHU_PROFILE=ci-bot feishu-cli msg send --chat-id oc_xxx --text "build done"
# 不动 active-profile 指针，下次普通命令仍用之前的 profile
```

---

## 踩坑

### `profile migrate` 不可逆，但原文件不删

- migrate 是**单向拷贝**：旧布局 `config.yaml` / `token.json` / `user_profile.json` 被复制到 `profiles/<target>/`，并写入 active 指针
- 旧文件**不会被删除**，但一旦启用 profile 系统（`profiles/` 目录存在且至少一个子目录），所有命令都走 `ActiveDir()` 解析，不再读旧路径
- 想"回滚"：要么 `profile remove <target>` 删干净所有 profile 让 `profiles/` 空 → 回退到旧布局；要么手动 `cp` 把 profile 目录内容拷回 `~/.feishu-cli/` 顶层
- 建议 migrate 后先验证 `profile current` + 跑一条业务命令（如 `auth status`）确认无误，再考虑 `rm ~/.feishu-cli/{config.yaml,token.json,user_profile.json}`

### `sync.Mutex` 仅进程内并发，不跨进程锁

- `internal/profile.writeMu` 是 Go 进程内 mutex，串行化同一进程内的 Create / Remove / Rename / Use / MigrateLegacy
- **不能**防御并发多个 `feishu-cli` 进程同时改 `active-profile` 指针文件——指针写入用了 `.tmp + rename` 原子替换，但两个进程 race 可能造成"最后一个赢"的不一致
- 同时跑多个 profile 写操作时（如 shell 脚本里背靠背 `profile use a & profile use b`），结果不可预期
- 解法：脚本里串行（顺序执行，不要 `&` 后台并行）；自动化场景一律单进程依次跑

### `profile use` 不会自动登录

切换到一个还没 `auth login` 过的 profile，后续命令会按需提示「未登录飞书」。需要先：

```bash
feishu-cli profile use newone
feishu-cli auth login                                   # 这次的 token 落到 profiles/newone/token.json
```

### `active-profile` 指针指向不存在的 profile

不会报错，会自动回退到字典序第一个 profile。如果想精确控制，请显式 `profile use <name>` 修正指针。

### 删 active profile 后没有 active

`profile remove` 删的是当前 active 时，指针被清空，下次命令会回退到字典序第一个 profile（如果还有的话）。`profile list` 会提示「当前无激活 profile」。

### 旧布局判定：只看 `profiles/` 是否存在子目录

`HasProfiles()` 检查 `~/.feishu-cli/profiles/` 下是否有**至少一个合法名字**的子目录。空目录 / 全是非法名（如 `.tmp`）的子目录会被忽略，仍走旧布局。所以单纯 `mkdir ~/.feishu-cli/profiles/` 不会改变 CLI 行为。

---

## 何时转其他 skill

- **只有一个飞书账号 / 一次性使用** → 不需要 profile 系统，直接 `feishu-cli config create-app --save` + `feishu-cli auth login`，走 [`feishu-cli-auth`](../feishu-cli-auth/SKILL.md)
- **OAuth 登录 / Token 过期 / scope 申请** → [`feishu-cli-auth`](../feishu-cli-auth/SKILL.md)
- **`config.yaml` 字段说明 / `config create-app` 一键创建应用** → [`feishu-cli-auth`](../feishu-cli-auth/SKILL.md) 的「创建飞书应用」节
- **多 profile 之间数据迁移**（如把 work 的 token 复制到 personal） → 直接 `cp ~/.feishu-cli/profiles/work/token.json ~/.feishu-cli/profiles/personal/`，CLI 没提供专门命令

---

## JSON 输出（AI Agent 推荐）

所有写操作（add / use / rename / remove / migrate）和 list 都支持 `--json`，返回结构化输出：

```bash
$ feishu-cli profile add work --app-id cli_xxx --use --json
{"active":true,"dir":"/Users/me/.feishu-cli/profiles/work","name":"work","ok":true}

$ feishu-cli profile list --json
{"active":"work","profiles":[{"name":"work","path":"...","active":true,"has_config":true,"has_token":true}]}

$ feishu-cli profile use personal --json
{"active":"personal","dir":"...","ok":true,"previous":"work"}

$ feishu-cli profile current
work    /Users/me/.feishu-cli/profiles/work
```

AI Agent 优先 `--json` 解析，避免依赖人类可读输出的格式稳定性。
