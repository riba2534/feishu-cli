---
name: feishu-cli-doctor
description: >-
  feishu-cli 环境健康检查。6 项检查：config 文件 / user_token 有效性 / endpoints 联通 /
  proxy 设置 / 二进制依赖（go/git）/ 配置完整性。
  支持 --offline（跳过网络检查）/--only <check>（只跑指定项）/--json（机器可读）。
  当用户报"feishu-cli 不工作"、"配置出问题"、"突然连不上"、"诊断"、"健康检查"时使用。
argument-hint: [--offline] [--json] [--only config,token,...]
user-invocable: true
allowed-tools: Bash, Read
---

# feishu-cli 健康检查（doctor）

`feishu-cli doctor` 跑一组本地诊断，快速验证 CLI 是否处于可用状态。对齐 lark-cli 的 `doctor` 体验，适合在「突然连不上 / 报错莫名其妙 / 新机器初始化后想一把验」等场景使用。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

---

## 何时使用

| 场景 | 触发命令 |
|------|----------|
| 用户报「feishu-cli 不工作」「突然连不上」 | `feishu-cli doctor` |
| 新机器/新容器初始化后一把验 | `feishu-cli doctor` |
| AI Agent 自检环境是否就绪 | `feishu-cli doctor --json` |
| CI/无外网环境验证本地配置 | `feishu-cli doctor --offline` |
| 只想确认某一项（如 token） | `feishu-cli doctor --only user_token` |
| 排查代理/NO_PROXY 配错 | `feishu-cli doctor --only proxy` |

**不适用**：
- 用户明确说「登录 / 授权 / 拿 token」→ 走 [`/feishu-cli-auth`](../feishu-cli-auth/SKILL.md)
- 用户已经定位到 scope 问题（缺权限）→ 走 `feishu-cli auth check --scope "..."`
- 报具体业务错误码（99991672/1069303 等）→ 各业务模块 skill

---

## 6 项检查

| 检查名 | 验证内容 | 失败原因 | 修复建议 |
|--------|----------|----------|----------|
| `config_file` | `~/.feishu-cli/config.yaml` 或环境变量提供 `app_id` + `app_secret` | 未初始化 / 凭证缺失 | `feishu-cli config create-app --save` 或 `feishu-cli config init` |
| `user_token` | `~/.feishu-cli/token.json` 存在 + access/refresh 状态 | 未登录 / 都过期 / 文件损坏 | `feishu-cli auth login`（搜索/邮件/妙记等命令必需） |
| `endpoint_open` | `https://open.feishu.cn` HTTPS HEAD 可达 | 网络不通 / 代理拦截 / DNS 异常 | 检查网络 / 配 `HTTPS_PROXY` / 加 `NO_PROXY` |
| `endpoint_larksuite` | `https://open.larksuite.com` HTTPS HEAD 可达 | 同上 | 同上 |
| `proxy` | `HTTPS_PROXY` 配置时，`NO_PROXY` 是否包含飞书三域 | `NO_PROXY` 缺 `.feishu.cn` / `.larkoffice.com` / `.larksuite.com` | 把缺失的域加进 `NO_PROXY` 避免代理拦截 |
| `dependencies` | Go runtime 版本 + `larksuite/oapi-sdk-go/v3` 版本 | 不会失败（信息展示） | — |

**网络检查并发执行**，两个 endpoint 同时探测，超时 10s，整体上下文超时 30s。

---

## 命令速查

### 默认 pretty 输出

```bash
feishu-cli doctor
```

输出示例：

```
config_file            ✓  app_id=cli_a7xx****xxx baseURL=https://open.feishu.cn
user_token             ✓  access_token 有效
endpoint_open          ✓  https://open.feishu.cn 可达 (RTT 142ms, status 200)
endpoint_larksuite     ✓  https://open.larksuite.com 可达 (RTT 198ms, status 200)
proxy                  ✓  未设置 HTTP(S)_PROXY
dependencies           ✓  go=go1.22.3 larksuite-sdk=v3.5.3

全部通过 ✓
```

任一项 fail 时，会在结尾 `stderr` 打印 `doctor: 有检查未通过，详见 fail 项的 hint`，并以 exit 1 退出。

### JSON 输出（AI Agent 自检推荐）

```bash
feishu-cli doctor --json
```

输出：

```json
{
  "ok": true,
  "checks": [
    {"name": "config_file", "status": "pass", "message": "app_id=cli_a7xx****xxx baseURL=https://open.feishu.cn"},
    {"name": "user_token", "status": "pass", "message": "access_token 有效"},
    {"name": "endpoint_open", "status": "pass", "message": "https://open.feishu.cn 可达 (RTT 142ms, status 200)"},
    {"name": "endpoint_larksuite", "status": "pass", "message": "https://open.larksuite.com 可达 (RTT 198ms, status 200)"},
    {"name": "proxy", "status": "pass", "message": "未设置 HTTP(S)_PROXY"},
    {"name": "dependencies", "status": "pass", "message": "go=go1.22.3 larksuite-sdk=v3.5.3"}
  ]
}
```

**字段说明**：
- 顶层 `ok`：`true` = 全部通过（含 warn/skip）；`false` = 至少一项 `fail`
- 每项 `status`：`pass` / `fail` / `warn` / `skip`
- `hint`：仅在 `fail` 或 `warn` 时出现，给出修复建议

### 跳过网络检查（`--offline`）

```bash
feishu-cli doctor --offline
```

`endpoint_open` 与 `endpoint_larksuite` 显示为 `skip`，其余正常跑。适用：
- CI 沙箱无外网
- 离线环境调试本地配置
- 只想快速验 token/proxy 不想等网络

### 只跑指定项（`--only`）

```bash
# 只跑 user_token
feishu-cli doctor --only user_token

# 多项用逗号
feishu-cli doctor --only user_token,proxy

# 与 --json 组合
feishu-cli doctor --only endpoint_open --json
```

**有效检查名**（与表格里 `检查名` 列严格对应）：`config_file` / `user_token` / `endpoint_open` / `endpoint_larksuite` / `proxy` / `dependencies`

未在 `--only` 列出的检查不会执行（不会出现在结果里）。

---

## 输出状态语义

| 状态 | 图标（pretty） | 含义 | 影响退出码 |
|------|---------------|------|------------|
| `pass` | `✓` | 通过 | 否 |
| `warn` | `⚠️` | 不影响主流程但建议修复（典型：proxy 缺飞书域、token 未登录但仅可选命令需要） | 否 |
| `fail` | `✗` | 阻断性失败 | 是（exit 1） |
| `skip` | `-` | 跳过（典型：`--offline` / 不在 `--only` 列表里被排除项；当前未在 `--only` 列出的项不会出现，仅 endpoint 在 `--offline` 时显式 skip） | 否 |

**退出码契约**：
- `0` = 全部通过（含 warn/skip）
- `1` = 至少一项 `fail`

AI Agent 写脚本判定环境就绪时，**判 `ok` 字段或 exit code 即可**，无需逐项解析。

---

## 典型排错链路

### 1. 用户报「feishu-cli 不工作」

```bash
feishu-cli doctor
```

按从上到下顺序看第一个 `✗` 项的 `hint`，照着做即可。常见首项失败：
- `config_file ✗` → 没初始化 → `feishu-cli config create-app --save`
- `user_token ✗` → 没登录或都过期 → `feishu-cli auth login`
- `endpoint_open ✗` → 网络/代理问题 → 检查 `HTTPS_PROXY` 与 `NO_PROXY`

### 2. 公司内网代理环境

公司有强制代理时，常见症状是 `endpoint_*` 慢/失败 + `proxy ⚠️`。修复：

```bash
# 把飞书三域加进 NO_PROXY
export NO_PROXY="$NO_PROXY,.feishu.cn,.larkoffice.com,.larksuite.com"

# 再跑一遍
feishu-cli doctor --only proxy,endpoint_open,endpoint_larksuite
```

### 3. CI 流水线自检

```bash
# 跳过网络，只验本地配置（写在 CI 启动步骤）
feishu-cli doctor --offline --json | jq -e '.ok == true'
```

任一项 fail，exit 1 让 CI 直接挂掉，避免后续业务命令报莫名其妙的错。

### 4. AI Agent 跑业务前预检

```bash
# 跑业务前先确认 token 有效 + 网络可达
feishu-cli doctor --only user_token,endpoint_open --json
```

`ok=false` 时根据 `checks[].name` 路由到对应修复 skill：
- `user_token` fail/warn → 走 [`/feishu-cli-auth`](../feishu-cli-auth/SKILL.md) 处理登录
- `endpoint_*` fail → 提示用户检查网络

---

## 与其他 skill 的边界

| 场景 | 用 `doctor` 还是别的？ |
|------|------------------------|
| 整体环境是否就绪 | `doctor` |
| 单独验 token + scope | [`/feishu-cli-auth`](../feishu-cli-auth/SKILL.md) 的 `auth check --scope "..."`（精确到 scope 维度） |
| 登录 / 拿 token | [`/feishu-cli-auth`](../feishu-cli-auth/SKILL.md) |
| 业务命令报错（99991672/1069303 等） | 各业务模块 skill（msg/doc/drive/bitable 等） |
| 新建飞书应用 | [`/feishu-cli-auth`](../feishu-cli-auth/SKILL.md) 的 `config create-app` |

**doctor 的定位**：**一把验环境是否就绪**，不解决任何业务级问题。业务级问题应该已经在 doctor pass 的前提下排查。

---

## 实现细节（开发者参考）

源文件 `cmd/doctor.go`：

- `checkResult` struct 统一表达每项结果（name/status/message/hint）
- 4 个 helper：`checkPass` / `checkFail` / `checkWarn` / `checkSkip`
- `parseOnly` 解析 `--only` 字符串为 `map[string]bool`，空字符串返回 `nil`（表示全跑）
- `shouldRun(name, only)`：`only == nil` 时永远 true；否则查 map
- `checkEndpoints` 用 `sync.WaitGroup` 并发探测两个 endpoint，每个超时 10s
- `outputJSON` / `outputPretty` 两个出口；fail 时返回 error 让 cobra exit 1

单测 `cmd/doctor_test.go` 覆盖：`parseOnly` 5 case / `shouldRun` 边界 / `checkProxy` 三种 env 组合 / `checkDependencies` smoke。

不引入新依赖，全部用标准库 + 已有 `internal/auth` + `internal/config`。

## v1 PR quality-pass 加固

- **`--only` 校验未知 name**：合法 check 名为 `config_file / user_token / endpoint_open / endpoint_larksuite / proxy / dependencies`。typo（如 `--only user_tokn`）会 **报错 + 列出合法清单**，不再 silent pass（避免 CI 静默通过空检查）
- **HTTPS_PROXY userinfo 自动 redact**：`HTTPS_PROXY=https://user:secret123@proxy.example` 在 doctor 输出里会 mask 成 `https://user:***@proxy.example`，避免 doctor 报告被 paste 时泄露 proxy 密码
