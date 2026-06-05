---
name: feishu-cli-htmlbox
description: >-
  往飞书文档里插入/更新/读取/删除「妙笔BOX」HTML 小组件块——飞书文档里**唯一能跑动画和可交互内容**的载体。
  把一整页 HTML（CSS/JS）塞进块，在 iframe 沙箱里真实执行：CSS @keyframes 动画、ECharts/Three.js 图表、
  Canvas、可拖拽力导向图、Dashboard、打字机/进度条/状态机动画等都能动。
  当用户请求"飞书文档里做动画/能动的图/可交互图表/数据大屏/Dashboard/真实地图/地理飞线/3D 图表（map3D、3D 曲面、Three.js）/在飞书文档里放 ECharts 可视化/批量做一套图表演示"、"妙笔BOX"、"HTML 小组件"、
  "在飞书文档里跑 ECharts/CSS 动画/JavaScript"、"嵌入网页/HTML 到飞书文档"、"飞书文档里的图怎么动起来"时使用。
  注意：要"动"必须用本技能（妙笔BOX）；画板（feishu-cli-board）的 SVG 节点会被服务端栅格化成静态图，不会动。
argument-hint: <document_id> [block_id]
user-invocable: true
allowed-tools: Bash(feishu-cli doc:*), Bash(feishu-cli perm:*), Bash(feishu-cli msg:*), Read, Write
---

# 飞书妙笔BOX（HTML 小组件块）

「妙笔BOX」是飞书文档的 AddOns HTML 小组件块（`block_type=40`）。它把一整页 HTML 存进块的 `add_ons.record`，
飞书在 **iframe 沙箱里真实渲染并执行 JS/CSS**——这是飞书文档里能跑动画、可交互图表的唯一办法。

## 选型：要"动"用妙笔BOX，不要用画板

| 需求 | 用什么 | 为什么 |
|------|--------|--------|
| 文档里要**动画 / 可交互图表 / Dashboard** | **本技能 `doc htmlbox`** | iframe 真跑 CSS/JS，ECharts/Three.js/Canvas/CSS 动画都生效 |
| 静态流程图 / 架构图 / 思维导图（节点可在飞书里单独编辑） | `feishu-cli-board` | 画板原生节点可编辑，但**不会动** |
| 纯文本 / 表格 / 标题等文档内容 | `feishu-cli-write` | 普通文档块 |

⚠ **关键认知**：画板 `board svg-import` 的 SVG（哪怕带 `<animate>` SMIL 动画）会被飞书服务端**栅格化成静态图，不会动**。
要动画只能走妙笔BOX。代价：妙笔BOX 是一个整体 iframe，**内部元素不能像画板节点那样在飞书里单独点选编辑**（鱼与熊掌不可兼得）。

## 前置条件

- **身份**：`create/update/get/delete` 默认 **Bot（App Token）**，操作 `feishu-cli` 自己创建的文档无需登录。
  操作他人或你在飞书里手动建的文档，要传 `--user-access-token` 切到 User 身份。
  ⚠ **同一文档的读写必须用同一身份**，否则会 `1770032 forBidden`（Bot 建的文档用 User 身份去读会被拒）。
- **组件 ID**：默认 `blk_6900429af84180025ce76527`（飞书妙笔BOX 官方组件）。海外 Lark / 其他环境若不同，用 `--component-type-id` 覆盖。

## 快速开始

```bash
# 增：插入一个妙笔BOX 块（HTML 文件 / 字符串 / stdin 三选一）
feishu-cli doc htmlbox create <doc_id> --html-file widget.html
feishu-cli doc htmlbox create <doc_id> --html '<div style="animation:spin 2s linear infinite">…</div>'
cat widget.html | feishu-cli doc htmlbox create <doc_id> --html-file -

# 查：读回块当前 HTML（--raw 出纯 HTML，便于存文件/再编辑）
feishu-cli doc htmlbox get <doc_id> <block_id> --jq '.html_len'
feishu-cli doc htmlbox get <doc_id> <block_id> --raw > current.html

# 改：替换块的 HTML（⚠ block_id 会变，见下）
feishu-cli doc htmlbox update <doc_id> <block_id> --html-file widget-v2.html

# 删：删除妙笔BOX 块
feishu-cli doc htmlbox delete <doc_id> <block_id>
```

典型迭代动画的工作流：`create` → 在飞书里看效果 → 改 HTML → `update`（拿返回的 `new_block_id`）→ 再看，循环到满意。

## 参数

| 命令 | 参数 / Flag | 说明 |
|------|-------------|------|
| 通用 | `--html` / `--html-file`（`-` 读 stdin） | HTML 输入，二选一；原文逐字节存储，`get --raw` 可完整还原 |
| 通用 | `--component-type-id` | 妙笔BOX 组件类型 ID（默认官方值） |
| 通用 | `--format json\|pretty\|table\|ndjson\|csv` / `--jq` / `--dry-run` | 统一输出与预览 |
| create | `--parent-id` / `--index` | 父块（默认文档根）/ 插入位置（-1 末尾） |
| get | `--raw` | 直接输出纯 HTML 到 stdout（否则输出含 html 字段的结构） |

## update = 先建后删，同位置重建（block_id 会变）

飞书 OpenAPI **不支持原地更新** HTML 组件（`PATCH add_ons` 返回 `1770001 invalid param`）。
所以 `update` 通过「在原位置新建 + 删除旧块」（先建后删，避免中途失败丢数据）实现，**新块 block_id 与原来不同**，输出里返回 `new_block_id`。
脚本里 update 后要用 `new_block_id` 继续后续操作，别再用旧 id。

## HTML 编写要点

- **优先自包含**（纯 CSS/Canvas/内联 JS），不依赖外网最稳；需要图表库时用 jsdelivr CDN，实测飞书沙箱能加载 `echarts@5`、`echarts-gl@2`（3D：map3D / surface / bar3D）、`three@0.128`（WebGL）、`gsap` 等。CDN `<script>` 是异步的，用前要轮询等库就绪（`if(typeof echarts==='undefined') return setTimeout(boot,150)`），并给每个 `<script src>` 加 `onerror` 兜底。
- **真实地图**：ECharts 5 不含内置地图数据，直接 `type:'map'` 会空白；要先 `fetch` GeoJSON（借 `echarts@4.9.0/map/json/china.json`）再 `echarts.registerMap`，平面地图 / geo 飞线 / map3D 共用这份注册。
- 一眼能看出"动"：CSS `@keyframes` + `animation` 是最稳的动画；ECharts 力导向图 `layout:force` 持续浮动且可拖拽。
- 避免依赖飞书未必放行的能力（弹窗、外部表单提交、需要用户授权的 API）。
- ⚠ **落库前必须本地浏览器实跑**：iframe 里任何顶层 JS 错误会让**整张图白屏、且不报错给你看**（未捕获异常走 pageerror，不进 console），光读代码看不出来。排查与逐张验证流程见 `references/pitfalls.md`。
- 详细模板与范例（CSS 动画 / ECharts / Canvas 粒子 / Dashboard）见 `references/html-recipes.md`；机制原理、iframe 沙箱限制、与画板的本质区别见 `references/mechanism.md`。

## 创建后交付（按需）

文档若要交给用户：参考 `feishu-cli-write` 的 owner 授权流程（`perm add full_access` + 视配置 `perm transfer-owner`），
并按全局规则发一张飞书卡片通知。

## 常见问题排障

| 症状 | 原因 | 解法 |
|------|------|------|
| `1770032 forBidden` | 文档归属与所用身份不一致（如 Bot 建的文档用 User 身份操作） | 统一身份：Bot 文档别传 `--user-access-token`；User 文档全程传 |
| `get` 报"不是妙笔BOX 块" | 该 block_id 不是 `block_type=40` | 用对 block_id；普通块用 `feishu-cli doc` 其他命令 |
| 块创建了但**空白/不显示**（且 console 也没报错） | iframe 里**顶层 JS 报错**中断了脚本（未捕获异常走 pageerror，不进 console） | 本地浏览器实跑；数 `#chart canvas`（0 = setOption 崩了）；try/catch 复现真实异常后二分定位。详见 `references/pitfalls.md` |
| ECharts/CDN 图不出来 | iframe 未加载到外网资源，或库未就绪就用了 | 轮询等库就绪再 init；加 `onerror` 兜底；或改自包含方案 |
| 真实地图一片空白 | ECharts 5 不带内置地图数据 | `fetch` GeoJSON 后 `registerMap`，见 `references/pitfalls.md` 第 4 节 |
| 按标题找不到某张图 / 批量读 record 匹配不到 | `record` 是双重编码、HTML 的 `<` 存成 `<` | 用 `.add_ons.record｜fromjson｜.html` 解码再匹配，见 pitfalls 第 6 节 |
| 批量追加多图触发 `99991400` | 跨进程的写操作不共享限流器 | 每次 create/append 之间 `sleep 0.5~0.7s`，见 pitfalls 第 8 节 |
| update 后旧链接失效 | update 重建了块，block_id 变了 | 用输出的 `new_block_id` |

## 验证清单

- [ ] `create` 返回 `block_id`，飞书里打开文档能看到块且**在动**
- [ ] `get --raw` 读回的 HTML 与写入一致
- [ ] `update` 后 `get` 新 `new_block_id` 内容已更新、旧 id 报 `not found`
- [ ] `delete` 后 `get` 报 `1770002 not found`

## 参考文档

- `references/html-recipes.md` — HTML 自包含写法、CSS/ECharts/Canvas/Dashboard 范例库、CDN 用法、尺寸性能建议
- `references/mechanism.md` — 妙笔BOX 块机制、`component_type_id` 说明、iframe 沙箱限制、与画板/SVG 的本质区别与 trade-off
- `references/pitfalls.md` — **实战踩坑与排障**（来自真实创建一篇 47 图大文档）：JS 报错=整图白屏且不报错、白屏系统排查法、CDN 加载时序、真实地图 `registerMap`、echarts-gl/three 用法、geo 飞线 / 极坐标着色 / 3D 配置坑、`record` 双重编码定位某图、批量追加多图的标题层级与限流、落库前本地验证流程
