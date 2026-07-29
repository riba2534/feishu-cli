# 卡片预制素材库

本目录来自用户提供并明确授权复用的 `create-lark-card 1.0.6` 素材。素材保留在 Skill 内，
生成卡片时按语义选择，不依赖外部下载或跨租户 `img_key`。

206 张 PNG 与原压缩包逐文件 SHA-256 一致，可以直接原样复制。主题说明和 token 则按本项目
Card JSON 2.0、严格 linter 与 WCAG 4.5:1 正文对比度门槛做了适配；原始 palette 色值仍保留在
`custom-visual-palettes.json`，starter 对低对比的次要文字使用派生 `muted_text`。

## 内容

| 路径 | 数量 | 用途 |
|---|---:|---|
| `headers/modern-minimal/*.png` | 5 | info、success、warning、danger、neutral 封面 |
| `icons/phosphor/*.png` | 48 | Modern Minimal 线性图标 |
| `icons/doodle/*.png` | 153 | Custom Visual 手绘图标 |
| `themes/custom-visual-palettes.json` | 11 主题 | 自定义主题原始 palette 与规则 |
| `themes/modern-minimal-tokens.json` | 1 套 | Modern Minimal 明暗模式 token |

`custom-visual-palettes.json` 保留原 manifest 的来源说明：
palette/mood 受 `beautiful-feishu-whiteboard` 启发并转换为 Card JSON 2.0 约束。图片素材由
用户提供并授权随本 Skill 复用。

## 选择和复制

从 card workflow 目录执行：

```bash
python3 scripts/style_assets.py styles
python3 scripts/style_assets.py assets --family doodle --query trend
python3 scripts/style_assets.py copy doodle:trend-up.safe16 --output /tmp/trend-up.png
python3 scripts/style_assets.py tokens handdrawn-dark --output /tmp/handdrawn-dark-tokens.json
```

素材文件可以直接复制，也可以把其本地路径写入 Card JSON 2.0 的 `img.img_key` 或
`img_combination.img_list[].img_key`。发送前使用：

```bash
python3 scripts/lint_card.py --strict --upload-images /tmp/card.json
feishu-cli msg send ... --msg-type interactive --content-file /tmp/card.json --upload-images
```

`--upload-images` 会把本地路径上传到当前 App 的 IM 图床，并在发送前替换为当前租户有效的
`img_key`。不要把一次上传返回的 token 固化进 Skill。
