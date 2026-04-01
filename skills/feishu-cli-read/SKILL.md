---
name: feishu-cli-read
description: >-
  只读操作，不修改文档内容。读取飞书云文档或知识库内容，分析文档结构。支持 docx（新版文档）和 sheet（电子表格）
  两种知识库文档类型。当用户请求"查看"、"阅读"、"分析"、"读取"、
  "打开"、"read"、"view" 飞书文档、知识库或电子表格时使用。支持通过文档 ID、知识库 Token 或 URL 读取。
  Markdown 作为中间格式存储在 /tmp 目录。如需写入请使用 feishu-cli-write。
argument-hint: <document_id|node_token|url>
user-invocable: true
allowed-tools: Bash, Read, Grep
---

# 飞书文档阅读技能

从飞书云文档或知识库读取内容，转换为 Markdown 格式后进行分析和展示。支持 docx（新版文档）和 sheet（电子表格）两种知识库文档类型。

## 前置条件

- **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式
- 已完成认证（`feishu-cli auth login`）
- App 权限：需要 `docx:document` 或 `docx:document:readonly`（普通文档）、`wiki:wiki:readonly`（知识库）
- User Token 权限：若 App 无权访问他人文档，需通过 `feishu-cli auth login --scopes "docx:document:readonly offline_access"` 授权，`doc export` 会自动读取保存的 User Token

## 核心概念

**Markdown 作为中间态**：本地文档与飞书云文档之间通过 Markdown 格式进行转换，中间文件存储在 `/tmp` 目录中。

## 使用方法

```bash
/feishu-read <document_id>
/feishu-read <node_token>
/feishu-read <url>
```

## 执行流程

1. **解析参数**

   - 判断 URL 类型：
     - `/docx/` → 普通文档，使用 `doc export`
     - `/wiki/` → 知识库文档，使用 `wiki export`
   - 如果是 Token，根据格式判断类型

2. **导出为 Markdown（含图片下载）**

   **普通文档**:

   ```bash
   feishu-cli doc export <document_id> --output /tmp/feishu_doc.md --download-images --assets-dir /tmp/feishu_assets
   ```

   `doc export` 会自动解析 User Access Token（如已登录），解析优先级：

   1. `--user-access-token` 命令行参数
   2. `FEISHU_USER_ACCESS_TOKEN` 环境变量
   3. `~/.feishu-cli/token.json`（通过 `auth login` 保存）
   4. `config.yaml` 中的 `user_access_token`

   找到 User Token 时使用用户身份访问，未找到时回退为 App Access Token（租户身份）。

   若遇到 `code=1770032 forBidden`（App 无权限且未登录）或 `code=99991679 Unauthorized`（User Token 缺少 scope），需先完成 User Token 授权：

   ```bash
   feishu-cli auth login --scopes "docx:document:readonly offline_access"
   ```

   **知识库文档**:

   ```bash
   feishu-cli wiki export <node_token> --output /tmp/feishu_wiki.md --download-images --assets-dir /tmp/feishu_assets
   ```

   **重要**：务必使用 `--download-images` 参数下载文档中的图片到本地，否则只能看到 `feishu://media/<token>` 引用，无法理解图片内容。

   **可选参数**：

   - `--user-access-token`：手动指定 User Access Token（不填则自动从 `~/.feishu-cli/token.json` 读取）
   - `--front-matter`：在 Markdown 顶部添加 YAML front matter（含标题和文档 ID）
   - `--highlight`：保留文本颜色和背景色（输出为 HTML `<span>` 标签）
   - `--expand-mentions`：展开 @用户为友好格式（默认开启，需要 contact:user.base:readonly 权限）

3. **读取文本内容**

   - 使用 Read 工具读取导出的 Markdown 文件
   - 分析文档结构和文本内容

4. **读取并理解图片内容**

   - 检查 `--assets-dir` 指定的目录是否有下载的图片
   - **使用 Read 工具逐个读取图片文件**（Claude 支持多模态，可直接理解图片内容）
   - 将图片内容整合到文档分析中

   ```bash
   # 列出下载的图片
   ls /tmp/feishu_assets/

   # 使用 Read 工具查看图片
   # Read /tmp/feishu_assets/image_1.png
   # Read /tmp/feishu_assets/image_2.png
   ```

5. **报告结果**
   - 提供文档摘要（包含图片内容描述）
   - 保留 Markdown 文件和图片供用户进一步操作

## 输出格式

向用户报告：

- 文档标题
- 文档结构概要（标题层级）
- 内容摘要（关键信息）
- 图片内容描述（如有图片）
- Markdown 文件路径（供后续使用）
- 图片文件路径（如有下载）

## 支持的 URL 格式

| URL 格式                                  | 类型     | 命令          |
| ----------------------------------------- | -------- | ------------- |
| `https://xxx.feishu.cn/docx/<id>`         | 普通文档 | `doc export`  |
| `https://xxx.feishu.cn/wiki/<token>`      | 知识库（docx/sheet） | `wiki export` |
| `https://xxx.larkoffice.com/docx/<id>`    | 普通文档 | `doc export`  |
| `https://xxx.larkoffice.com/wiki/<token>` | 知识库（docx/sheet） | `wiki export` |

## 示例

```bash
# 读取普通文档
/feishu-read <document_id>
/feishu-read https://xxx.feishu.cn/docx/<document_id>

# 读取知识库文档
/feishu-read <node_token>
/feishu-read https://xxx.feishu.cn/wiki/<node_token>
```

## 导出格式说明

导出的 Markdown 支持以下飞书特有块类型的转换：

| 飞书块类型         | Markdown 表现                                          |
| ------------------ | ------------------------------------------------------ |
| Callout 高亮块     | `> [!NOTE]`、`> [!WARNING]` 等 6 种 GitHub-style alert |
| 块级/行内公式      | `$formula$`（LaTeX 格式）                              |
| 画板 (Board)       | `[画板/Whiteboard](feishu://board/...)` 链接（见下方「画板读取」） |
| ISV 块 (Mermaid)   | 画板链接                                               |
| QuoteContainer     | `>` 引用语法（支持嵌套）                               |
| AddOns/SyncedBlock | 透明展开子块内容                                       |
| Iframe             | `<iframe>` HTML 标签                                   |

使用 `--highlight` 参数时，带颜色的文本输出为 `<span style="color:...">` 标签。

## 画板读取：优先使用结构化 API

当文档中包含画板（Board/Whiteboard）时，导出的 Markdown 仅显示 `[画板/Whiteboard](feishu://board/<whiteboard_id>)` 链接，无法看到画板内容。

**推荐方式**：使用 `feishu-cli board nodes` 获取完整结构化数据，而非导出 PNG 截图再视觉识别。

```bash
# 从画板链接中提取 whiteboard_id，然后获取结构化数据
feishu-cli board nodes <whiteboard_id>
```

**返回内容包含**：
- 所有形状节点：文本内容、节点类型、填充颜色、坐标位置、尺寸
- 所有连接线：起点/终点节点 ID、连线方向
- 完整的层级和分组关系

**为什么优于 PNG 截图**：
- **零信息丢失**：所有节点文字、分支条件、连线关系完整保留
- **可分类分析**：通过颜色值可识别节点角色（如入口节点、判断节点、过程节点等）
- **可追溯路径**：通过连接线的起点/终点 ID 可还原完整流程路径

**前置条件**：画板所在的文档需要添加文档应用（飞书机器人），并授予 `board:whiteboard` 权限。

### 处理流程

```bash
# 1. 导出文档，发现画板链接
feishu-cli doc export <document_id> --output /tmp/doc.md
# Markdown 中出现: [画板/Whiteboard](feishu://board/<whiteboard_id>)

# 2. 获取画板结构化数据
feishu-cli board nodes <whiteboard_id> > /tmp/board_nodes.json

# 3. 分析节点数据（用 Read 工具读取 JSON）
# 可通过颜色、类型、连线关系理解完整图表结构
```

## 高级：Wiki 目录节点处理

知识库文档可能是**目录节点**（包含子节点），需要特殊处理。

### 1. 识别目录节点

当导出知识库文档时，如果 Markdown 内容显示为：

```markdown
[Wiki 目录 - 使用 'wiki nodes <space_id> --parent <node_token>' 获取子节点列表]
```

说明这是一个**Wiki 目录节点**（block_type=42），子文档列表存储在知识库元数据中。

### 2. 获取子节点列表

```bash
# 1. 先获取节点信息，记录 space_id
feishu-cli wiki get <node_token>

# 2. 列出该节点下的子节点
feishu-cli wiki nodes <space_id> --parent <node_token>
```

### 3. 完整处理流程

```bash
# 步骤 1：尝试导出文档
feishu-cli wiki export <node_token> -o /tmp/doc.md

# 步骤 2：检查内容
# 如果显示 "[Wiki 目录...]"，说明是目录节点

# 步骤 3：获取节点信息
feishu-cli wiki get <node_token>
# 记录 space_id 和 has_child 字段

# 步骤 4：获取子节点
feishu-cli wiki nodes <space_id> --parent <node_token>

# 步骤 5：逐个导出子节点
feishu-cli wiki export <child_node_token_1> -o /tmp/child1.md
feishu-cli wiki export <child_node_token_2> -o /tmp/child2.md
```

## 错误处理与边界情况

### 1. 常见错误

| 错误                              | 原因                                           | 解决                                                                                                        |
| --------------------------------- | ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `code=1770032, msg=forBidden`     | App Token 无权限访问该文档                     | 通过 `auth login --scopes "docx:document:readonly offline_access"` 授权 User Token，`doc export` 会自动读取 |
| `code=99991679, msg=Unauthorized` | User Token 缺少 `docx:document:readonly` scope | 重新执行 `feishu-cli auth login --scopes "docx:document:readonly offline_access"`                           |
| `code=131002, param err`          | 参数错误                                       | 检查 token 格式                                                                                             |
| `code=131001, node not found`     | 节点不存在                                     | 检查 token 是否正确                                                                                         |
| `code=131003, no permission`      | 无权限访问                                     | 确认应用有 wiki:wiki:readonly 权限                                                                          |
| `code=131004, space not found`    | 知识空间不存在                                 | 检查 space_id 是否正确                                                                                      |
| 空内容或 `Unknown block type`     | 特殊块类型                                     | 见「高级：Wiki 目录节点处理」章节                                                                           |

### 2. 边界情况处理

**情况 1：文档内容为空**

- 检查文档是否真的为空
- 检查是否有权限查看内容
- 检查是否是目录节点（见上文）

**情况 2：图片下载失败**

- 检查 `--assets-dir` 目录是否可写
- 检查网络连接
- 图片可能已被删除或过期

**情况 3：部分块类型无法识别**

- 飞书 API 可能返回未知的块类型
- 这些块会显示为 `<!-- Unknown block type: XX -->`
- 这是正常现象，不影响其他内容的读取

**情况 4：大型文档**

- 超过 1000 个块的文档可能需要分页获取
- 使用 `feishu-cli doc blocks <doc_id> --all` 自动分页

### 3. 重试机制

如果遇到网络错误或 API 限流：

```bash
# 添加 --debug 查看详细错误信息
feishu-cli wiki export <token> --debug

# 等待几秒后重试
sleep 5 && feishu-cli wiki export <token>
```

## 注意事项

1. **识别目录节点**：目录节点的内容是子节点列表，不是实际文档内容
2. **公式内容**：导出的 LaTeX 公式保持原文，可直接被 Markdown 渲染器显示
3. **Callout 类型**：支持 NOTE/WARNING/TIP/CAUTION/IMPORTANT/SUCCESS 六种高亮块类型

## 常见问题

**Q: 提示权限不足 / `no permission` / `forBidden`**

- 确认应用已获得 `docx:document:readonly`（普通文档）或 `wiki:wiki:readonly`（知识库）权限
- 如果是他人文档且 App 没有被添加为协作者，需要使用 User Token：
  ```bash
  feishu-cli auth login --scopes "docx:document:readonly offline_access"
  ```
  授权后 `doc export` 会自动读取，无需额外参数

**Q: 文档不存在 / `node not found`**

- 检查文档 ID 或 node_token 是否正确（注意区分 `document_id` 和 `node_token`）
- 从 URL 中提取 ID 时确认使用了正确的路径段（`/docx/` 后为 document_id，`/wiki/` 后为 node_token）

**Q: Token 过期 / 认证失败**

- 运行 `feishu-cli auth status` 检查当前认证状态
- 如已过期，运行 `feishu-cli auth login` 重新认证
- 如使用 App Access Token，检查 `FEISHU_APP_ID` 和 `FEISHU_APP_SECRET` 环境变量是否正确
