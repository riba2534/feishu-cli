# 妙笔BOX HTML 范例库

可直接复制改用的自包含 HTML 模板。原则：**优先自包含（纯 CSS/Canvas/内联 JS）**，需要图表库时才用 CDN。
所有模板都设了深色背景、居中布局、`viewport`，落进飞书文档观感统一。

存成文件后用：`feishu-cli doc htmlbox create <doc_id> --html-file x.html`。

---

## 1. 纯 CSS 动画（最稳，不依赖外网）

一眼能看出"动"的三件套：旋转 / 脉动 / 变色。

```html
<!doctype html><html lang="zh"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><style>
body{margin:0;background:#0f1729;color:#fff;font-family:system-ui,-apple-system,sans-serif;
display:flex;justify-content:space-around;align-items:center;height:200px}
.demo{text-align:center}.label{font-size:12px;color:#cbd5e1;margin-top:14px}
@keyframes spin{to{transform:rotate(360deg)}}
@keyframes pulse{0%,100%{transform:scale(1);opacity:1}50%{transform:scale(1.6);opacity:.4}}
@keyframes hue{0%{background:#f9ca24}33%{background:#eb4d4b}66%{background:#6ab04c}100%{background:#f9ca24}}
.box{width:64px;height:64px;border-radius:12px;background:#ff6b6b;animation:spin 2.5s linear infinite}
.dot{width:60px;height:60px;border-radius:50%;background:#4ecdc4;animation:pulse 1.8s ease-in-out infinite}
.bar{width:78px;height:58px;border-radius:10px;animation:hue 4s linear infinite}
</style></head><body>
<div class="demo"><div class="box"></div><div class="label">旋转</div></div>
<div class="demo"><div class="dot"></div><div class="label">脉动</div></div>
<div class="demo"><div class="bar"></div><div class="label">变色</div></div>
</body></html>
```

可叠加的 CSS 动画：`transform: translate/rotate/scale`、`opacity`、`background`、`width`、`clip-path`、`left/top`（配 `position`）。
`animation: <name> <dur> <timing> infinite`，`@keyframes` 用 `from/to` 或 `0%/50%/100%`。

---

## 2. ECharts 图表（走 CDN，带兜底）

通用骨架：异步等待 ECharts 加载，失败有提示，自适应宽度。把 `OPT` 换成任意 ECharts option 即可。

```html
<!doctype html><html lang="zh"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><style>
body{margin:0;background:#fff;font-family:system-ui,sans-serif}.wrap{padding:14px}
h1{font-size:16px;margin:0 0 8px}#chart{width:100%;height:360px}#st{font-size:11px;color:#94a3b8}
</style></head><body><div class="wrap">
<h1>图表标题</h1><div id="chart"></div><p id="st">加载 ECharts…</p>
<script>var OPT={
  tooltip:{trigger:'item'},
  series:[{type:'graph',layout:'force',roam:true,draggable:true,
    label:{show:true,fontSize:11},force:{repulsion:260,edgeLength:[60,120],layoutAnimation:true},
    data:[{name:'A',symbolSize:48},{name:'B',symbolSize:38},{name:'C',symbolSize:30}],
    links:[{source:'A',target:'B'},{source:'A',target:'C'}]}]
};</script>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"
  onerror="document.getElementById('st').textContent='ECharts CDN 加载失败（该环境 iframe 可能禁外网）'"></script>
<script>(function(){var st=document.getElementById('st'),el=document.getElementById('chart'),n=0;
(function w(){if(typeof echarts!=='undefined'){st.textContent='';var c=echarts.init(el);c.setOption(OPT);
window.addEventListener('resize',function(){c.resize()});}else if(n++<40){setTimeout(w,150)}else{st.textContent='加载超时'}})();})();
</script></div></body></html>
```

常用 `OPT`（替换上面的 series 即可）：
- **力导向网络图**（上例，节点持续浮动 + 可拖拽）：`type:'graph', layout:'force'`
- **柱状竞赛**：`type:'bar'` + `setInterval` 定时 `setOption` 改 data + `animationDuration`
- **仪表盘**：`type:'gauge'`，指针从 0 扫到目标值，动画明显
- **关系/旭日/漏斗/热力**：换 `type` 即可，ECharts 自带 `animationDuration:1500` 入场动画

---

## 3. Canvas 粒子动画（自包含，requestAnimationFrame）

```html
<!doctype html><html><head><meta charset="utf-8"><style>
body{margin:0;background:#0a0e1a}canvas{display:block;width:100%;height:260px}
</style></head><body><canvas id="c"></canvas><script>
var cv=document.getElementById('c'),x=cv.getContext('2d');
function fit(){cv.width=cv.clientWidth;cv.height=cv.clientHeight}fit();addEventListener('resize',fit);
var P=Array.from({length:80},function(){return{x:Math.random()*cv.width,y:Math.random()*cv.height,
vx:(Math.random()-.5)*1.2,vy:(Math.random()-.5)*1.2}});
(function loop(){x.clearRect(0,0,cv.width,cv.height);x.fillStyle='#4ecdc4';
P.forEach(function(p){p.x=(p.x+p.vx+cv.width)%cv.width;p.y=(p.y+p.vy+cv.height)%cv.height;
x.beginPath();x.arc(p.x,p.y,2,0,7);x.fill();});
x.strokeStyle='rgba(78,205,196,.15)';P.forEach(function(a){P.forEach(function(b){
var d=Math.hypot(a.x-b.x,a.y-b.y);if(d<90){x.beginPath();x.moveTo(a.x,a.y);x.lineTo(b.x,b.y);x.stroke();}})});
requestAnimationFrame(loop);})();
</script></body></html>
```

---

## 4. CSS Dashboard 卡片（数值滚动 + 进度条）

```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><style>
body{margin:0;background:#0f1729;color:#fff;font-family:system-ui,sans-serif;padding:18px;
display:grid;grid-template-columns:repeat(3,1fr);gap:14px}
.card{background:#1a2236;border:1px solid #2d3748;border-radius:12px;padding:16px}
.k{font-size:12px;color:#8b9bb4}.v{font-size:28px;font-weight:700;margin:6px 0}
.track{height:6px;background:#2d3748;border-radius:3px;overflow:hidden}
.fill{height:100%;background:linear-gradient(90deg,#4ecdc4,#5b6cff);width:0;animation:grow 2s ease-out forwards}
@keyframes grow{to{width:var(--w)}}
</style></head><body>
<div class="card"><div class="k">QPS</div><div class="v">18.4k</div><div class="track"><div class="fill" style="--w:82%"></div></div></div>
<div class="card"><div class="k">成功率</div><div class="v">99.2%</div><div class="track"><div class="fill" style="--w:99%"></div></div></div>
<div class="card"><div class="k">延迟 P99</div><div class="v">86ms</div><div class="track"><div class="fill" style="--w:43%"></div></div></div>
</body></html>
```

---

## 尺寸 / 性能 / 沙箱注意

- **高度**：用 `body`/容器固定高度（如 `height:360px`），iframe 才不会塌缩或留大白边。
- **自适应宽度**：图表监听 `resize` 调 `c.resize()`；用 `width:100%`。
- **体积**：`record` 存的是整页 HTML，几 KB~几十 KB 没问题；别内联超大 base64 图片，改用 CDN 或飞书图床。
- **不要依赖**：跨域表单提交、需要用户授权的浏览器 API、超长阻塞 JS。
- **本地先自测**：落库前用浏览器打开 HTML 确认能动、无 JS 报错，再 `create`。
