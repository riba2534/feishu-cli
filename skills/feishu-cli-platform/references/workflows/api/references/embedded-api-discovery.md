# 飞书埋藏 API 反向工程方法论

> 沉淀于 2026-05-24。背景：实际场景里发现"飞书官方开放平台文档站漏收录某个 API"，
> 但**飞书官方自家的开源工程源码已经在调用它了**。本文教 Agent 系统性发现这种「埋藏 API」。

## 关键事实

**`open.feishu.cn/document/...` 官方文档站并不全**。至少有以下 API 在文档站 0 命中，
但在飞书官方开源工程（本地镜像 `~/airepo/cli`）源码里被正常调用：

| 接口 | 用途 |
|---|---|
| `POST /docs_ai/v1/documents` | **AI 文档创建** |
| `PUT /docs_ai/v1/documents/{id}` | **AI 文档编辑**（str_replace + block 操作） |
| `POST /docs_ai/v1/documents/{id}/fetch` | AI 文档读取（结构化） |
| `POST /slides_ai/v1/xml_presentations/{id}/slide/replace` | 按 XML 替换 slide 页 |
| `POST /drive/v1/permissions/{token}/members/apply` | **以用户身份发起权限申请** |
| `POST /drive/v1/metas/batch_query` | 批量查文档元数据 |

推测原因：这些 API 是飞书内部为 AI Agent 专门开放的"下一代"接口，
飞书故意不在公开文档站宣传（防止滥用 / 还在 GA 化过程中）。

## 调研方法论（4 步法）

### 第 1 步：先查本项目 `feishu-cli schema`

```bash
feishu-cli schema list 2>&1 | grep -i <关键词>
feishu-cli schema <service>.<resource>.<method>
```

本项目内嵌的 `meta_data.json` 含 ~152 个 method（约覆盖飞书开放平台 6%）。
**命中** → 直接用现有命令或 `feishu-cli api` 调。

### 第 2 步：查 `~/airepo/feishu-open-docs`

本地镜像了官方文档站。grep 三路：

```bash
# 按 API 路径
grep -rln "members/apply\|/your/endpoint" feishu-open-docs --include="*.md"

# 按 scope
grep -rln "docs:permission.member:apply" feishu-open-docs --include="*.md"

# 按中文标题
grep -rln "title:.*<关键词>" feishu-open-docs --include="*.md"
```

**命中** → 接口存在 + 有官方文档，直接按文档调用。

### 第 3 步：查飞书官方开源工程源码（埋藏 API 关键步骤）⭐

本地镜像在 `~/airepo/cli`（飞书官方维护的 production 级开源工程，内部按域组织了大量 API 调用代码）：

```bash
# 按用途关键词找调用点
grep -rln "<关键词>" ~/airepo/cli/shortcuts/ --include="*.go"

# 按 API 路径片段找
grep -rn "apiPath\|/open-apis/<部分路径>" ~/airepo/cli/shortcuts/ --include="*.go"

# 找所有 docs_ai / slides_ai 等"埋藏域"
grep -rn "/docs_ai/\|/slides_ai/\|/base_ai/" ~/airepo/cli/ --include="*.go"
```

**命中** → 接口存在但官方文档站未收录。读源码看：
- `Scopes:` 必需的 scope
- `AuthTypes:` 支持 user / bot / 两者
- `dryRun*` 函数 → 看 API 路径 + body 结构 + query 参数
- `execute*` 函数 → 看完整调用细节

### 第 4 步：真实试调验证

```bash
feishu-cli api <METHOD> <path> --params '<json>' --data '<json>' --as user --dry-run
# OK 后去掉 --dry-run 真发
```

返回码：
- `code: 0` → 接口存在且能用
- `code: 99991679` → 接口存在但 scope 不足，按错误信息补 scope 重新登录
- `code: 99991671` / 404 → 接口可能不存在或路径错
- `code: 99991663` → token 失效，重新 auth login

## 这次发现的反思

我（Claude）一开始判断"飞书 OpenAPI 没有用户主动发起权限申请的接口"，理由是 `feishu-open-docs/uUDN04SN0QjL1QDN/uIzNzUjLyczM14iM3MTN/permission-member/` 目录下只有 auth/create/delete/list/transfer_owner/update 六个 .md，**没有 apply.md**。

错在哪：**只看官方文档站，没查官方开源工程源码**。把"文档站没有" 等同于 "接口不存在"。

修正后调研流程：在路径 2 失败后，**必须**走路径 3。官方开源工程源码是飞书官方维护的 production code，调用过的接口都是真实可用的，比文档站可靠。

## 优先级顺序

```
开始
  ├─ schema list 命中？
  │      ├─ 是 → 用 feishu-cli api / 现有命令
  │      └─ 否 ↓
  ├─ feishu-open-docs grep 命中？
  │      ├─ 是 → 按官方文档 + feishu-cli api 调
  │      └─ 否 ↓
  ├─ ~/airepo/cli 源码 grep 命中？  ⭐ 关键一步
  │      ├─ 是 → 「埋藏 API」，按源码 + feishu-cli api 调
  │      └─ 否 ↓
  ├─ 用 web-content-fetcher 查 open.feishu.cn/document 在线（可能比本地镜像新）
  │      └─ 同样按是否命中决定
  └─ 仍找不到 → 接口大概率不存在，回报用户
```

## 如何让 feishu-cli 长期跟上埋藏 API

每月 cron 扫一次官方开源工程仓库新增的 `dryRun*` 函数和 `apiPath := ...` 字面量，
diff 出本项目未覆盖的端点路径，自动起 issue。这是 K2 "埋藏 API 探测器"
路线图的核心内容（见前期路线图 P1 项）。

## 本项目已经移植的埋藏 API

| 命令 | 对应 API | 移植日期 |
|---|---|---|
| `feishu-cli drive apply-permission` | `POST /drive/v1/permissions/{token}/members/apply` | 2026-05-24 |
| `feishu-cli drive inspect` | `POST /drive/v1/metas/batch_query` + `GET /wiki/v2/spaces/get_node` | 2026-05-24 |

待移植（按优先级）：

| 命令 | 对应 API | 工期 |
|---|---|---|
| `feishu-cli doc v2 *` | `docs_ai/v1` 全套 | 3-5 天（大改造） |
| `feishu-cli slides replace-slide` | `slides_ai/v1` 替换 slide | 1 天 |
| `feishu-cli bitable record upload-attachment` 等 | `base/v3` 附件三件套 | 1-2 天 |
