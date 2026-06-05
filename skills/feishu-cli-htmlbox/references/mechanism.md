# 妙笔BOX 机制与边界

## 它是什么

「妙笔BOX」是飞书文档的 **AddOns HTML 小组件块**：

- 块类型 `block_type = 40`（AddOns）
- 组件类型 `component_type_id = blk_6900429af84180025ce76527`（飞书妙笔BOX 官方组件）
- 数据存在 `add_ons.record`，是一个 **JSON 字符串**：`{"html":"<整页 HTML>"}`

飞书渲染时把 `record.html` 丢进一个 **iframe 沙箱真实执行**——这就是它能跑 CSS/JS 动画、ECharts、Canvas、可拖拽图的根本原因。

block 结构（创建时）：

```json
{
  "block_type": 40,
  "add_ons": {
    "component_type_id": "blk_6900429af84180025ce76527",
    "record": "{\"html\":\"<!doctype html>…</html>\"}"
  }
}
```

## API 路径

| 操作 | 端点 | feishu-cli 命令 |
|------|------|-----------------|
| 创建 | `POST /open-apis/docx/v1/documents/{doc}/blocks/{parent}/children` | `doc htmlbox create` |
| 读取 | `GET /open-apis/docx/v1/documents/{doc}/blocks/{block}` | `doc htmlbox get` |
| 删除 | `DELETE`（按 parent + index 区间，`DeleteBlocks`） | `doc htmlbox delete` |
| 更新 | ❌ 不支持（见下） | `doc htmlbox update`（删除+重建） |

底层用 SDK：larkdocx v3 的 `Block.AddOns`（`AddOnsBuilder`）原生支持，`client.CreateBlock` 直接可用，无需手拼 HTTP。

## update 为什么是"先建后删、同位置重建"

飞书的 `PATCH /blocks/{block}` 更新块接口（`UpdateBlockRequest`）只支持
`update_text_elements` / `update_table_property` / `replace_image` 等**特定操作**，**没有更新 `add_ons` 的字段**。
实测直接 PATCH `{"add_ons":{...}}` 或 `{"update_add_ons":{...}}` 都返回 `1770001 invalid param`。

所以 `doc htmlbox update` 只能：定位原块的 index → 在原 index 新建新块 → 删除被后移到 index+1 的旧块
（**先建后删**：create 失败则原块不动、不丢数据）。**新块 block_id 与原来不同**，命令输出里返回 `new_block_id`。

## 身份与权限（重要）

- AddOns 块**可以用 Bot（Tenant）Token 创建/读/删**（实测 code 0）。这点和"搜索类必须 User Token"不同。
- feishu-cli 的 `doc htmlbox` 因此默认 **Bot 身份**（`resolveOptionalUserToken`），和 `doc add-board`/`doc add-callout` 一致——操作 `feishu-cli` 自己创建的文档无需登录。
- **同一文档读写必须用同一身份**。Bot 创建的文档，用 User 身份去读/写会 `1770032 forBidden`（User 对 Bot 私有文档无权限）；反之亦然。
- 操作他人分享、或你在飞书里手动创建的文档时，全程传 `--user-access-token`（或 `FEISHU_USER_ACCESS_TOKEN`）。

## iframe 沙箱能力边界

**能做**：
- 任意 HTML + CSS（含 `@keyframes` 动画、`transition`、`clip-path`、`grid/flex`）
- 任意内联 JS（`requestAnimationFrame`、`setInterval`、Canvas 2D、事件）
- 加载外网 CDN（实测 `cdn.jsdelivr.net` 上的 echarts/three/gsap 可加载）
- 内联 `<svg>`（含 SMIL `<animate>` 动画——在 iframe 里是真浏览器渲染，会动）

**不保证 / 别依赖**：
- 外网放行因环境而异：CDN 加载要加 `onerror` 兜底，关键场景优先自包含
- 需要用户授权的浏览器 API、跨域表单提交、弹窗
- 超大体积（record 是整页 HTML，避免内联巨型 base64）

## 与画板（feishu-cli-board）的本质区别

| | 妙笔BOX（本技能） | 画板 board svg-import |
|---|---|---|
| 渲染 | iframe 真浏览器执行 | 服务端把 SVG **栅格化成静态位图** |
| 动画 | ✅ CSS/JS/SMIL 都能动 | ❌ 不会动（栅格化后是静态图） |
| 可编辑性 | ❌ 整体 iframe，内部元素不可单独点选 | ✅ 每个节点可在飞书里单独改色/拖动 |
| 适用 | 动画、可交互图表、Dashboard | 静态流程图/架构图，要可编辑节点 |

**鱼与熊掌**：要"动"就用妙笔BOX（不可单独编辑内部元素）；要"节点可编辑"就用画板（不会动）。飞书没有"既可编辑又会动"的形态。

## 逆向来源

本能力是从两个真实会动的飞书文档逆向得到的：它们各有 8~9 个 `block_type=40` 的 AddOns 块，
`component_type_id` 都是 `blk_6900429af84180025ce76527`，`record.html` 里分别是 ECharts/Three.js（CDN）与纯 CSS/Canvas 动画。
`component_type_id` 作为官方组件大概率全局通用，但海外 Lark / 其他租户未必相同——`--component-type-id` 可覆盖。
