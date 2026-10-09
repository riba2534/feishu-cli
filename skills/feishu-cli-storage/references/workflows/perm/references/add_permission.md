# perm add 输入检查清单

执行 `perm add` 前，逐项确认以下内容：

## 1. TOKEN 与 doc-type 匹配

前缀 → doc-type 对照见主工作流 [`../workflow.md`](../workflow.md) 的「参数取值」。

新版 token 多数没有类型前缀：有 URL 时按路径判断；只有裸 token 时运行
`feishu-cli drive inspect --url <token> -o json`，用输出的 `token` 配 `type`。wiki 节点会被展开为底层文档并附带
`wiki_node`：给知识库节点授权（可配 `--perm-type`）时用 `wiki_node.node_token` 配 `--doc-type wiki`。

## 2. member-type 与 member-id 一致

| member-type | member-id 格式 |
|-------------|---------------|
| email | user@example.com |
| openid | `ou_` 前缀 |
| userid | 纯数字或自定义 ID |
| unionid | `on_` 前缀 |
| openchat | `oc_` 前缀 |
| opendepartmentid | `od_` 前缀 |
| groupid | `gc_` 前缀 |
| wikispaceid | `ws_` 前缀 |

## 3. 身份与知识库范围

- 默认 Bot 身份；文档属于当前登录用户、应用不是协作者时加 `--as user`（Bot 会报 1063004/1063002）
- 知识库节点（`--doc-type wiki`）可用 `--perm-type container|single_page` 控制是否包含子页面

## 4. 是否需要通知对方

- 默认不通知；添加 `--notification` 才会向被授权者发送飞书通知
- 用户没有明确要求时按需选择，不要擅自通知大范围成员（群、部门）

## 5. 权限级别选择

- `view`：只读分享（外部人员、大范围分享）
- `edit`：日常协作（团队成员）
- `full_access`：管理员（需要管理协作者或文档设置的场景）
