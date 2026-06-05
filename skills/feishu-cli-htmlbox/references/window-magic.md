# 妙笔BOX 文档小程序运行时（window.magic）

**关键事实（已实测确认）**：用 `doc htmlbox create`（纯 OpenAPI）创建的 `block_type=40` 块，在飞书文档里打开时被注入**完整的 `window.magic` 运行时**——和妙笔编辑器建的块一致（注入认 `component_type_id`，不认块来源）。

所以妙笔BOX 不只能画图，还能：**拿当前用户身份、读当前文档全文、持久化存储、读写多维表、调 AI**。要做「会读数据、能交互、有状态」的文档小程序时看这里；只画静态动图用 `gallery.md` / `geo-3d.md` 就够。

## 铁律：永远判存兜底

`window.magic` **只在飞书文档端注入**，本地 `file://` 预览、独立页都没有。所以本地验证（SKILL 工作流第 2 步）时它必为 `undefined`——**任何 window.magic 用法都要判存兜底**，否则本地直接白屏、线上某些环境也可能缺失：

```js
var m = window.magic;
if (!m) {
  // 本地预览 / 非文档环境：给 mock 或占位，保证页面不白屏
  render({ name: '本地预览', data: MOCK });
} else {
  // 飞书文档内：真实能力
  m.getCurrentUserInfo().then(render);
}
```

落库前本地验证时，看到「未注入 / 用了 mock」是**正常**的；真实能力要在飞书文档里打开才生效。

## 能力速查（实测可用）

| 能力 | 调用 | 说明 | 权限前提 |
|---|---|---|---|
| 当前用户 | `m.user`（同步缓存）/ `await m.getCurrentUserInfo()`（刷新） | name / avatar_url / open_id / union_id | 文档登录态 |
| 按 ID 取用户 | `await m.getUserInfoById(open_id)` | 他人 name/en_name/avatar | — |
| 文档全文 | `await m.getDocAsMarkdown()` | 当前文档正文转 Markdown 字符串 | 文档内 |
| 文档元信息 | `await m.getPageMeta()` | title / owner / pv / uv / 评论数 | 文档内 |
| 文档评论 | `await m.doc_comments_get(doc_token)` | 全文评论 | 文档内 |
| 私有持久化 | `m.store.get/set`、`m.store.global_get/global_set` | 组件私有（global=用户间共享） | store 需阅读权 |
| 跨复制持久化 | `m.redis.get/set`、`m.redis.global_get/global_set` | 复制文档后仍共享 | redis 需编辑权 |
| 多维表查询 | `await m.base_records_search(app_token,table_id,view_id,filter,sort,page_token,page_size)` | 带 filter/sort/分页 | 当前页对该 base 有读权 |
| 多维表批量取 | `await m.base_records_get(app_token,table_id,record_ids)` | 按 record_id 取 | 同上 |
| 多维表写 | `await m.base_record_create(app_token,table_id,fields)` / `base_record_update(...,record_id,fields)` | 人员字段用 `[{id:open_id}]` | 写权 |
| AI 调用 | `await m.ai({system,user,image_url,model,thinking})` → `{data:{result}}` | 豆包等大模型，含图片理解，无需自带 key | — |
| 高度刷新 | `m.updateHeight()`（别名 `refreshHeight`/`resize`） | 异步变高后刷新宿主块高度 | 配合 height-mode meta |

> **依赖妙笔托管后端、feishu-cli 场景不保证**（别在通用配方里用）：用别人 app 的 `LarkClient`、`m.proxy`、`initSDK(妙笔 appId)`、TOS 分片上传——这些走妙笔私有后端，脱离妙笔生态可能断链。

## 单能力配方

### 当前用户身份
```js
var m = window.magic;
var u = m ? (m.user || (m.getCurrentUserInfo && await m.getCurrentUserInfo())) : { name: '访客' };
el.textContent = '你好，' + (u && u.name);
// u.open_id / u.union_id 可用于写多维表人员字段：[{ id: u.open_id }]
```

### 读当前文档全文（喂给 AI / 统计字数）
```js
var md = window.magic && await window.magic.getDocAsMarkdown();
el.textContent = md ? ('本文档 ' + md.length + ' 字') : '本地预览无文档内容';
```

### 持久化存储（**替代 localStorage**，见 pitfalls 禁区）
```js
var m = window.magic;
// 复制传播仍共享 → redis；只想当前块私有 → store。global_* = 所有读者共享
if (m && m.redis) { await m.redis.set('count', n); n = +(await m.redis.get('count')) || 0; }
else { n = 0; /* 本地兜底，切勿用 localStorage */ }
```

### AI 调用
```js
var m = window.magic;
if (m && m.ai) {
  var r = await m.ai({ system: '你是简洁的助手', user: '一句话总结：' + text });
  el.textContent = r && r.data && r.data.result;
} else el.textContent = '（本地预览：AI 不可用）';
```

## 组合配方（杀手级场景）

### A. 文档内 AI 摘要 / 问答卡
当前文档全文 → AI → 渲染。**这是 window.magic 最有想象力的用法，且不用 feishu-cli 自己出 AI key。**
```html
<!doctype html><html lang="zh"><head><meta charset="utf-8">
<meta name="html-box-height-mode" content="auto"></head>
<body style="margin:0;background:#0f1729;color:#e6edf7;font:14px/1.7 -apple-system,'PingFang SC';padding:16px">
<div id="o">生成摘要中…</div>
<script>(async function(){
  var m=window.magic,o=document.getElementById('o');
  if(!m||!m.ai||!m.getDocAsMarkdown){o.textContent='本地预览：window.magic 不可用';return;}
  try{
    var md=await m.getDocAsMarkdown();
    var r=await m.ai({system:'你是文档助手，输出 3 条要点',user:'总结这篇文档:\n'+md.slice(0,6000)});
    o.textContent=(r&&r.data&&r.data.result)||'（无返回）';
  }catch(e){o.textContent='出错: '+e.message;}
  m.updateHeight&&m.updateHeight();
})();</script></body></html>
```

### B. 活数据 Dashboard（多维表 → ECharts）
把 `gallery.md` 里图表的写死 data 换成多维表实时数据。`<APP_TOKEN>` / `<TABLE_ID>` / `<VIEW_ID>` 由用户填。
```html
<!-- 用 gallery.md 的 ECharts 骨架，把 boot() 改成： -->
<script>
async function boot(){
  if(typeof echarts==='undefined') return setTimeout(boot,150);
  var c=echarts.init(document.getElementById('chart')), m=window.magic;
  var rows = (m && m.base_records_search)
    ? (await m.base_records_search('<APP_TOKEN>','<TABLE_ID>','<VIEW_ID>')).data.items
    : MOCK_ROWS;               // 本地兜底
  var cat=rows.map(function(r){return r.fields['名称']}), val=rows.map(function(r){return r.fields['数值']});
  c.setOption({backgroundColor:'#0f1729',
    xAxis:{type:'category',data:cat,axisLabel:{color:'#9fb6d6'}},
    yAxis:{type:'value',axisLabel:{color:'#9fb6d6'}},
    series:[{type:'bar',data:val,itemStyle:{color:'#36e0c6',borderRadius:[4,4,0,0]}}]});
  document.getElementById('st').textContent=''; addEventListener('resize',function(){c.resize()});
}
</script>
```

### C. 持久化互动（祝福墙 / 计数器 / 已读列表）
用 `redis.global_*` 让所有读者共享状态。
```js
var m=window.magic;
async function addBless(text){
  if(!m||!m.redis) return;
  var list=JSON.parse(await m.redis.global_get('blessings')||'[]');
  list.push({by:(m.user&&m.user.name)||'匿名', text:text});
  await m.redis.global_set('blessings', JSON.stringify(list));
  render(list);
}
```

## 落库与定位

- 写好带 window.magic 的 HTML（务必判存兜底）→ 本地验证不白屏（本地走 mock 分支）→ `doc htmlbox create <doc_id> --html-file x.html`。
- **真实能力只能在飞书文档里打开验证**（本地 `file://` 一定没有 window.magic）。要交付给用户的，按 `feishu-cli-write` 的 owner 授权流程转所有权后让用户打开。
