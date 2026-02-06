---
name: feishu-auth
description: |
  飞书用户认证管理工具。帮助用户通过 OAuth 2.0 授权流程获取 User Access Token，
  支持自动刷新和状态查看。获取的 Token 可用于搜索消息、搜索应用等需要用户身份的操作。
argument-hint: "[login|refresh|status]"
user-invocable: true
allowed-tools: ["Bash"]
---

# 飞书用户认证 Skill

## 功能概述

此 Skill 帮助用户完成飞书 OAuth 2.0 授权流程，获取 User Access Token。

## 授权流程

1. 用户运行 `/feishu-auth login`
2. Skill 检查 App ID 和 App Secret 是否配置
3. 启动本地 HTTP 服务器监听指定端口（默认 8080）
4. 生成授权 URL 并提示用户访问
5. 用户在浏览器中授权后，飞书重定向到本地服务器
6. 服务器接收授权码，调用 API 换取 token
7. 保存 token 到 `~/.lark_user_token` 文件
8. 显示 token 信息和使用说明

## 使用方法

### 登录获取 Token

```
/feishu-auth login
```

选项：
- `-p, --port`: 指定本地服务器端口（默认 8080，0 表示随机端口）
- `-H, --host`: 指定本地服务器主机（默认 127.0.0.1）

示例：
```
/feishu-auth login --port 3000
```

### 刷新 Token

当 access_token 过期时，使用 refresh_token 获取新的 token：

```
/feishu-auth refresh
```

### 查看 Token 状态

```
/feishu-auth status
```

显示当前 token 的来源、过期时间等信息。

## 命令详解

### login 命令

启动完整的 OAuth 授权流程：

1. 检查应用凭证（App ID 和 App Secret）
2. 启动本地 HTTP 服务器
3. 生成授权链接
4. 等待用户授权
5. 自动获取并保存 token

Token 文件位置：`~/.lark_user_token`

### refresh 命令

使用 refresh_token 换取新的 access_token：
- 无需重新登录
- refresh_token 有效期通常为 30 天
- 刷新后会更新 token 文件

### status 命令

显示 token 状态，按以下优先级检查：
1. `FEISHU_USER_ACCESS_TOKEN` 环境变量
2. 配置文件中的 `user_access_token`
3. `~/.lark_user_token` 文件

## Token 文件格式

`~/.lark_user_token` (JSON 格式):
```json
{
  "access_token": "u-xxx",
  "refresh_token": "ur-xxx",
  "expires_at": 1704067200,
  "expires_in": 7200
}
```

文件权限：0600（仅所有者可读写）

## 与其他命令集成

其他需要 User Access Token 的命令（如 search）将自动：
1. 检查 `FEISHU_USER_ACCESS_TOKEN` 环境变量
2. 检查配置文件中的 `user_access_token`
3. 检查 `~/.lark_user_token` 文件
4. 按优先级使用第一个找到的 token
5. 如果 token 过期且来自文件，自动刷新

## 前置要求

1. 已配置飞书应用凭证（App ID 和 App Secret）
2. 应用需要在飞书开放平台开启"网页应用"能力
3. 需要在应用设置中配置正确的回调地址（如 `http://127.0.0.1:8080/callback`）

## 常见问题

### Q: 授权时提示"回调地址不匹配"？
A: 请确保在飞书开放平台应用设置的"安全设置"中添加了正确的回调地址，
   例如 `http://127.0.0.1:8080/callback`。

### Q: Token 过期了怎么办？
A: 运行 `/feishu-auth refresh` 刷新 token，或重新运行 `/feishu-auth login`。

### Q: 如何切换账号？
A: 删除 `~/.lark_user_token` 文件，然后重新运行 `/feishu-auth login`。

### Q: 端口被占用怎么办？
A: 使用 `--port` 指定其他端口，如 `/feishu-auth login --port 3000`。

## 相关链接

- 飞书 OAuth 文档: https://open.feishu.cn/document/server-side-identification/authen/login
