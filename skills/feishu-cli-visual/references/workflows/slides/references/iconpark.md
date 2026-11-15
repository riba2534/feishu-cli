# IconPark 图标

<!-- 改编自 larksuite/cli 的 lark-slides 技能 IconPark 说明（MIT License，见仓库根目录 THIRD_PARTY_NOTICES.md）；
     常用清单中的 iconType 均取自官方离线索引 iconpark-index.json。 -->

slides XML 用 `<icon>` 放 IconPark 图标。`iconType` 写成 `iconpark/<分类>/<名称>.svg`，**分类与名称必须真实存在**，
不要凭印象拼路径（例如 `lightbulb`、`info`、`help` 都不在图标库里）。本 skill 不内嵌完整索引（约 750KB），
优先从下方常用清单里选；清单没有时到 IconPark 官网（https://iconpark.oceanengine.com/official ）按英文名查，
分类取官网的英文分类名（Base、Charts、Office、Peoples、Safe、Travel 等）。

```xml
<icon iconType="iconpark/Charts/chart-line.svg" topLeftX="80" topLeftY="120" width="32" height="32">
  <fill>
    <fillColor color="rgba(37, 99, 235, 1)"/>
  </fill>
</icon>
```

## 使用规则

- 图标用于概念提示、步骤、状态、指标、角色和导航，不要用无关装饰图标填版面。
- 常用尺寸：行内状态图标 16-24px，卡片标题图标 28-40px，主视觉图标 56-96px。
- 必须用 `<fill>` 上色并与背景有足够对比；深色背景上放在浅色圆形/方形底上，或填 `rgba(255, 255, 255, 1)`。
- 清单外的图标写入后用 `slides screenshot` 回看：路径不存在时服务端不报错，而是静默改写成 `iconpark/Base/setting.svg`
  （画成齿轮，实测）；回读 XML 或截图看到意外的齿轮，就换成清单里的近义图标。

## 常用清单

### 通用

| 语义 | iconType |
|---|---|
| 设置/配置 | `iconpark/Base/setting.svg` |
| 目标 | `iconpark/Base/aiming.svg` |
| 搜索 | `iconpark/Base/search.svg` |
| 首页 | `iconpark/Base/home.svg` |
| 时间 | `iconpark/Time/time.svg` |
| 日历 | `iconpark/Edit/calendar.svg` |
| 提醒 | `iconpark/Music/remind.svg` |
| 标签 | `iconpark/Base/tag-one.svg` |
| 星标/重点 | `iconpark/Edit/star.svg` |
| 提示/想法 | `iconpark/Base/tips.svg` |
| 火箭/启动 | `iconpark/Travel/rocket.svg` |
| 旗帜/里程碑 | `iconpark/Travel/flag.svg` |
| 锁/安全 | `iconpark/Edit/lock.svg` |
| 链接 | `iconpark/Edit/link-one.svg` |
| 刷新/循环 | `iconpark/Base/refresh.svg` |

### 数据与图表

| 语义 | iconType |
|---|---|
| 增长趋势 | `iconpark/Charts/positive-dynamics.svg` |
| 下降趋势 | `iconpark/Charts/negative-dynamics.svg` |
| 折线趋势 | `iconpark/Charts/chart-line.svg` |
| 柱状图 | `iconpark/Charts/chart-histogram.svg` |
| 饼图 | `iconpark/Charts/chart-pie.svg` |
| 占比 | `iconpark/Charts/chart-proportion.svg` |
| 数据看板 | `iconpark/Charts/data-screen.svg` |
| 数据库 | `iconpark/Charts/data.svg` |
| 漏斗 | `iconpark/Edit/filter.svg` |

### 状态

| 语义 | iconType |
|---|---|
| 成功 | `iconpark/Character/check-one.svg` |
| 失败/风险 | `iconpark/Character/close-one.svg` |
| 错误/警告 | `iconpark/Abstract/error-prompt.svg` |
| 进行中 | `iconpark/Base/loading-four.svg` |
| 加号/新增 | `iconpark/Character/add-one.svg` |
| 减号/移除 | `iconpark/Character/reduce-one.svg` |

### 人与组织

| 语义 | iconType |
|---|---|
| 用户 | `iconpark/Peoples/user.svg` |
| 团队/用户 | `iconpark/Peoples/peoples.svg` |
| 组织架构 | `iconpark/Connect/tree-diagram.svg` |
| 消息/沟通 | `iconpark/Communicate/message.svg` |
| 点赞 | `iconpark/Hands/thumbs-up.svg` |
| 握手/合作 | `iconpark/Others/cooperative-handshake.svg` |

### 业务与办公

| 语义 | iconType |
|---|---|
| 文档 | `iconpark/Office/file-text.svg` |
| 文件夹 | `iconpark/Office/folder-open.svg` |
| 邮件/联系 | `iconpark/Office/envelope-one.svg` |
| 公文包/业务 | `iconpark/Clothes/briefcase.svg` |
| 钱/营收 | `iconpark/Money/wallet.svg` |
| 购物车 | `iconpark/Money/shopping-cart.svg` |
| 礼物/福利 | `iconpark/Money/gift.svg` |
| 奖杯/成果 | `iconpark/Game/trophy.svg` |
| 演示/汇报 | `iconpark/Music/ppt.svg` |
| 清单/任务 | `iconpark/Edit/list-checkbox.svg` |

### 技术

| 语义 | iconType |
|---|---|
| 代码 | `iconpark/Edit/code.svg` |
| 云存储 | `iconpark/Hardware/cloud-storage.svg` |
| 服务器 | `iconpark/Hardware/server.svg` |
| API/连接 | `iconpark/Connect/api.svg` |
| 手机 | `iconpark/Hardware/phone.svg` |
| 电脑 | `iconpark/Hardware/computer.svg` |
| 芯片/AI | `iconpark/Hardware/chip.svg` |
| 机器人 | `iconpark/Hardware/robot.svg` |
| 齿轮/流程 | `iconpark/Base/setting-two.svg` |

### 地点与交通

| 语义 | iconType |
|---|---|
| 全球/市场 | `iconpark/Travel/world.svg` |
| 定位 | `iconpark/Travel/local.svg` |
| 地图 | `iconpark/Travel/map-draw.svg` |
| 飞机/出差 | `iconpark/Travel/airplane.svg` |
| 汽车/物流 | `iconpark/Hardware/car.svg` |

### 安全

| 语义 | iconType |
|---|---|
| 安全防护 | `iconpark/Safe/protect.svg` |
| 盾牌 | `iconpark/Safe/shield.svg` |
| 钥匙 | `iconpark/Safe/key.svg` |
