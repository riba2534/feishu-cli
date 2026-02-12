---
name: feishu-cli-wiki
description: 知识库操作。当用户请求查看知识库、列出知识空间、获取知识库节点时使用。Markdown 作为中间格式存储在 /tmp 目录。
argument-hint: <subcommand> [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书知识库操作技能

操作飞书知识库（Wiki），包括获取节点信息、导出文档、列出空间和节点。

## 核心概念

**Markdown 作为中间态**：知识库文档与本地文件之间通过 Markdown 格式进行转换，中间文件存储在 `/tmp` 目录中。

## 使用方法

```bash
/feishu-wiki get <node_token|url>        # 获取节点信息
/feishu-wiki export <node_token|url>     # 导出为 Markdown
/feishu-wiki spaces                       # 列出知识空间
/feishu-wiki nodes <space_id>            # 列出空间下的节点
```

## CLI 命令详解

### 1. 获取知识库节点信息

```bash
# 通过 Token 获取
feishu-cli wiki get <node_token>

# 通过 URL 获取（自动解析）
feishu-cli wiki get https://xxx.feishu.cn/wiki/<node_token>
feishu-cli wiki get https://xxx.larkoffice.com/wiki/<node_token>
```

**输出示例**：
```
知识库节点信息:
  空间 ID:     <space_id>
  节点 Token:  <node_token>
  文档 Token:  <document_token>
  文档类型:    docx
  节点类型:    origin
  标题:        示例文档标题
  有子节点:    true
  创建者:      ou_xxx
  所有者:      ou_xxx
```

### 2. 导出知识库文档为 Markdown

```bash
# 基本导出（输出到 stdout）
feishu-cli wiki export <node_token>

# 导出到文件
feishu-cli wiki export <node_token> --output /tmp/wiki_doc.md

# 通过 URL 导出
feishu-cli wiki export https://xxx.larkoffice.com/wiki/<token> -o doc.md

# 下载图片到本地
feishu-cli wiki export <node_token> -o doc.md --download-images --assets-dir ./images
```

**参数说明**：
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--output, -o` | 输出文件路径 | stdout |
| `--download-images` | 下载图片到本地 | false |
| `--assets-dir` | 图片保存目录 | `./assets` |

### 3. 列出知识空间

```bash
feishu-cli wiki spaces

# 指定每页数量
feishu-cli wiki spaces --page-size 20
```

**参数说明**：
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--page-size` | 每页数量 | 50 |

**输出示例**：
```
知识空间列表:
  1. 空间ID: <space_id>
     名称: 技术文档
     类型: team

  2. 空间ID: 7123456789012345678
     名称: 个人笔记
     类型: personal
```

### 4. 列出空间下的节点

```bash
# 列出根节点
feishu-cli wiki nodes <space_id>

# 列出指定父节点下的子节点
feishu-cli wiki nodes <space_id> --parent <node_token>

# 指定每页数量
feishu-cli wiki nodes <space_id> --page-size 20
```

**参数说明**：
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--parent` | 父节点 Token（不指定则列出根节点） | 根节点 |
| `--page-size` | 每页数量 | 50 |

**输出示例**：
```
节点列表（空间: <space_id>）:
  1. <node_token_1>
     标题: 示例文档 1
     类型: docx

  2. <node_token_2>
     标题: API Reference
     类型: docx
```

## URL 格式支持

| URL 格式 | 说明 |
|---------|------|
| `https://xxx.feishu.cn/wiki/<token>` | 飞书国内版 |
| `https://xxx.larkoffice.com/wiki/<token>` | 飞书国际版/内部版 |
| `https://xxx.larksuite.com/wiki/<token>` | Lark 国际版 |

## 典型工作流

### 导出知识库文档进行分析

```bash
# 1. 获取节点信息
feishu-cli wiki get https://xxx.feishu.cn/wiki/<node_token>

# 2. 导出为 Markdown
feishu-cli wiki export <node_token> -o /tmp/wiki_doc.md

# 3. 分析文档内容
cat /tmp/wiki_doc.md
```

### 批量导出空间文档

```bash
# 1. 列出空间
feishu-cli wiki spaces

# 2. 列出节点
feishu-cli wiki nodes <space_id>

# 3. 逐个导出
feishu-cli wiki export <token1> -o /tmp/doc1.md
feishu-cli wiki export <token2> -o /tmp/doc2.md
```

## 识别目录节点

知识库文档可能是**目录节点**（包含子节点），通过以下方式识别：

### 1. 使用 `wiki get` 查看节点信息

```bash
feishu-cli wiki get <node_token>
```

输出中的 `has_child` 字段表示是否有子节点：
```
知识库节点信息:
  空间 ID:     xxx
  节点 Token:  xxx
  文档 Token:  xxx
  文档类型:    docx
  节点类型:    origin
  标题:        目录节点示例
  有子节点:    true    <-- 表示这是目录节点
```

### 2. 导出时识别目录节点

当导出目录节点时，如果文档内容显示为：
```markdown
[Wiki 目录 - 使用 'wiki nodes <space_id> --parent <node_token>' 获取子节点列表]
```

说明这是一个**Wiki 目录块**（block_type=42），表示该节点是知识库目录，子文档列表存储在知识库元数据中，而非文档内容中。

### 3. 获取子节点列表

对于目录节点，使用以下命令获取子节点：

```bash
# 1. 先获取节点信息，记录 space_id
feishu-cli wiki get <node_token>

# 2. 列出该节点下的子节点
feishu-cli wiki nodes <space_id> --parent <node_token>
```

**示例工作流**：
```bash
# 获取知识库节点
feishu-cli wiki get WvHQwCaDKiJZAjkm354cvZNjnxd
# 输出：有子节点: true, 空间 ID: 7540365701802885139

# 获取子节点列表
feishu-cli wiki nodes 7540365701802885139 --parent WvHQwCaDKiJZAjkm354cvZNjnxd
# 输出：列出所有子节点及其文档类型
```

## 注意事项

1. **知识库 vs 普通文档**：知识库使用 `node_token`，普通文档使用 `document_id`
2. **权限要求**：需要应用具有 `wiki:wiki:readonly` 权限
3. **中间文件**：导出的 Markdown 默认存放在 `/tmp` 目录
4. **目录节点**：目录节点的内容可能为空或只包含 `WikiCatalog` 块（type=42），实际内容在子节点中

## 图片处理（重要）

导出文档时务必下载图片，以便理解文档中的图片内容：

```bash
# 导出并下载图片
feishu-cli wiki export <node_token> \
  --output /tmp/wiki_doc.md \
  --download-images \
  --assets-dir /tmp/wiki_assets

# 查看下载的图片
ls /tmp/wiki_assets/

# 使用 Read 工具读取图片（Claude 支持多模态）
# Read /tmp/wiki_assets/image_1.png
```

**流程**：
1. 导出时使用 `--download-images --assets-dir <dir>` 下载图片
2. 使用 `ls` 查看下载的图片文件
3. 使用 Read 工具逐个读取图片，理解图片内容
4. 将图片内容整合到文档分析中
