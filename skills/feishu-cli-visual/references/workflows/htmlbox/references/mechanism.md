# 妙笔BOX 机制速查

画图直接看 `gallery.md` / `geo-3d.md`；**只在排查 token / 组件 ID / update 行为时**才需要这里。

## 是什么

妙笔BOX = 飞书文档的 AddOns HTML 小组件块（`block_type=40`）。整页 HTML 存进 `add_ons.record`（一个 JSON 字符串 `{"html":"..."}`），飞书在 **iframe 沙箱里真实执行**——这就是它能跑 CSS/JS 动画、ECharts、Canvas 的根本原因。
组件类型 ID 默认 `blk_6900429af84180025ce76527`（官方公共组件，非密钥）；海外 Lark / 其他租户若不同，用 `--component-type-id` 覆盖。

## 身份

`doc htmlbox` 默认 **Bot（App Token）**，操作 feishu-cli 自建文档无需登录（AddOns 块 Bot 可建/读/删，与"搜索类必须 User Token"不同）。
⚠ **同一文档读写必须用同一身份**：Bot 建的文档用 User 身份去读会 `1770032 forBidden`。改他人分享、或你在飞书里手建的文档时，全程传 `--user-access-token`（或 `FEISHU_USER_ACCESS_TOKEN`）。

## update = 先建后删

飞书 OpenAPI 不支持原地改 `add_ons`（`PATCH` 返回 `1770001 invalid param`）。`update` 走「原位置新建 + 删旧块」：新块在原 index 建成后才删旧块，中途失败最多多一块、**不丢数据**。新块 `block_id` 与原来不同，输出返回 `new_block_id`，脚本里后续用它，别再用旧 id。

## iframe 沙箱能力边界

| 能做 | 别依赖 |
|---|---|
| 任意 HTML/CSS（`@keyframes`/`transition`/`clip-path`/`grid`） | 外网放行因环境而异 → CDN 加 `onerror` 兜底、关键场景自包含 |
| 内联 JS（`requestAnimationFrame`/`setInterval`/Canvas 2D/WebGL） | 需用户授权的浏览器 API、跨域表单提交、弹窗 |
| 加载 jsdelivr CDN（echarts / echarts-gl / three / gsap 实测可用） | 超大体积（避免内联巨型 base64，改用 CDN 或飞书图床） |
| 内联 `<svg>`（含 SMIL `<animate>`，是真浏览器渲染、会动） | 超长阻塞 JS |

## 沙箱运行时：window.magic

iframe 里飞书会注入 `window.magic` 运行时，**且认 `component_type_id`、不认块来源**——所以 `doc htmlbox`（纯 OpenAPI）建的块与妙笔编辑器建的等价，一样拿得到这层能力（已真机实测）。这是"妙笔BOX 不止画图、还是文档小程序平台"的机制根源。

⚠ `window.magic` 只在飞书文档端注入，本地 `file://` 预览没有，用前必须判存兜底。能力清单与配方见 `references/window-magic.md`。

## 与画板（feishu-cli-visual）的区别

| | 妙笔BOX（本技能） | 画板 `board svg-import` |
|---|---|---|
| 渲染 | iframe 真执行 | 服务端栅格化成静态位图 |
| 动画 | ✅ CSS/JS/SMIL 都能动 | ❌ 不会动 |
| 可编辑 | ❌ 整体 iframe，内部元素不可单独点选 | ✅ 每个节点可在飞书里单独改色/拖动 |

**鱼与熊掌**：要"动"用妙笔BOX，要"节点可编辑"用画板，飞书没有"既可编辑又会动"的形态。
