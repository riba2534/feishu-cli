# 妙笔BOX 实战踩坑与排障

本文件汇总**真实创建一篇 47 张图的 ECharts / Three.js / Canvas 能力演示文档**过程中踩出来的坑，按类型归纳。
和 `gallery.md` / `geo-3d.md`（给配方）、`mechanism.md`（讲机制）互补——这里讲**画图时实际会摔哪些跟头、怎么爬起来**。

## 目录

1. [JS 报错 = 整图白屏（最高频，且飞书界面看不到错误）](#1-js-报错--整图白屏)
2. [白屏的系统排查法](#2-白屏的系统排查法)
3. [CDN 加载与时序](#3-cdn-加载与时序)
4. [真实地图：ECharts 5 不含地图数据](#4-真实地图echarts-5-不含地图数据)
5. [特定图表类型的配置坑（geo 飞线 / 极坐标着色 / 3D）](#5-特定图表类型的配置坑)
6. [record 双重编码（批量读取或定位某张图时）](#6-record-双重编码)
7. [update 改图 block_id 会变](#7-update-改图-block_id-会变)
8. [批量追加多图 + 文档标题层级](#8-批量追加多图--文档标题层级)
9. [落库前本地验证流程（标准动作）](#9-落库前本地验证流程)
10. [状态持久化禁区：别用 localStorage](#10-状态持久化禁区别用-localstorage)

---

## 1. JS 报错 = 整图白屏

**症状**：块 `create` 成功、返回了 `block_id`，但飞书里那一块是一片空白（连背景色都对，就是没有图/没有动画）。

**根因**：iframe 里任何**顶层 JS 运行时错误**会中断整个 `<script>`，后面的 option、init 全都不执行。更阴险的是它常常**不报错给你看**：

- `function w(){…}` 这种函数声明会被 hoisting，所以「等库加载好再 init」的 `w` 即便前面的代码崩了，它本身仍然存在、仍然会被调用；
- `w()` 跑 `chart.setOption(OPT)` 时，`OPT` 因为脚本中途崩了**从未定义** → 实际是 `setOption(undefined)` → ECharts 内部抛 `Cannot read properties of undefined (reading 'baseOption')`；
- 这个未捕获异常走浏览器的 `pageerror` 事件，**不进 `console`**，所以飞书界面看不到、甚至 DevTools 的 console 也是空的——你只看到白屏，毫无线索。

**真实案例**：`var HUB={name:'北京',value:[50,52]}` 定义了 `value`，但构建飞线数据那行写成了 `HUB.coord.slice()`（`coord` 这个属性根本不存在）→ `undefined.slice()` 在脚本顶层抛错 → 整张图打不开。**一个变量名拼写错，就让整张图白屏，而且不报错。**

**教训**：htmlbox 的 HTML 没有「部分渲染失败」这回事——要么整张全好，要么整张全白。光读代码看不出这种运行时错误，**落库前必须本地浏览器真跑一遍**。

---

## 2. 白屏的系统排查法

别靠猜，按这套查（本地浏览器 DevTools，或 `agent-browser` / `playwright-cli`）：

1. **本地打开 HTML 文件**，看图出没出来。
2. **数 canvas**：`document.querySelectorAll('#chart canvas').length`
   - `0` 且 echarts 已加载 → `setOption` 抛异常了（实例 init 了、但没画出 canvas）。这是「白屏」最常见的内部状态。
3. **抓真实异常**（关键：未捕获异常不在 console）——在控制台 try/catch 复现：
   ```js
   try { var c = echarts.init(document.createElement('div')); c.setOption(OPT); }
   catch (e) { console.log(e.message, e.stack); }
   ```
   ⚠ 注意自动化工具（agent-browser eval 等）常在 **isolated world** 执行，拿不到页面里 `var OPT`（会显示 `undefined`，但 `window.echarts` 这类挂在全局的库能拿到）。这时把 option 直接**内联进测试代码**再 setOption，才能复现真实报错。
4. **二分定位**：逐个删 series / 删顶层配置（title/tooltip/grid/visualMap…）重试，找出是哪一项让 setOption 崩。
5. 旁证：连 `getOption()` 都抛异常 → 实例 model 已半损坏，进一步坐实 setOption 在早期就崩了。

`agent-browser` 自动化验证片段（落库前自测都靠它）：
```bash
agent-browser open "file:///tmp/widget.html"; sleep 3
agent-browser eval 'document.querySelectorAll("#chart canvas").length'   # 应 > 0
agent-browser console            # 看有没有报错
agent-browser screenshot /tmp/widget.png   # 截图肉眼确认
```

---

## 3. CDN 加载与时序

- 飞书 iframe 沙箱**能联 `cdn.jsdelivr.net`**：实测 echarts、echarts-gl、three.js 都能加载并运行（含 WebGL）。
- `<script src>` 是**异步**的，用之前必须「轮询等待库就绪」，否则 race 到 `undefined`：
  ```js
  function boot(){ if (typeof echarts === 'undefined') { return setTimeout(boot, 150); } /* init… */ }
  ```
- 不同库的「就绪判据」不同：

  | 库 | CDN（实测可用） | 就绪判据 |
  |----|----------------|----------|
  | echarts | `echarts@5/dist/echarts.min.js` | `typeof echarts !== 'undefined'` |
  | echarts-gl | `echarts-gl@2/dist/echarts-gl.min.js` | 它是 echarts 的**扩展**，要在 echarts 之后加载；等到 `echarts.getMap` 之类 API 就绪再用 |
  | three.js | `three@0.128.0/build/three.min.js`（UMD，挂 `window.THREE`） | `typeof THREE !== 'undefined'`。⚠ three 新版本改成 ESM 模块、`build/three.min.js` 不再暴露全局 `THREE`；想要全局变量就用 0.128 这类**老版 UMD build** |

- 每个 `<script src>` 都加 `onerror`，CDN 万一被某环境拦截时能显示原因，而不是白屏：
  ```html
  <script src="…" onerror="document.getElementById('st').textContent='CDN 加载失败'"></script>
  ```
- 越关键的图越优先「自包含」（纯 CSS / Canvas / 内联 JS），不依赖外网最稳。

---

## 4. 真实地图：ECharts 5 不含地图数据

- ECharts 5 **移除了内置地图 JSON**，直接写 `type:'map', map:'china'` 会是一片空白（没有地图底图）。
- 做法：先 `fetch` GeoJSON、再 `registerMap`，**必须在 then 回调里 init**（异步未就绪不能在外面用）：
  ```js
  fetch('https://cdn.jsdelivr.net/npm/echarts@4.9.0/map/json/china.json')
    .then(r => r.json())
    .then(geo => { echarts.registerMap('china', geo); /* echarts.init + setOption */ })
    .catch(() => { /* 地图数据加载失败提示 */ });
  ```
  （借 `echarts@4.9.0` 的 npm 包——它仍然带 `map/json`——来拿地图数据。）
- 平面地图（`type:'map'`）、geo 飞线（`geo` 组件）、3D 地图（`map3D`）**共用同一份 `registerMap('china', …)`**。

---

## 5. 特定图表类型的配置坑

- **geo 飞线**（比纯抽象坐标的飞线专业得多）：`geo:{map:'china'}` 组件打底 + `series` 用 `type:'lines', coordinateSystem:'geo'`，data 的 `coords` 填**真实经纬度** `[lng, lat]`；城市散点也用 `coordinateSystem:'geo'`。
- **极坐标柱状（polar bar）的颜色**：`visualMap` 对 polar bar **不一定按值上色**（实测整圈柱子全是同一色）。可靠做法是给每个 data 项**显式** `itemStyle.color`：
  ```js
  series:[{ type:'bar', coordinateSystem:'polar',
    data: VAL.map(v => ({ value:v, itemStyle:{ color: colorFor(v) } })) }]
  ```
- **3D（echarts-gl）**：`map3D` / `surface` / `bar3D` 都要先加载 echarts-gl；`viewControl:{autoRotate:true}` 让它自转，更有「在动」的观感；曲面用 `series.equation:{x,y,z:function(x,y){…}}` 直接给笛卡尔函数，不用手算网格。
- **Three.js**：用老版 UMD build（全局 `THREE`）；`new THREE.WebGLRenderer({canvas, alpha:true})` + `requestAnimationFrame` 里转 `group.rotation.y`。沙箱里 WebGL 可用（echarts-gl 本身就是 WebGL，能跑就证明放行了）。

---

## 6. record 双重编码

- 块的 `add_ons.record` 是个字符串 `{"html":"…"}`，而且里面 HTML 的 `<` 被编码成了 `<`（Go 的 json HTMLEscape 干的）。
- 所以你用 `feishu-cli api` 批量读 blocks、想**按标题/内容定位某一张图**时，直接对 `record` 做 `contains` / 正则会**匹配不到**（`<title>` 实际存成了 `<title>`）。
- 正确姿势：`fromjson` 解一层、取 `.html` 再匹配：
  ```bash
  feishu-cli api GET /open-apis/docx/v1/documents/<doc>/blocks --params '{"page_size":500}' --as bot \
    --jq '[.data.items[]|select(.block_type==40)] | to_entries[]
          | "\(.key+1)\t\((.value.add_ons.record|fromjson|.html|capture("<title>(?<t>[^<]*)</title>").t)? // "(无标题)")"' \
    --format ndjson
  ```
- 用这招能一次列出整篇文档每个 htmlbox 的标题、序号，并据此反查 `block_id`（`update`/`delete` 都要 block_id）。

---

## 7. update 改图 block_id 会变

飞书 OpenAPI 不支持原地更新 HTML 组件（`PATCH add_ons` 返回 `1770001 invalid param`），`update` 走「同位置新建 + 删旧块」（先建后删，避免中途失败丢数据），**新块 `block_id` 与原来不同**，输出里返回 `new_block_id`。脚本里 update 之后要拿 `new_block_id` 继续，别再用旧 id。机制详见 `mechanism.md`。

---

## 8. 批量追加多图 + 文档标题层级

一篇文档里放很多图，要有**标题结构**，不能一张接一张糊上去。做法是「标题」和「图」**交替追加**（两者都默认追加到文档末尾，最终顺序 = 调用顺序）：

```bash
# 大节标题（H2）+ 小标题（H3）一起追加，再补图
feishu-cli doc content-update <doc> --mode append --markdown $'## 四、地理类\n\n### 39. 全国分布地图'
feishu-cli doc htmlbox create   <doc> --html-file g39.html
feishu-cli doc content-update <doc> --mode append --markdown $'### 40. 实时飞线'
feishu-cli doc htmlbox create   <doc> --html-file g40.html
```

- 飞书 markdown：`## ` → Heading2，`### ` → Heading3。先用 `api … blocks` 看现有文档用的是哪一级标题，**对齐已有层级**（比如已有大节是 H2、每图是 H3，就续这个）。
- **限流坑**：每次 `feishu-cli` 是独立进程，进程内的 `docWriteLimiter` **跨进程不共享**，连续快速写会触发 `99991400`（频率超限）。批量循环里每步之间 `sleep 0.5~0.7s` 就稳了。
- `create --index -1`（默认）= 追加到文档末尾；要插到中间用 `--index N` 或 `--parent-id`。

---

## 9. 落库前本地验证流程

每张图固定走这套，别图省事跳过——这是上面 9 张新图**全部一次成功落库**的原因（白屏坑在本地就拦掉了）：

1. 写 HTML 到本地文件。
2. 浏览器 / `agent-browser` 打开 `file://`；CDN / 地图类**多等 3–4 秒**（要 fetch 资源）。
3. 检查渲染：canvas 或 svg 数 > 0、加载状态文本已清空、console 无 error。
4. **截图肉眼确认**——「有 canvas」不等于「画对了、在动」，看一眼最实在。
5. 通过后再 `doc htmlbox create`。失败就在本地修到好，别反复 create/update 去污染线上文档。

---

## 10. 状态持久化禁区：别用 localStorage

要在妙笔BOX 里存状态（计数器 / 抽奖 / 祝福墙 / 已读列表 / 用户上次选择），**别用 `localStorage`**——飞书沙箱环境里它会静默失效（官方妙笔文档三处反复强调）。用注入的 `window.magic` 持久化 API：

- `window.magic.store.get/set`：当前组件**私有**（各看各的），需阅读权限；`store.global_get/set` = 所有读者共享。
- `window.magic.redis.get/set`：**复制文档后仍共享**，需编辑权限；`redis.global_get/set` = 跨文档共享。

```js
var m = window.magic;
if (m && m.redis) { await m.redis.set('count', n); n = +(await m.redis.get('count')) || 0; }
else { n = 0; /* 本地预览兜底，绝不退回 localStorage */ }
```

`window.magic` 只在飞书文档端注入，本地 `file://` 没有——所以**带状态的配方必须判存兜底**（本地走 mock）。完整能力与配方见 `references/window-magic.md`。
