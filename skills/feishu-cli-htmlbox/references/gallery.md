# 妙笔BOX 绘制配方库

按图表类型组织的、**验证过能直接 `create` 的**绘制配方。地图 / echarts-gl 3D / Three.js 在 `geo-3d.md`。

用法：取下面对应的**骨架**，把对应配方的 `OPT`（ECharts 类）粘进骨架的 `OPT` 占位处，或直接用整段 HTML（Canvas/SVG/CSS 类）。统一深色底 `#0f1729`、青 `#36e0c6` / 金 `#ffd166` / 蓝 `#2e6cff` 配色。落库前务必本地浏览器验证（见 SKILL 工作流第 2 步）。

## 目录

- [通用骨架](#通用骨架)（ECharts / Canvas / SVG-CSS；Three.js 见 geo-3d.md）
- [数据对比](#数据对比)：折线 / 面积 / 柱状 / 堆叠柱 / 柱状竞赛 / 玫瑰饼 / 雷达
- [分布统计](#分布统计)：涟漪散点 / 热力 / 日历热力 / 箱线 / K线 / 平行坐标
- [构成流向](#构成流向)：漏斗 / 桑基 / 主题河流 / 仪表盘 / 水球 / 词云
- [关系层级](#关系层级)：力导向关系图 / 组织树 / 旭日 / 矩形树图
- [流程时序](#流程时序)：拓扑 / 编排动画 / 状态机 / 看板流动 / 甘特
- [创意动画](#创意动画)：CSS 三件套 / Canvas 粒子 / SVG 路径 / 维恩 / 像素柱 / KPI 大屏

---

## 通用骨架

### ECharts 骨架（数据/分布/构成/关系/流程类都用它）

把任一 ECharts 配方的 `OPT` 粘到 `★` 处即可。已含异步等待、`onerror` 兜底、`resize` 自适应、状态提示，以及妙笔BOX 高度协议 `<meta name="html-box-height-mode" content="auto">`（`auto`=宿主块高度跟内容，适合固定高度图表；全屏 Dashboard 才用 `viewport`）。异步变高（延迟加载、动态追加）后调 `window.magic&&window.magic.updateHeight()` 刷新宿主块高度。

```html
<!doctype html><html lang="zh"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="html-box-height-mode" content="auto"><style>
html,body{margin:0;background:#0f1729;color:#e6edf7;font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif}
#wrap{padding:8px 10px 12px}#chart{width:100%;height:380px}#st{margin:6px;font-size:13px;color:#8aa0c0;text-align:center}
</style></head><body><div id="wrap"><div id="chart"></div><p id="st">加载中…</p></div>
<script>
var OPT = { /* ★ 把下面任一配方的 OPT 粘到这里 */ };
function boot(){if(typeof echarts==='undefined')return setTimeout(boot,150);
  var c=echarts.init(document.getElementById('chart'));c.setOption(OPT);
  document.getElementById('st').textContent='';addEventListener('resize',function(){c.resize()});
  window.__c=c; /* 动态类配方(BarRace 等)会用到 */ }
</script>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"
  onerror="document.getElementById('st').textContent='ECharts CDN 加载失败'"></script>
<script>boot()</script>
</body></html>
```

通用深色样式约定（配方里复用，让观感统一）：
```js
// 标题
title:{left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16,fontWeight:600},subtextStyle:{color:'#6f86ab',fontSize:11}},
// 提示框
tooltip:{backgroundColor:'rgba(16,26,46,0.92)',borderColor:'#2b4a7a',textStyle:{color:'#e6edf7'}},
// 直角坐标轴
xAxis:{axisLine:{lineStyle:{color:'#2b4a7a'}},axisLabel:{color:'#9fb6d6'},splitLine:{show:false}},
yAxis:{axisLine:{lineStyle:{color:'#2b4a7a'}},axisLabel:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'rgba(60,90,130,.25)'}}},
backgroundColor:'#0f1729',
```

### Canvas 骨架（粒子 / 自绘动画）

```html
<!doctype html><html><head><meta charset="utf-8">
<meta name="html-box-height-mode" content="auto"><style>
html,body{margin:0;background:#0a0e1a}#c{display:block;width:100%;height:300px}
</style></head><body><canvas id="c"></canvas><script>
var cv=document.getElementById('c'),x=cv.getContext('2d');
function fit(){cv.width=cv.clientWidth;cv.height=cv.clientHeight}fit();addEventListener('resize',fit);
/* ★ 在这里建对象数组，并在 loop() 里更新+绘制 */
(function loop(){x.clearRect(0,0,cv.width,cv.height);
  /* ★ 每帧绘制 */
  requestAnimationFrame(loop)})();
</script></body></html>
```

### SVG / CSS 骨架（矢量动画 / 纯 CSS 动画）

内联 `<svg>` + CSS `@keyframes`/SMIL `<animate>` 直接动，不依赖任何库（最稳）。整段 HTML 见 [创意动画](#创意动画)。

---

## 数据对比

### 渐变面积折线 / 多指标平滑折线
「动」在入场逐段绘出（`animationDuration`）。多指标只需多加 series。
```js
OPT = { backgroundColor:'#0f1729', animationDuration:1400,
  title:{text:'周活跃趋势',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{trigger:'axis',backgroundColor:'rgba(16,26,46,.92)',borderColor:'#2b4a7a',textStyle:{color:'#e6edf7'}},
  grid:{left:48,right:20,top:48,bottom:30},
  xAxis:{type:'category',data:['周一','周二','周三','周四','周五','周六','周日'],boundaryGap:false,axisLine:{lineStyle:{color:'#2b4a7a'}},axisLabel:{color:'#9fb6d6'}},
  yAxis:{type:'value',axisLine:{show:false},axisLabel:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'rgba(60,90,130,.25)'}}},
  series:[{type:'line',smooth:true,data:[820,932,901,1134,1290,1330,1320],
    lineStyle:{width:3,color:'#36e0c6'},showSymbol:false,
    areaStyle:{color:{type:'linear',x:0,y:0,x2:0,y2:1,colorStops:[{offset:0,color:'rgba(54,224,198,.45)'},{offset:1,color:'rgba(54,224,198,0)'}]}}}]
};
```

### 多系列柱状 / 百分比堆叠柱
堆叠：所有 series 加同一 `stack:'总量'`。
```js
OPT = { backgroundColor:'#0f1729', animationDuration:1200,
  title:{text:'各渠道转化',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{trigger:'axis',axisPointer:{type:'shadow'},backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  legend:{top:30,textStyle:{color:'#9fb6d6'}}, grid:{left:44,right:20,top:64,bottom:30},
  xAxis:{type:'category',data:['Q1','Q2','Q3','Q4'],axisLabel:{color:'#9fb6d6'},axisLine:{lineStyle:{color:'#2b4a7a'}}},
  yAxis:{type:'value',axisLabel:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'rgba(60,90,130,.25)'}}},
  series:[
    {name:'新增',type:'bar',stack:'总量',data:[120,200,150,180],itemStyle:{color:'#2e6cff',borderRadius:[0,0,0,0]}},
    {name:'留存',type:'bar',stack:'总量',data:[80,120,110,140],itemStyle:{color:'#36e0c6'}},
    {name:'付费',type:'bar',stack:'总量',data:[40,60,70,90],itemStyle:{color:'#ffd166',borderRadius:[4,4,0,0]}}]
};
```

### 柱状竞赛 Bar Race（动态排序，强"动"感）
靠 `setInterval` 定时 `setOption` 改 data + `realtimeSort`。粘进骨架后**追加**这段（用骨架暴露的 `window.__c`）：
```js
OPT = { backgroundColor:'#0f1729',
  title:{text:'城市 GMV 竞赛',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  grid:{left:90,right:40,top:48,bottom:20},
  xAxis:{type:'value',axisLabel:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'rgba(60,90,130,.25)'}}},
  yAxis:{type:'category',inverse:true,max:4,data:['北京','上海','广州','深圳','杭州','成都'],axisLabel:{color:'#cde0ff'},animationDuration:300,animationDurationUpdate:300},
  series:[{type:'bar',realtimeSort:true,label:{show:true,position:'right',color:'#fff',valueAnimation:true},
    itemStyle:{color:'#36e0c6'},data:[10,20,30,40,50,60]}],
  animationDuration:0,animationDurationUpdate:1200,animationEasingUpdate:'linear'
};
// 追加到 boot() 之后：定时刷新数据
setInterval(function(){ if(!window.__c)return;
  var d=[10,20,30,40,50,60].map(function(v){return v+Math.round(Math.random()*v)});
  window.__c.setOption({series:[{type:'bar',data:d}]}); }, 1500);
```

### 玫瑰饼（南丁格尔）
```js
OPT = { backgroundColor:'#0f1729', animationDuration:1200,
  title:{text:'流量来源',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{trigger:'item',backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  series:[{type:'pie',roseType:'area',radius:[24,150],center:['50%','55%'],
    itemStyle:{borderColor:'#0f1729',borderWidth:2},
    label:{color:'#bcd2f0'},
    color:['#2e6cff','#36e0c6','#ffd166','#7df0ff','#9d7bff','#ff7a90'],
    data:[{value:38,name:'直接访问'},{value:52,name:'搜索引擎'},{value:61,name:'社交'},{value:45,name:'邮件'},{value:30,name:'广告'}]}]
};
```

### 雷达
```js
OPT = { backgroundColor:'#0f1729', animationDuration:1200,
  title:{text:'能力模型',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  radar:{center:['50%','56%'],radius:'66%',
    indicator:[{name:'性能',max:100},{name:'稳定',max:100},{name:'体验',max:100},{name:'成本',max:100},{name:'安全',max:100}],
    axisName:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'rgba(60,90,130,.35)'}},splitArea:{show:false},axisLine:{lineStyle:{color:'rgba(60,90,130,.35)'}}},
  series:[{type:'radar',data:[{value:[86,90,75,60,95],areaStyle:{color:'rgba(54,224,198,.35)'},lineStyle:{color:'#36e0c6'}}]}]
};
```

---

## 分布统计

### 涟漪气泡散点（effectScatter，点会脉冲）
```js
OPT = { backgroundColor:'#0f1729',
  title:{text:'渠道分布',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  grid:{left:44,right:20,top:48,bottom:30},
  xAxis:{type:'value',axisLabel:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'rgba(60,90,130,.25)'}}},
  yAxis:{type:'value',axisLabel:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'rgba(60,90,130,.25)'}}},
  series:[{type:'effectScatter',rippleEffect:{brushType:'stroke',scale:3.2},
    symbolSize:function(d){return Math.sqrt(d[2])/2},
    itemStyle:{color:'#43e8c4',shadowBlur:12,shadowColor:'rgba(67,232,196,.7)'},
    data:[[10,80,900],[30,40,400],[55,65,1600],[70,30,250],[85,70,1200]]}]
};
```

### 矩阵热力 heatmap
```js
OPT = { backgroundColor:'#0f1729',
  title:{text:'时段热力',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{position:'top',backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  grid:{left:60,right:20,top:48,bottom:50},
  xAxis:{type:'category',data:['0-6','6-9','9-12','12-15','15-18','18-21','21-24'],axisLabel:{color:'#9fb6d6'},splitArea:{show:true}},
  yAxis:{type:'category',data:['周一','周二','周三','周四','周五'],axisLabel:{color:'#9fb6d6'},splitArea:{show:true}},
  visualMap:{min:0,max:100,calculable:true,orient:'horizontal',left:'center',bottom:10,inRange:{color:['#10243e','#1e5fa8','#36e0c6','#ffd166']},textStyle:{color:'#9fb6d6'}},
  series:[{type:'heatmap',label:{show:false},data:(function(){var a=[];for(var y=0;y<5;y++)for(var x=0;x<7;x++)a.push([x,y,Math.round(Math.random()*100)]);return a})(),
    emphasis:{itemStyle:{shadowBlur:10,shadowColor:'rgba(0,0,0,.5)'}}}]
};
```

### 全年日历热力 calendar
```js
OPT = { backgroundColor:'#0f1729',
  title:{text:'提交日历',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  visualMap:{min:0,max:30,orient:'horizontal',left:'center',bottom:6,inRange:{color:['#10243e','#1e5fa8','#36e0c6','#ffd166']},textStyle:{color:'#9fb6d6'}},
  calendar:{top:60,left:30,right:20,cellSize:['auto',14],range:'2026',itemStyle:{color:'#0c1c33',borderColor:'#0f1729'},
    dayLabel:{color:'#9fb6d6'},monthLabel:{color:'#9fb6d6'},splitLine:{lineStyle:{color:'#2b4a7a'}}},
  series:[{type:'heatmap',coordinateSystem:'calendar',
    data:(function(){var a=[],d=new Date(2026,0,1);for(var i=0;i<365;i++){a.push([new Date(d.getTime()+i*864e5).toISOString().slice(0,10),Math.round(Math.random()*30)]);}return a})()}]
};
```

### 箱线 boxplot / K线 candlestick / 平行坐标 parallel
- **箱线**：`series:[{type:'boxplot',data:[[655,850,940,980,1070],...]}]`，配 `xAxis:{type:'category'}` + `yAxis:{type:'value'}`。
- **K线**：`series:[{type:'candlestick',data:[[open,close,low,high],...],itemStyle:{color:'#ff7a90',color0:'#36e0c6',borderColor:'#ff7a90',borderColor0:'#36e0c6'}}]`，`xAxis:{type:'category'}`。
- **平行坐标**：顶层 `parallelAxis:[{dim:0,name:'性能'},{dim:1,name:'成本'},...]` + `series:[{type:'parallel',data:[[...],[...]],lineStyle:{color:'#36e0c6',opacity:.5}}]`。

---

## 构成流向

### 漏斗 funnel
```js
OPT = { backgroundColor:'#0f1729', animationDuration:1200,
  title:{text:'转化漏斗',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{trigger:'item',backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  series:[{type:'funnel',top:50,bottom:20,gap:3,label:{color:'#fff'},
    color:['#2e6cff','#3b7bff','#36e0c6','#7df0ff','#ffd166'],
    data:[{value:100,name:'曝光'},{value:80,name:'点击'},{value:55,name:'加购'},{value:38,name:'下单'},{value:25,name:'支付'}]}]
};
```

### 桑基 Sankey
```js
OPT = { backgroundColor:'#0f1729',
  title:{text:'流量流向',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{trigger:'item',backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  series:[{type:'sankey',top:50,left:20,right:20,bottom:20,
    label:{color:'#cde0ff'},lineStyle:{color:'gradient',opacity:.4},
    data:[{name:'首页'},{name:'搜索'},{name:'详情'},{name:'下单'},{name:'流失'}],
    links:[{source:'首页',target:'搜索',value:60},{source:'首页',target:'详情',value:40},
      {source:'搜索',target:'详情',value:50},{source:'详情',target:'下单',value:55},{source:'详情',target:'流失',value:35}]}]
};
```

### 主题河流 themeRiver
```js
OPT = { backgroundColor:'#0f1729',
  title:{text:'话题热度',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{trigger:'axis',backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  singleAxis:{top:50,bottom:30,type:'time',axisLabel:{color:'#9fb6d6'},axisLine:{lineStyle:{color:'#2b4a7a'}}},
  series:[{type:'themeRiver',label:{color:'#cde0ff'},
    color:['#2e6cff','#36e0c6','#ffd166'],
    data:[['2026-01-01',30,'A'],['2026-01-02',45,'A'],['2026-01-03',38,'A'],
      ['2026-01-01',20,'B'],['2026-01-02',35,'B'],['2026-01-03',50,'B'],
      ['2026-01-01',15,'C'],['2026-01-02',25,'C'],['2026-01-03',30,'C']]}]
};
```

### 仪表盘 gauge（指针从 0 扫到目标，动感强）
```js
OPT = { backgroundColor:'#0f1729',
  series:[{type:'gauge',center:['50%','58%'],radius:'75%',min:0,max:100,
    progress:{show:true,width:14,itemStyle:{color:'#36e0c6'}},
    axisLine:{lineStyle:{width:14,color:[[1,'#1a2a44']]}},
    axisLabel:{color:'#9fb6d6'},axisTick:{show:false},splitLine:{lineStyle:{color:'#2b4a7a'}},
    pointer:{itemStyle:{color:'#ffd166'}},
    title:{color:'#9fb6d6'},detail:{valueAnimation:true,color:'#fff',fontSize:30,formatter:'{value}%'},
    data:[{value:82,name:'健康度'}]}]
};
```

### 水球 liquidFill / 词云 wordCloud（需扩展 CDN）
在 ECharts 骨架的 echarts `<script>` **后**再加扩展 CDN：
- 水球：`<script src="https://cdn.jsdelivr.net/npm/echarts-liquidfill@3/dist/echarts-liquidfill.min.js"></script>`，`series:[{type:'liquidFill',data:[0.62,0.55,0.5],color:['#36e0c6'],backgroundStyle:{color:'#0c1c33'},label:{color:'#fff',fontSize:36}}]`
- 词云：`<script src="https://cdn.jsdelivr.net/npm/echarts-wordcloud@2/dist/echarts-wordcloud.min.js"></script>`，`series:[{type:'wordCloud',gridSize:8,sizeRange:[12,48],rotationRange:[-45,45],textStyle:{color:function(){return ['#2e6cff','#36e0c6','#ffd166','#7df0ff'][Math.floor(Math.random()*4)]}},data:[{name:'飞书',value:100},{name:'文档',value:80},...]}]`
> 扩展类记得 boot() 里等扩展也就绪（`typeof echarts.graphic` 之类不可靠时，直接把 boot 的等待放到扩展 script 之后用 `<script>boot()</script>`）。

---

## 关系层级

### 力导向关系图（节点持续浮动 + 可拖拽，最像"活的"）
```js
OPT = { backgroundColor:'#0f1729',
  title:{text:'服务依赖',left:'center',top:6,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  series:[{type:'graph',layout:'force',roam:true,draggable:true,
    force:{repulsion:280,edgeLength:[60,140],layoutAnimation:true},
    label:{show:true,color:'#cde0ff',fontSize:11},
    lineStyle:{color:'#2b4a7a',curveness:.1},
    itemStyle:{color:'#36e0c6'},
    data:[{name:'网关',symbolSize:50,itemStyle:{color:'#ffd166'}},{name:'订单',symbolSize:38},{name:'支付',symbolSize:34},{name:'库存',symbolSize:30},{name:'用户',symbolSize:30}],
    links:[{source:'网关',target:'订单'},{source:'网关',target:'用户'},{source:'订单',target:'支付'},{source:'订单',target:'库存'}]}]
};
```

### 组织树 tree / 旭日 sunburst / 矩形树图 treemap
同一份层级数据，换 `series.type` 即可换表现：
```js
var TREE = {name:'总部',children:[
  {name:'研发',children:[{name:'前端',value:8},{name:'后端',value:12}]},
  {name:'市场',children:[{name:'品牌',value:6},{name:'增长',value:9}]}]};
// 组织树（径向：layout:'radial'）
OPT = { backgroundColor:'#0f1729', series:[{type:'tree',data:[TREE],top:30,left:60,right:60,bottom:20,
  symbolSize:9,label:{color:'#cde0ff',position:'left'},leaves:{label:{position:'right'}},
  lineStyle:{color:'#2b4a7a'},itemStyle:{color:'#36e0c6'},expandAndCollapse:true,initialTreeDepth:3}] };
// 旭日：series:[{type:'sunburst',data:TREE.children,radius:[0,'90%'],label:{color:'#fff'}}]
// 矩形树图：series:[{type:'treemap',data:TREE.children,roam:false,label:{color:'#fff'},breadcrumb:{show:false}}]
```

---

## 流程时序

| 你要画的图 | 引擎 / 方案 | 配方位置 |
|---|---|---|
| 拓扑 / 编排动画 / Agent 协作回放 | `scripts/animate_diagram.py`（结构化 JSON → 自包含 SVG 动画 HTML） | `references/animated-flowchart.md` |

时序图 / 状态机 / CI 流水线这类「流程」，ECharts 没有现成 series，两条路：
1. **简单的用 graph**（节点 + 有向边 `edgeSymbol:['none','arrow']`，`layout:'none'` 手摆坐标），见上方力导向改 `layout:'none'`。
2. **要动感的用纯 CSS**（节点高亮沿流程流动），见下例。

### 看板流动（纯 CSS，列间高亮卡片流动）
```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><style>
html,body{margin:0;background:#0f1729;color:#e6edf7;font-family:-apple-system,"PingFang SC",sans-serif}
.board{display:flex;gap:12px;padding:16px}.col{flex:1;background:#0c1c33;border-radius:10px;padding:10px;border:1px solid #1e3454}
.col h4{margin:0 0 8px;font-size:13px;color:#9fb6d6;text-align:center}
.card{background:#13294a;border-radius:8px;padding:8px 10px;margin:6px 0;font-size:12px;border-left:3px solid #36e0c6;
  animation:flow 4s ease-in-out infinite}
@keyframes flow{0%,100%{transform:translateX(0);opacity:.85}50%{transform:translateX(4px);opacity:1;box-shadow:0 0 12px rgba(54,224,198,.4)}}
</style></head><body>
<div class="board">
  <div class="col"><h4>待办</h4><div class="card">需求评审</div><div class="card" style="animation-delay:.5s">接口设计</div></div>
  <div class="col"><h4>进行中</h4><div class="card" style="border-color:#ffd166;animation-delay:1s">开发联调</div></div>
  <div class="col"><h4>已完成</h4><div class="card" style="border-color:#7df0ff;animation-delay:1.5s">单元测试</div></div>
</div></body></html>
```

### 甘特图
ECharts 用 `series:[{type:'custom',renderItem:...}]` 画横条；简单场景直接用 CSS 横条（每行一个 `.bar`，`width`/`left` 按时间百分比，`animation:grow` 入场）。复杂甘特建议 custom renderItem（参考 ECharts 官方 gantt 示例，套深色配色）。

---

## 创意动画

这些**纯前端、不依赖任何库**，最稳，整段 HTML 直接 `create`。

### CSS 三件套（旋转 / 脉动 / 变色）
```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><style>
body{margin:0;background:#0f1729;color:#e6edf7;font-family:system-ui,sans-serif;display:flex;justify-content:space-around;align-items:center;height:200px}
.demo{text-align:center}.label{font-size:12px;color:#9fb6d6;margin-top:14px}
@keyframes spin{to{transform:rotate(360deg)}}
@keyframes pulse{0%,100%{transform:scale(1);opacity:1}50%{transform:scale(1.6);opacity:.4}}
@keyframes hue{0%{background:#ffd166}50%{background:#36e0c6}100%{background:#2e6cff}}
.box{width:60px;height:60px;border-radius:12px;background:#ff7a90;animation:spin 2.5s linear infinite}
.dot{width:56px;height:56px;border-radius:50%;background:#36e0c6;animation:pulse 1.8s ease-in-out infinite}
.bar{width:72px;height:54px;border-radius:10px;animation:hue 4s linear infinite}
</style></head><body>
<div class="demo"><div class="box"></div><div class="label">旋转</div></div>
<div class="demo"><div class="dot"></div><div class="label">脉动</div></div>
<div class="demo"><div class="bar"></div><div class="label">变色</div></div>
</body></html>
```

### Canvas 粒子网络（rAF）
基于 Canvas 骨架，`★` 处填：
```js
var P=Array.from({length:90},function(){return{x:Math.random()*cv.width,y:Math.random()*cv.height,vx:(Math.random()-.5)*1.1,vy:(Math.random()-.5)*1.1}});
// loop 内：
x.fillStyle='#36e0c6';P.forEach(function(p){p.x=(p.x+p.vx+cv.width)%cv.width;p.y=(p.y+p.vy+cv.height)%cv.height;x.beginPath();x.arc(p.x,p.y,2,0,7);x.fill()});
x.strokeStyle='rgba(54,224,198,.15)';P.forEach(function(a){P.forEach(function(b){var d=Math.hypot(a.x-b.x,a.y-b.y);if(d<90){x.beginPath();x.moveTo(a.x,a.y);x.lineTo(b.x,b.y);x.stroke()}})});
```

### SVG 矢量路径自绘（描边动画 + 流动渐变 + 脉冲点）
```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><style>
body{margin:0;background:#0f1729}svg{width:100%;height:320px;display:block}
.wave{fill:none;stroke-width:2.4;stroke-linecap:round;stroke-dasharray:1400;stroke-dashoffset:1400;
  animation:draw 3s ease forwards,flow 2.2s linear 3s infinite}
@keyframes draw{to{stroke-dashoffset:0}}@keyframes flow{to{stroke-dashoffset:-560}}
.pulse{animation:pl 1.8s ease-in-out infinite}@keyframes pl{0%,100%{r:4;opacity:1}50%{r:9;opacity:.35}}
</style></head><body>
<svg viewBox="0 0 600 300"><defs>
<linearGradient id="g" x1="0" x2="1"><stop offset="0" stop-color="#2e6cff"/><stop offset=".5" stop-color="#36e0c6"/><stop offset="1" stop-color="#ffd166"/></linearGradient></defs>
<path class="wave" stroke="url(#g)" d="M40,210 C120,210 130,90 210,110 C290,130 290,250 370,230 C450,210 450,80 520,110"/>
<circle class="pulse" cx="210" cy="110" r="4" fill="#7df0ff"/><circle class="pulse" cx="370" cy="230" r="4" fill="#ffd166" style="animation-delay:.6s"/>
</svg></body></html>
```

### 维恩图（SVG，三圆交集 + screen 混合 + 脉动）
```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><style>
body{margin:0;background:#0f1729}svg{width:100%;height:340px;display:block}
.c{mix-blend-mode:screen;animation:br 4s ease-in-out infinite}@keyframes br{0%,100%{opacity:.55}50%{opacity:.8}}
</style></head><body>
<svg viewBox="0 0 480 340">
<circle class="c" cx="195" cy="140" r="100" fill="#2e6cff"/><circle class="c" cx="285" cy="140" r="100" fill="#36e0c6" style="animation-delay:1s"/>
<circle class="c" cx="240" cy="215" r="100" fill="#ffd166" style="animation-delay:2s"/>
<text x="150" y="135" fill="#e6edf7" font-size="14" text-anchor="middle">活跃</text>
<text x="330" y="135" fill="#e6edf7" font-size="14" text-anchor="middle">付费</text>
<text x="240" y="285" fill="#e6edf7" font-size="14" text-anchor="middle">高频</text></svg></body></html>
```

### 像素创意柱（pictorialBar，用图标堆叠成柱）
ECharts 骨架 + `series:[{type:'pictorialBar',symbol:'rect',symbolRepeat:true,symbolSize:[12,4],symbolMargin:2,data:[40,60,55,80],itemStyle:{color:'#36e0c6'}}]`，配 `xAxis:{type:'category',data:[...]}` + `yAxis`.

### KPI 数据大屏（数字从 0 滚到目标 + 迷你 sparkline）
```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><style>
body{margin:0;background:#0f1729;color:#e6edf7;font-family:-apple-system,"PingFang SC",sans-serif;padding:14px}
.grid{display:grid;grid-template-columns:repeat(3,1fr);gap:12px}
.card{background:linear-gradient(160deg,#13294a,#0c1c33);border:1px solid rgba(90,150,230,.25);border-radius:12px;padding:14px 16px}
.lab{font-size:12px;color:#8aa0c0}.num{font-size:28px;font-weight:700;margin:6px 0 2px}.num .u{font-size:13px;color:#6f86ab;font-weight:400}
.up{color:#36e0c6;font-size:12px}.down{color:#ff7a90;font-size:12px}
</style></head><body><div class="grid" id="g"></div><script>
var K=[{l:'日活',to:128456,u:'人',d:'+12.4%',up:1},{l:'GMV',to:8624,u:'万',d:'+8.1%',up:1},{l:'订单',to:53210,u:'单',d:'+5.6%',up:1}];
var g=document.getElementById('g');
K.forEach(function(k){g.insertAdjacentHTML('beforeend','<div class="card"><div class="lab">'+k.l+'</div><div class="num" data-to="'+k.to+'">0<span class="u">'+k.u+'</span></div><div class="'+(k.up?'up':'down')+'">'+(k.up?'▲':'▼')+' '+k.d+'</div></div>')});
var ns=document.querySelectorAll('.num'),t0=null;
function step(ts){if(!t0)t0=ts;var p=Math.min(1,(ts-t0)/1400),e=1-Math.pow(1-p,3);
  ns.forEach(function(n){var to=+n.getAttribute('data-to');n.innerHTML=Math.round(to*e).toLocaleString()+'<span class="u">'+n.querySelector('.u').textContent+'</span>'});
  if(p<1)requestAnimationFrame(step)}requestAnimationFrame(step);
</script></body></html>
```
