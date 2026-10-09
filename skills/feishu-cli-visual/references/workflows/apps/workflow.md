# 妙搭（Miaoda）应用（HTML 秒搭一键部署）

把一份 HTML（单文件或整目录）按官方三段协议发布成妙搭应用，拿到 `release_id`，
再用 `--wait` 或 `apps release get` 确认发布完成并拿到访问链接 `online_url`。

## 前置条件

- **认证**：所有 `apps` 命令都需要 **User Access Token**
- **scope**：`spark:app:write`（create / update / html-publish / access-scope-set）、`spark:app:read`（get / list / release get / release list / html-publish 校验 app_type、access-scope-get）
- **登录**：`auth login` 是增量授权，补授 spark scope 不会丢掉已有授权：

```bash
feishu-cli auth check --scope "spark:app:read spark:app:write"   # 先预检当前是否已有
feishu-cli auth login --scope "spark:app:read spark:app:write"   # 或 auth login --domain apps
```

`apps list --keyword <名称>` 用于按应用名定位 app_id；用户已给出 `app_xxx` 或妙搭链接时直接提取，不要无目的地全量枚举。

## 典型流程（三步部署）

```bash
# 1. 创建一个 HTML 应用，拿 app_id（CLI 已剥掉飞书响应的 data 外层，jq 路径为 .app.app_id）
feishu-cli apps create --name "我的页面" --app-type html
# 2. 打包发布并等待完成：finished 时输出 online_url，failed 时输出 error_logs 并非零退出
feishu-cli apps html-publish --app-id app_xxx --path ./dist --wait
# 3. 设访问范围（online_url 默认仅创建者可见）
feishu-cli apps access-scope-set --app-id app_xxx --scope tenant
```

## 命令速查

### 1. 创建应用 `apps create`

```bash
feishu-cli apps create --name "我的页面" --app-type html
feishu-cli apps create --name "Dashboard" --app-type html --description "数据看板" --icon-url https://...
feishu-cli apps create --name "审批系统" --app-type full_stack --dry-run     # 只看将要发的请求
```

- `--app-type` 取值 `html` / `frontend` / `full_stack`（对齐官方小写枚举）；旧写法 `HTML` 等仍兼容，会归一为小写并在 stderr 提示
- 设置环境变量 `FEISHU_CLI_AGENT_NAME`（兼容 `LARKSUITE_CLI_AGENT_NAME`）时，请求体附带 `source_agent` 标记创建方 Agent
- 返回里取 `.app.app_id` 给后续命令用（CLI 已剥掉飞书响应的 `data` 外层，故不是 `.data.app.app_id`）

### 2. 发布 HTML `apps html-publish`（一键部署）

```bash
feishu-cli apps html-publish --app-id app_xxx --path ./dist          # 目录形态，返回 release_id
feishu-cli apps html-publish --app-id app_xxx --path ./index.html    # 单文件形态
feishu-cli apps html-publish --app-id app_xxx --path ./dist --wait   # 等发布完成，输出 online_url / error_logs
feishu-cli apps html-publish --app-id app_xxx --path ./dist --dry-run        # 看打包清单 + 凭证扫描
feishu-cli apps html-publish --app-id app_xxx --path ./dist --allow-sensitive  # 放行凭证文件
```

- **`--app-id` 必须以 `app_` 开头**：手上是 meta_token 或 `/page/<token>/` 链接时先 `apps get --app-id <meta_token> --jq '.app.app_id'` 换出 app_id
- **跳过 .git**：目录形态自动跳过 `.git` 目录与 `.git` 文件（submodule/worktree 指针），不会把仓库历史发布到公网
- **必须有 index.html**：目录形态根目录下要有 `index.html`；单文件形态文件名必须就是 `index.html`（妙搭以它作为应用入口）
- **app_type**：仅 `html` / `modern_html` 可走本命令；实跑会先 GET 应用校验，其它类型非零退出并给出恢复建议
- **三段协议**（退役单 POST `/upload_and_release_html_code`）：GET `pre_release` 解析 `upload_url`/`tos_path` → 对预签名 URL PUT tar.gz（**不携带飞书 Authorization**）→ POST `/apps/{id}/releases` body `{"tos_path":...}` 返回 `release_id`
- **打包方式**：`--path` 整个打包成单个 in-memory tar.gz；未压缩 ≤ 200MB、打包后 tar.gz ≤ 20MB、单个 `.html` 文件 ≤ 10MB（妙搭服务端硬约束，超限客户端提前拦截并点名文件，`--dry-run` 回填 `oversize_html`）
- **凭证文件防呆**：默认拦截 `.env` / `.env.*` / `.npmrc` / `.netrc` / `.git-credentials` / `.aws/credentials` / `.docker/config.json` / `.kube/config`，命中即非零退出（`--dry-run` 也拦）；确实要发布加 `--allow-sensitive`
- **`--dry-run`**：只展示三段计划 + 打包清单，不获取 token、不访问网络、不上传。缺 `index.html`、单个 `.html` 超 10MB
  等问题不会让 dry-run 失败（仍 exit 0），而是输出 `would_block: true` 与 `block_reasons`——实跑前必须确认 `would_block=false`
- **真实发布前取得用户明确同意**：发布会把内容放到妙搭线上环境；只是验证打包时停在 `--dry-run`
- 不加 `--wait` 时返回里取 `.release_id`，再按下一节查询发布结果
- **`--wait`**：每 20 秒查询一次（`--wait-timeout` 默认 5m），结果见下一节的状态表；输出为 release 详情 + `wait.outcome`
  （`finished` / `failed` / `pending_approval` / `timeout` / `stopped`）。只有 `failed` 非零退出，其余都是 exit 0，
  要读 `wait.outcome` 判断；`timeout` 表示仍在发布，用 `apps release get` 继续查同一 `release_id`，不要重新发布

### 3. 发布结果 `apps release get` / `apps release list`

```bash
feishu-cli apps release get --app-id app_xxx --release-id <release_id>
feishu-cli apps release list --app-id app_xxx --status failed --page-size 5
feishu-cli apps get --app-id app_xxx --jq '.app.is_published'
```

| status | 含义与处理 |
|--------|-----------|
| `finished` | 发布成功；`online_url` 是本次发布的访问链接（未返回时只报告完成，不要编造），默认仅创建者可见 |
| `failed` | 发布失败；`error_logs[]`（`step` / `error_log`）给出失败步骤 |
| `publishing` | 进行中；约每 20 秒查同一个 `release_id`，总计约 5 分钟 |
| `pending` 或 `current_node_info.current_status=PENDING` | 等待服务端配置的审批负责人处理，**不是失败**；停止轮询，审批后再查同一 `release_id`，不要重新发布；`submitted_by` 是申请人不是审批人 |

- `release get` 输出已把 `data.release` 展平，`current_node_info` 的 camelCase 统一为 snake_case（`current_status`、`result.approval_url`、`submitted_by.open_id`）
- `is_published=true`（`apps get` / `apps list`）只说明历史上发布过，不能证明最新内容已部署
- `release list` 在 `has_more=true` 时会在 stderr 提示 `--page-token` 续翻

### 4. 修改应用 `apps update`

```bash
feishu-cli apps update --app-id app_xxx --name "新名字"
feishu-cli apps update --app-id app_xxx --description "更新后的描述"   # --name / --description 至少一个
```

### 5. 访问范围 `apps access-scope-get` / `apps access-scope-set`

```bash
feishu-cli apps access-scope-get --app-id app_xxx

feishu-cli apps access-scope-set --app-id app_xxx --scope tenant        # 组织内可见
feishu-cli apps access-scope-set --app-id app_xxx --scope public --require-login=true   # 互联网公开（require-login 必填）
feishu-cli apps access-scope-set --app-id app_xxx --scope specific \
  --targets '[{"type":"user","id":"ou_xxx"},{"type":"department","id":"od_xxx"},{"type":"chat","id":"oc_xxx"}]'
feishu-cli apps access-scope-set --app-id app_xxx --scope specific \
  --targets '[{"type":"user","id":"ou_xxx"}]' --apply-enabled --approver ou_appr   # 开放申请 + 审批人
```

- `--scope` 三选一：`specific`（部分人员，映射后端 `Range`）/ `public`（互联网公开，映射 `All`）/ `tenant`（组织内，映射 `Tenant`）
- `specific`：必须配 `--targets`（统一格式，发请求时自动拆成后端的 users/departments/chats）
- `public`：必须显式给 `--require-login`（true/false，不能依赖默认）；设为互联网公开前先向用户确认
- `tenant`：不接受其它 flag

## app_id 怎么来

- 自己刚 `apps create` 的：从返回的 `.app.app_id` 取（CLI 已剥掉 `data` 外层）
- 别人/已有的应用：让用户给妙搭应用链接，从 `https://miaoda.feishu.cn/app/app_xxx` 里 `/app/` 后面那段提取，或直接给 `app_xxx` 字符串
- 只知道应用名：`feishu-cli apps list --keyword <名称>`（可加 `--ownership mine|shared`、`--app-type html`；list 只收小写类型值），多个候选时展示名称、app_id、updated_at 让用户确认

## 输出与排错

- 所有命令支持 `--format json|pretty|table|ndjson|csv` + `--jq`；除 `access-scope-get` 外都有 `--dry-run`（同样尊重 `--format/--jq`）
- 用法错误（`--app-type`/`--status`/`--page-size` 取值非法、`html-publish` 与 `release get` 的 `--app-id` 不是 `app_` 开头）exit 2；
  缺 spark scope 报 99991679，exit 3
- 输出已剥掉飞书响应的 `data` 外层：`apps create` 直接是 `{"app":{"app_id":...}}`（jq 用 `.app.app_id`），`apps html-publish` 直接是 `{"release_id":...}`（jq 用 `.release_id`；加 `--wait` 时为 release 详情）
- 业务错误 `code=400002577` / `90002` = 应用不存在或无权访问（核对 app_id）；`code=400000059` = app_type 不支持 html-publish；`code=90001` = 服务端构建失败（用 `--dry-run` 检查打包文件清单）
- TOS PUT 4xx/5xx 会中止、不调用 release-create；5xx 可重试同一条命令换新预签名 URL。不要把飞书 Authorization 带到 TOS。
- scope 不足报错时：`feishu-cli auth check --scope "spark:app:read spark:app:write"` 预检，再按上面「前置条件」并入完整 scope 重新登录
