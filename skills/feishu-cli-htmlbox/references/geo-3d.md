# 地图 / 3D / Three.js 完整模板

这几类有 CDN 加载、`registerMap`、坐标系、着色等坑，所以**给完整可直接 `create` 的 HTML**（都已本地验证渲染）。
通用配色：深色底 `#0f1729`；**数据系列色**遵循 `gallery.md` 开头的统一色板约定（`#3370ff` / `#bf8600` / `#00ad96` 起按序）；地图发光、大气层等**特效装饰色**沿用模板原值即可。落库前仍按 SKILL 工作流第 2 步本地验证（地图/CDN 类多等到 4s）。

## 目录
- [真实地图着色 choropleth](#真实地图着色-choropleth)
- [经纬度 geo 飞线](#经纬度-geo-飞线)
- [3D 立体地图 map3D](#3d-立体地图-map3d)
- [3D 柱 / 3D 散点（echarts-gl）](#3d-柱--3d-散点echarts-gl)
- [3D 曲面 surface](#3d-曲面-surface)
- [Three.js 真 3D 场景](#threejs-真-3d-场景)
- [极坐标柱状 polar](#极坐标柱状-polar)

> **地图数据来源**：ECharts 5 不带内置地图。下面地图类都靠 `fetch('https://cdn.jsdelivr.net/npm/echarts@4.9.0/map/json/china.json')` 拿中国 GeoJSON 再 `registerMap('china', geo)`。换世界地图用 `world.json`，换省级用 DataV：`https://geo.datav.aliyun.com/areas_v3/bound/{adcode}_full.json`。**必须在 fetch 的 then 回调里 init**。

---

## 真实地图着色 choropleth

各省按值热力着色。`type:'map'` + `visualMap`。

```html
<!doctype html><html lang="zh"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><style>
html,body{margin:0;background:#0f1729;color:#e6edf7;font-family:-apple-system,"PingFang SC",sans-serif}
#wrap{padding:8px 10px 12px}#chart{width:100%;height:420px}#st{margin:6px;font-size:13px;color:#8aa0c0;text-align:center}
</style></head><body><div id="wrap"><div id="chart"></div><p id="st">加载中…</p></div>
<script>
var PROV=[{name:'广东',value:1860},{name:'江苏',value:1520},{name:'浙江',value:1380},{name:'山东',value:1210},
{name:'北京',value:1150},{name:'上海',value:1090},{name:'四川',value:980},{name:'湖北',value:870},{name:'河南',value:760},
{name:'福建',value:720},{name:'湖南',value:680},{name:'河北',value:640},{name:'陕西',value:560},{name:'四川',value:980}];
function render(geo){
  echarts.registerMap('china',geo);
  var c=echarts.init(document.getElementById('chart'));
  c.setOption({backgroundColor:'#0f1729',
    title:{text:'全国业务分布',subtext:'各省接入量',left:'center',top:8,textStyle:{color:'#cde0ff',fontSize:16},subtextStyle:{color:'#6f86ab'}},
    tooltip:{trigger:'item',backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'},formatter:function(p){return p.name+'<br/>'+(p.value||0)}},
    visualMap:{min:0,max:1900,left:18,bottom:18,calculable:true,inRange:{color:['#1c3672','#2b57bd','#4b80f5','#8ab1ff','#cedfff']},textStyle:{color:'#9fb6d6'}},
    series:[{type:'map',map:'china',roam:false,zoom:1.2,label:{show:false},
      itemStyle:{borderColor:'rgba(120,180,255,.35)',areaColor:'#10243e'},
      emphasis:{label:{show:true,color:'#fff'},itemStyle:{areaColor:'#3370ff'}},data:PROV}]});
  document.getElementById('st').textContent='';addEventListener('resize',function(){c.resize()});
}
function boot(){if(typeof echarts==='undefined')return setTimeout(boot,150);
  fetch('https://cdn.jsdelivr.net/npm/echarts@4.9.0/map/json/china.json').then(function(r){return r.json()}).then(render)
    .catch(function(){document.getElementById('st').textContent='地图数据加载失败'})}
</script>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js" onerror="document.getElementById('st').textContent='CDN加载失败'"></script>
<script>boot()</script>
</body></html>
```

---

## 经纬度 geo 飞线

中心 → 各城市的真实经纬度飞线动画。`geo` 组件打底 + `lines`(coordinateSystem:'geo') + `effectScatter`。比 cartesian2d 抽象坐标专业得多。在上面地图模板基础上，把 `render` 改成：

```js
var G={'北京':[116.40,39.90],'上海':[121.47,31.23],'广州':[113.26,23.13],'深圳':[114.06,22.54],'成都':[104.07,30.57],
'杭州':[120.15,30.27],'武汉':[114.30,30.59],'西安':[108.95,34.27],'沈阳':[123.43,41.80],'重庆':[106.55,29.56]};
var HUB='北京', TO=['上海','广州','深圳','成都','杭州','武汉','西安','沈阳','重庆'];
function render(geo){
  echarts.registerMap('china',geo);
  var lines=TO.map(function(n){return{coords:[G[HUB],G[n]]}});
  var sc=TO.map(function(n){return{name:n,value:G[n]}});
  var c=echarts.init(document.getElementById('chart'));
  c.setOption({backgroundColor:'#0f1729',
    title:{text:'实时数据飞线',left:'center',top:8,textStyle:{color:'#cde0ff',fontSize:16}},
    geo:{map:'china',roam:false,zoom:1.2,itemStyle:{areaColor:'#0c1c33',borderColor:'rgba(90,150,230,.35)'},emphasis:{itemStyle:{areaColor:'#13294a'}}},
    series:[
      {type:'lines',coordinateSystem:'geo',zlevel:2,effect:{show:true,period:5,trailLength:.4,symbol:'arrow',symbolSize:7,color:'#ffffff'},lineStyle:{width:1.2,opacity:.6,curveness:.25,color:'#00ad96'},data:lines},
      {type:'effectScatter',coordinateSystem:'geo',zlevel:3,rippleEffect:{brushType:'stroke',scale:3},symbolSize:8,itemStyle:{color:'#00ad96'},label:{show:true,position:'right',formatter:'{b}',color:'#bcd2f0',fontSize:10},data:sc},
      {type:'effectScatter',coordinateSystem:'geo',zlevel:4,rippleEffect:{brushType:'stroke',scale:5},symbolSize:16,itemStyle:{color:'#bf8600'},label:{show:true,position:'top',formatter:'{b}',color:'#ffe6a3',fontWeight:600},data:[{name:HUB,value:G[HUB]}]}]});
  document.getElementById('st').textContent='';addEventListener('resize',function(){c.resize()});
}
```

---

## 3D 立体地图 map3D

各省按值挤出高度、可自动旋转。需 **echarts-gl**（在 echarts 之后再加一个 CDN `<script>`）。在地图模板基础上：① `<head>` 后加 `<script src="https://cdn.jsdelivr.net/npm/echarts-gl@2/dist/echarts-gl.min.js" onerror="..."></script>`（放在 echarts 的 script 之后）；② `boot()` 的等待条件改成等地图 API 就绪（`typeof echarts==='undefined' || typeof echarts.getMap==='undefined'`）；③ `render` 改成：

```js
function render(geo){
  echarts.registerMap('china',geo);
  var c=echarts.init(document.getElementById('chart'));
  c.setOption({backgroundColor:'#0f1729',
    title:{text:'3D 立体中国地图',left:'center',top:8,textStyle:{color:'#cde0ff',fontSize:16}},
    visualMap:{min:0,max:1900,left:18,bottom:18,calculable:true,inRange:{color:['#1c3672','#2b57bd','#4b80f5','#8ab1ff','#cedfff']},textStyle:{color:'#9fb6d6'}},
    series:[{type:'map3D',map:'china',regionHeight:3,shading:'realistic',
      realisticMaterial:{roughness:.55,metalness:.1},
      light:{main:{intensity:1.4,shadow:true,alpha:40,beta:30},ambient:{intensity:.35}},
      viewControl:{distance:130,alpha:55,beta:8,autoRotate:true,autoRotateSpeed:8},
      itemStyle:{borderColor:'rgba(125,240,255,.5)',borderWidth:.6},
      emphasis:{itemStyle:{color:'#bf8600'}},label:{show:false},data:PROV}]});
  document.getElementById('st').textContent='';addEventListener('resize',function(){c.resize()});
}
```

---

## 3D 柱 / 3D 散点（echarts-gl）

不需要地图，但同样要 echarts-gl CDN。用 `grid3D` + `xAxis3D/yAxis3D/zAxis3D`，`series.type` 为 `bar3D` 或 `scatter3D`，`grid3D.viewControl.autoRotate:true` 自转。

```js
// OPT（粘进「带 echarts-gl 的骨架」，等待条件用 typeof echarts!=='undefined'）
OPT = { backgroundColor:'#0f1729',
  title:{text:'3D 柱状',left:'center',top:8,textStyle:{color:'#cde0ff',fontSize:16}},
  visualMap:{max:100,inRange:{color:['#2b57bd','#4b80f5','#8ab1ff']},left:14,bottom:16,textStyle:{color:'#9fb6d6'}},
  xAxis3D:{type:'category',data:['A','B','C','D']},yAxis3D:{type:'category',data:['一','二','三']},zAxis3D:{type:'value'},
  grid3D:{viewControl:{autoRotate:true,distance:200},light:{main:{intensity:1.2},ambient:{intensity:.4}}},
  series:[{type:'bar3D',shading:'lambert',data:(function(){var a=[];for(var x=0;x<4;x++)for(var y=0;y<3;y++)a.push([x,y,Math.round(Math.random()*100)]);return a})()}]
};
// 3D 散点星云：把 series 换成 type:'scatter3D'，data 为 [x,y,z]，itemStyle:{opacity:.8}，symbolSize:6
```

---

## 3D 曲面 surface

`z=f(x,y)` 函数曲面，可旋转。echarts-gl，用 `series.equation` 直接给笛卡尔函数（无需手算网格）。

```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>
html,body{margin:0;background:#0f1729;color:#e6edf7}#chart{width:100%;height:430px}#st{margin:6px;font-size:13px;color:#8aa0c0;text-align:center}
</style></head><body><div id="chart"></div><p id="st">加载中…</p>
<script>
function w(){if(typeof echarts==='undefined'||typeof echarts.getMap==='undefined')return setTimeout(w,150);
  var c=echarts.init(document.getElementById('chart'));
  c.setOption({backgroundColor:'#0f1729',
    title:{text:'3D 曲面',left:'center',top:8,textStyle:{color:'#cde0ff',fontSize:16}},
    // z∈[-1,1] 是极性数据 → diverging：两极最亮、中点贴近画布读作"零"
    visualMap:{dimension:2,min:-1,max:1,left:14,bottom:16,calculable:true,inRange:{color:['#8ab1ff','#2b57bd','#373a3d','#893b00','#ea6a03']},textStyle:{color:'#9fb6d6'}},
    xAxis3D:{},yAxis3D:{},zAxis3D:{},
    grid3D:{viewControl:{autoRotate:true,distance:200,alpha:25,beta:40},light:{main:{intensity:1.2},ambient:{intensity:.4}}},
    series:[{type:'surface',wireframe:{show:false},shading:'color',
      equation:{x:{min:-3,max:3,step:.12},y:{min:-3,max:3,step:.12},z:function(x,y){var r=Math.sqrt(x*x+y*y);return Math.sin(r*2.2)/(r+.6)}}}]});
  document.getElementById('st').textContent='';addEventListener('resize',function(){c.resize()});}
</script>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js" onerror="document.getElementById('st').textContent='CDN加载失败'"></script>
<script src="https://cdn.jsdelivr.net/npm/echarts-gl@2/dist/echarts-gl.min.js" onerror="document.getElementById('st').textContent='echarts-gl 加载失败'"></script>
<script>w()</script>
</body></html>
```

---

## Three.js 真 3D 场景

ECharts/echarts-gl 给不出的自由 3D（旋转星球、GPU 粒子、自定义几何/着色器）用 Three.js。
**用老版 UMD build**（`three@0.128.0/build/three.min.js`，暴露全局 `THREE`；新版改 ESM 没有全局变量）。下例：线框星球 + 环绕粒子。

```html
<!doctype html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>
html,body{margin:0;background:#0f1729;color:#e6edf7}#cv{width:100%;height:380px;display:block}#st{margin:6px;font-size:13px;color:#8aa0c0;text-align:center}
</style></head><body><canvas id="cv"></canvas><p id="st">加载中…</p>
<script>
function boot(){if(typeof THREE==='undefined')return setTimeout(boot,150);
  var cv=document.getElementById('cv'),w=cv.clientWidth||600,h=380;
  var r=new THREE.WebGLRenderer({canvas:cv,antialias:true,alpha:true});r.setSize(w,h,false);r.setClearColor(0x0f1729,1);
  var sc=new THREE.Scene(),cam=new THREE.PerspectiveCamera(55,w/h,.1,100);cam.position.z=4;
  var grp=new THREE.Group();sc.add(grp);
  grp.add(new THREE.LineSegments(new THREE.WireframeGeometry(new THREE.IcosahedronGeometry(1.5,3)),new THREE.LineBasicMaterial({color:0x36e0c6,transparent:true,opacity:.45})));
  var N=800,pos=new Float32Array(N*3);
  for(var i=0;i<N;i++){var t=Math.acos(2*Math.random()-1),f=2*Math.PI*Math.random(),rr=1.9+Math.random()*1.1;
    pos[i*3]=rr*Math.sin(t)*Math.cos(f);pos[i*3+1]=rr*Math.sin(t)*Math.sin(f);pos[i*3+2]=rr*Math.cos(t)}
  var pg=new THREE.BufferGeometry();pg.setAttribute('position',new THREE.BufferAttribute(pos,3));
  grp.add(new THREE.Points(pg,new THREE.PointsMaterial({color:0x7df0ff,size:.035,transparent:true,opacity:.9})));
  document.getElementById('st').textContent='';
  (function loop(){requestAnimationFrame(loop);grp.rotation.y+=.0045;grp.rotation.x+=.0016;r.render(sc,cam)})();
  addEventListener('resize',function(){var nw=cv.clientWidth||600;r.setSize(nw,h,false);cam.aspect=nw/h;cam.updateProjectionMatrix()});}
</script>
<script src="https://cdn.jsdelivr.net/npm/three@0.128.0/build/three.min.js" onerror="document.getElementById('st').textContent='Three.js CDN加载失败'"></script>
<script>boot()</script>
</body></html>
```

> 沙箱里 WebGL 可用（echarts-gl 也是 WebGL，能跑就证明放行）。

---

## 极坐标柱状 polar

环形分布的柱状。**坑：`visualMap` 对 polar bar 不按值上色（整圈同色）**，所以给每个 data 项显式 `itemStyle.color`。粘进 ECharts 骨架：

```js
var CAT=['周一','周二','周三','周四','周五','周六','周日','大促','夜间','凌晨','早高峰','节假日'];
var VAL=[82,95,78,120,134,156,168,210,64,42,150,190];
function colorFor(v){var t=Math.max(0,Math.min(1,(v-40)/170));function m(a,b,k){return Math.round(a+(b-a)*k)}
  var c1,c2,k;if(t<.5){c1=[30,95,168];c2=[54,224,198];k=t/.5}else{c1=[54,224,198];c2=[255,209,102];k=(t-.5)/.5}
  return 'rgb('+m(c1[0],c2[0],k)+','+m(c1[1],c2[1],k)+','+m(c1[2],c2[2],k)+')'}
OPT = { backgroundColor:'#0f1729',
  title:{text:'各时段请求量',left:'center',top:8,textStyle:{color:'#cde0ff',fontSize:16}},
  tooltip:{trigger:'item',backgroundColor:'rgba(16,26,46,.92)',textStyle:{color:'#e6edf7'}},
  polar:{radius:[28,'78%'],center:['50%','54%']},
  angleAxis:{type:'category',data:CAT,startAngle:90,axisLine:{lineStyle:{color:'#2b4a7a'}},axisLabel:{color:'#9fb6d6',fontSize:11},axisTick:{show:false}},
  radiusAxis:{axisLine:{show:false},axisLabel:{color:'#5d769a'},splitLine:{lineStyle:{color:'rgba(60,90,130,.3)'}}},
  series:[{type:'bar',coordinateSystem:'polar',roundCap:true,
    data:VAL.map(function(v){return {value:v,itemStyle:{color:colorFor(v),borderRadius:4}}})}]
};
```
