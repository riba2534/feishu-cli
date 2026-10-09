# 官方文档站未收录接口的调研方法

开放平台文档站（`open.feishu.cn/document`）并不全：部分接口在文档站检索不到，但飞书官方开源 CLI 工程
[larksuite/cli](https://github.com/larksuite/cli) 的源码已经在调用它们，属于真实可用的接口。需要的能力在本项目
没有专用命令、文档站也查不到时，按下面顺序调研，不要因为"文档站没有"就断定接口不存在。

## 调研步骤

### 第 1 步：查本项目命令与 `feishu-cli schema`

```bash
feishu-cli schema list                         # 列出全部 service
feishu-cli schema list --service drive         # 列出某个 service 的 resource.method
feishu-cli schema drive.metas.batch_query      # 查 path / 参数 / 身份 / scope
```

本地 catalog 内嵌 12 个 service / 152 个 method，运行时 overlay 生效后约 15 个 service / 250 个 method，
只覆盖开放平台的一小部分。**命中** → 直接用现有命令或 `feishu-cli api` 调。

### 第 2 步：查官方文档

在开放平台文档站或 [API Explorer](https://open.feishu.cn/api-explorer) 按接口路径、scope 或中文名称检索。
**命中** → 接口存在且有官方文档，按文档用 `feishu-cli api` 调用。

### 第 3 步：查官方开源 CLI 源码

在 GitHub 上检索 larksuite/cli 的源码（需要已登录的 `gh`），或浅克隆到临时目录后检索：

```bash
# GitHub 代码搜索
gh search code "/open-apis/docs_ai/" --repo larksuite/cli

# 或浅克隆到临时目录后 grep
git clone --depth 1 https://github.com/larksuite/cli /tmp/larksuite-cli
grep -rn "/open-apis/<部分路径>" /tmp/larksuite-cli --include="*.go"
```

**命中** → 接口存在但文档站未收录。读调用处源码确认：
- `Scopes`：所需 scope
- `AuthTypes`：支持 user / bot / 两者
- `dryRun*` 函数：API 路径、body 结构、query 参数
- `execute*` 函数：完整调用细节

### 第 4 步：本地预览后再真实试调

```bash
feishu-cli api <METHOD> <path> --params '<json>' --data '<json>' --as user --dry-run
# 确认 path、body、身份无误，且用户同意后再去掉 --dry-run
```

写接口只在用户授权或自建的测试资源上真实调用。结果判读：
- 成功：stdout 输出响应 JSON，退出码 0。
- 99991679 / 99991672：接口存在但 scope 不足，按 stderr 提示用 `auth scopes` 区分应用未开通与用户未授权。
- HTTP 404：路径错误或接口不存在，回到第 1–3 步核对。
- 99991663 / 99991668：Token 无效或身份不被接口接受，按 auth 工作流排错表处理。

## 优先级

```
schema 命中？ ──是──> 现有命令或 feishu-cli api
   │否
官方文档命中？ ──是──> 按官方文档 + feishu-cli api
   │否
larksuite/cli 源码命中？ ──是──> 文档站未收录接口：按源码 + feishu-cli api
   │否
接口大概率不存在，回报用户
```

## 本项目已封装的此类接口

| 命令 | 对应 API |
|---|---|
| `drive apply-permission` | `POST /open-apis/drive/v1/permissions/{token}/members/apply`（以用户身份申请文档权限） |
| `drive inspect`、`drive update-title`、`markdown` 系列 | `POST /open-apis/drive/v1/metas/batch_query` 等 |
| `doc read --engine docs_ai`、`doc create --content`、`doc content-update`、`doc export --engine docs_ai` | `/open-apis/docs_ai/v1/documents...` |
| `slides create`、`slides add-slide/replace-slide/update-slide`、`slides screenshot` | `/open-apis/slides_ai/v1/...` |

已有专用命令时优先用专用命令；需要它们未暴露的参数时再用 `feishu-cli api` 透传同一端点。
