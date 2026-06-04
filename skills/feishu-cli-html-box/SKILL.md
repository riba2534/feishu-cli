---
name: feishu-cli-html-box
description: >-
  把一份自包含的单文件 HTML 嵌入飞书云文档，渲染成可交互的 HTML Box（妙笔网页 / Magic Page）。
  机制：先插一个 block_type=14 + code.style.language=24 的 HTML 代码块，再紧跟插一个
  block_type=40 + add_ons.component_type_id="blk_6900429af84180025ce76527" 的 HTML Box widget；
  渲染源是 widget 自己的 add_ons.record（代码块只是编辑入口，灌完默认删掉只留 iframe）。
  全程用 feishu-cli api 透传实现。当用户说"在飞书文档里嵌 HTML / 跑一个网页 / 放一个可交互页面"、
  "妙笔网页 / Magic Page / HTML Box"、"把这段 HTML 在文档里渲染出来"、"做一个会动的流程图/架构图/Agent 编排动画放进飞书文档"时使用。
  内置 recipe：animated-flowchart（结构化 JSON → 自包含动画 HTML），见 references/animated-flowchart.md。
argument-hint: --html <file> [--title <标题> | --doc-token <token>]
user-invocable: true
allowed-tools: Bash(feishu-cli api:*), Bash(feishu-cli doc:*), Bash(feishu-cli auth:*), Bash(python3:*), Read, Write
---

# 飞书文档 HTML Box 嵌入技能

把一份**自包含的单文件 HTML**（CSS/JS 内联，无构建依赖）嵌入飞书云文档，渲染成沙箱 iframe 里的可交互页面。飞书里叫 **HTML Box / 妙笔网页（Magic Page）**。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

---

## 1. 机制：两个 block 协作，渲染源是 widget 的 record

文档里两个 block 配合完成嵌入：

- **HTML 代码块** —— `block_type: 14`，`code.style.language: 24`（必须是 24=HTML；`15` 是 Dart，填错则不渲染）。存单文件 HTML 源码。
- **HTML Box 小组件** —— `block_type: 40`，`add_ons.component_type_id: "blk_6900429af84180025ce76527"`。沙箱 iframe。

**关键事实**：沙箱 iframe 实际从 widget 自己的 `add_ons.record`（形如 `"{\"html\":\"<!DOCTYPE...\"}"`）读源码渲染，**不是读代码块**。所以：

- 代码块的角色 = **编辑入口**（在文档 UI 里改它，飞书会同步进 widget）。
- 代码块**不是渲染必需**。灌完后可删掉、只留 widget，iframe 照常工作。汇报 / 只读展示场景默认这么做。

---

## 2. 前置条件

```bash
feishu-cli auth login          # HTML Box 嵌入默认用 User 身份（--as user）
feishu-cli auth status         # 确认已登录
```

为什么用 User 身份：新建文档与插入 block 必须是**同一身份**，否则新建的文档归属与插块身份不一致会无编辑权限。脚本统一用 `--as`（默认 `user`）走 `feishu-cli api`，建文档也走 `api`，保证一致。

---

## 3. 标准流程：建 / 灌 / 删源（默认不留源码）

最常见的就是「建一篇文档，灌进 HTML，让用户看见交互页面、不看见源码」。bundled 脚本默认就这么干：

```bash
python3 skills/feishu-cli-html-box/scripts/publish_html_box.py \
  --html my_app.html --title "我的 HTML Box"
# 默认行为：建文档 → 插代码块 → 插 widget（record 预填 HTML）→ 删代码块。返回 doc_url。
```

附加用法：

- 在已有文档里追加 widget：`--doc-token <docx_token>`（跳过建文档步骤）。
- 保留代码块以便在文档 UI 里继续编辑：`--keep-source`。
- 换身份：`--as bot|auto`（默认 `user`）。
- 离线预览将要发出的请求：`--dry-run`（只打印 feishu-cli 命令，不实际调用）。
- 指定二进制路径：`--feishu-cli /path/to/feishu-cli`。

输出 JSON 含 `doc_token` / `doc_url` / `html_box_block_id` / `source_deleted`。

---

## 4. 手动 feishu-cli api 调用（脚本不可用或想看实际请求时）

```bash
# 1) 建文档（用 api 透传，保证与插块同身份）
DOC=$(feishu-cli api POST /open-apis/docx/v1/documents --as user \
  --data '{"title":"我的 HTML Box"}' --jq '.data.document.document_id')

# 2) 插入 HTML 代码块（language=24, wrap=true）—— 记下返回的 block_id
HTML=$(cat my_app.html)
feishu-cli api POST "/open-apis/docx/v1/documents/$DOC/blocks/$DOC/children" --as user \
  --data "$(python3 -c 'import json,sys;print(json.dumps({"children":[{"block_type":14,"code":{"style":{"language":24,"wrap":True},"elements":[{"text_run":{"content":sys.stdin.read()}}]}}],"index":-1}))' < my_app.html)"

# 3) 紧跟其后插入 HTML Box widget，record 直接预填 HTML（不要依赖飞书异步同步）
feishu-cli api POST "/open-apis/docx/v1/documents/$DOC/blocks/$DOC/children" --as user \
  --data "$(python3 -c 'import json,sys;h=sys.stdin.read();print(json.dumps({"children":[{"block_type":40,"add_ons":{"component_id":"","component_type_id":"blk_6900429af84180025ce76527","record":json.dumps({"html":h})}}],"index":-1}))' < my_app.html)"

# 4) （默认）删代码块：record 已在 widget 里，删掉只剩渲染
#    先 GET 顶层 children 找到代码块下标，再 batch_delete
```

顺序铁律：**先代码块、再 widget**；删代码块要在 widget 插入成功、`record` 确认含 HTML 之后。

---

## 5. 替换已有文档里的 HTML

### 5.1 文档里还留着源代码块（用了 `--keep-source`）

PATCH 那个代码块即可，widget 的 record 会被飞书自动同步：

```bash
BLOCK=$(feishu-cli api GET "/open-apis/docx/v1/documents/$DOC/blocks" --as user \
  --params '{"page_size":500}' \
  --jq '[.data.items[] | select(.block_type==14 and .code.style.language==24)][0].block_id')
feishu-cli api PATCH "/open-apis/docx/v1/documents/$DOC/blocks/$BLOCK" --as user \
  --data "$(python3 -c 'import json,sys;print(json.dumps({"update_code":{"style":{"language":24,"wrap":True},"elements":[{"text_run":{"content":sys.stdin.read()}}]}}))' < my_app.html)"
```

### 5.2 默认情况：只剩 widget、源代码块已删

直接 PATCH widget 的 `add_ons.record`（注意 record 是**字符串化的 JSON**）：

```bash
BOX=$(feishu-cli api GET "/open-apis/docx/v1/documents/$DOC/blocks" --as user \
  --params '{"page_size":500}' \
  --jq '[.data.items[] | select(.block_type==40 and .add_ons.component_type_id=="blk_6900429af84180025ce76527")][0].block_id')
feishu-cli api PATCH "/open-apis/docx/v1/documents/$DOC/blocks/$BOX" --as user \
  --data "$(python3 -c 'import json,sys;h=sys.stdin.read();print(json.dumps({"update_add_ons":{"component_type_id":"blk_6900429af84180025ce76527","record":json.dumps({"html":h})}}))' < my_app.html)"
```

---

## 6. 写 HTML 时的铁律

1. **单文件**：CSS/JS 内联到一份 HTML，外部依赖只能走公网 CDN。
2. **代码块 language 必须填 24**：不填、填默认值、或填 15（Dart）都会导致 widget 拿不到 HTML、页面空白。
3. **固定浅色主题**：飞书文档通常是浅色，别依赖 `prefers-color-scheme: dark`，否则会出现意外的黑色卡片。
4. **不要内部竖向滚动条**：`html, body { overflow: hidden; }`，卡片高度紧凑；iframe 内滚动会干扰文档滚动。
5. **响应式、别锁死小宽度**：用 `max-width: min(1180px, calc(100vw - padding))` 之类，让飞书全屏打开时能放大，而不是卡在小卡片宽度。
6. **登录态/私密数据不要写进 HTML**：开源/分享文档里别内联任何 token、密钥、个人信息。

---

## 7. Recipes

| recipe | 说明 | 文档 |
|--------|------|------|
| **animated-flowchart** | 结构化 JSON（nodes/edges/timeline）→ 自包含动画 HTML（SVG 拓扑 + 自动播放时间线），再嵌入飞书文档 | [references/animated-flowchart.md](references/animated-flowchart.md) |

更多自包含 HTML 来源（图表、看板、报告页等）都可用本技能第 3 节的 `publish_html_box.py` 嵌入，recipe 只负责「怎么生成那份 HTML」。

---

## 8. 实测的坑

- **language 写错（最常见）**：`code.style.language` 不显式填 24，飞书不会把 HTML 同步进 widget 的 record，iframe 空白。
- **顺序错**：必须「建文档 → POST 代码块 → POST widget → 视情况删代码块」。先 POST widget 再 POST 代码块，record 不回填。
- **删早了**：删代码块要在 widget 创建响应里确认 `record` 已含 HTML 之后；record 还是 `{}` 时删源码块会留下空白盒子。脚本直接预填 record 规避了这一点。
- **record 是字符串**：HTML Box 的 `add_ons.record` 值是字符串 `"{\"html\":\"...\"}"`，不是对象 `{}`；创建时省略或写成对象会 400。
- **block_id 路径**：`POST .../blocks/{doc_token}/children` 第二段是 doc_token（顶层 page），不是某个具体 block。
- **身份不一致**：建文档与插块用不同身份会导致无编辑权限；统一用 `--as`（脚本已处理）。

---

## 9. 文件

- `scripts/publish_html_box.py`：通用发布脚本（feishu-cli api 后端，建/灌/删源）。
- `scripts/animate_diagram.py`：animated-flowchart recipe 的生成器（pattern JSON → 自包含动画 HTML）。
- `references/animated-flowchart.md`：animated-flowchart recipe 工作流。
- `references/pattern-schema.md`：animated-flowchart 输入 JSON schema。
- `examples/supervisor.json`：示例 pattern（多 Agent 编排动画）。
