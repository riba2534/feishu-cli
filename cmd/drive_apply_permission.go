package cmd

import (
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// permApplyTypes 是 /drive/v1/permissions/:token/members/apply 接口接受的 type 枚举。
// 与官方 lark-cli `shortcuts/drive/drive_apply_permission.go` permApplyTypes 对齐。
var permApplyTypes = []string{
	"doc", "sheet", "file", "wiki", "bitable", "docx", "mindnote", "slides",
}

// permApplyURLMarkers 文档 URL 路径片段 → API 接受的 type 值映射
// 参考 lark-cli `shortcuts/drive/drive_apply_permission.go` permApplyURLMarkers
var permApplyURLMarkers = []struct {
	Marker string
	Type   string
}{
	{"/wiki/", "wiki"},
	{"/docx/", "docx"},
	{"/sheets/", "sheet"},
	{"/base/", "bitable"},
	{"/bitable/", "bitable"},
	{"/file/", "file"},
	{"/mindnote/", "mindnote"},
	{"/slides/", "slides"},
	{"/doc/", "doc"},
}

// resolvePermApplyTarget 从 --token（可以是 token 或完整 URL）+ 可选 --type 推断 (token, type)
// 显式 --type 优先于 URL 推断
func resolvePermApplyTarget(raw, explicitType string) (token, docType string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("--token 必填（可以是文档 token 或完整 URL）")
	}

	if strings.Contains(raw, "://") {
		// 完整 URL → 抽 token + 推断 type
		for _, m := range permApplyURLMarkers {
			if idx := strings.Index(raw, m.Marker); idx >= 0 {
				rest := raw[idx+len(m.Marker):]
				// 截掉 ? # / 后的内容
				for _, sep := range []string{"?", "#", "/"} {
					if i := strings.Index(rest, sep); i >= 0 {
						rest = rest[:i]
					}
				}
				if rest != "" {
					token = rest
					if explicitType == "" {
						docType = m.Type
					}
					break
				}
			}
		}
		if token == "" {
			return "", "", fmt.Errorf("无法从 URL 推断 token: %q\n支持的 URL 模式: /docx/、/sheets/、/base/、/bitable/、/file/、/wiki/、/doc/、/mindnote/、/slides/\n如果 URL 格式不常见，请用 --token 直接传 token + --type 指定类型", raw)
		}
	} else {
		token = raw
	}

	if explicitType != "" {
		docType = explicitType
	}
	if docType == "" {
		return "", "", fmt.Errorf("--type 必填（当 --token 是裸 token 时）。可选值: %s",
			strings.Join(permApplyTypes, ", "))
	}
	return token, docType, nil
}

var drivePermApplyCmd = &cobra.Command{
	Use:   "apply-permission",
	Short: "以用户身份向文档所有者发起权限申请（view / edit）",
	Long: `向飞书云文档所有者发起协作权限申请。所有者会收到一张审批卡片，由其点同意/拒绝。

⚠️ 这是飞书的「埋藏 API」—— 官方文档站未收录，但 lark-cli 已实现。
端点: POST /open-apis/drive/v1/permissions/:token/members/apply
必需 scope: docs:permission.member:apply（或 drive:drive / docs:doc 等任一大权限）
必需 token: User Access Token（Bot 身份会被拒绝）

参数:
  --token      文档 token 或完整 URL（必填）
               支持 URL: /docx/、/sheets/、/base/、/bitable/、/file/、/wiki/、/doc/、/mindnote/、/slides/
  --type       文档类型（URL 推断不出来时必填）
               可选: doc / sheet / file / wiki / bitable / docx / mindnote / slides
  --perm       申请权限（必填）: view / edit
  --remark     申请说明（可选，会显示在发给所有者的审批卡片上）
  --dry-run    仅打印将要发出的请求，不实际申请

示例:
  # 用 URL 直接申请（自动推断 type=docx）
  feishu-cli drive apply-permission --token "https://xxx.feishu.cn/docx/doxcnxxx" --perm view --remark "调研需要"

  # 用裸 token 申请
  feishu-cli drive apply-permission --token doxcnxxx --type docx --perm edit --remark "..."

  # 预览请求
  feishu-cli drive apply-permission --token <url> --perm view --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, _ := cmd.Flags().GetString("token")
		explicitType, _ := cmd.Flags().GetString("type")
		perm, _ := cmd.Flags().GetString("perm")
		remark, _ := cmd.Flags().GetString("remark")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if err := validateEnum(perm, "perm", []string{"view", "edit"}); err != nil {
			return err
		}

		realToken, docType, err := resolvePermApplyTarget(token, explicitType)
		if err != nil {
			return err
		}
		if err := validateEnum(docType, "type", permApplyTypes); err != nil {
			return err
		}

		userToken, err := requireUserToken(cmd, "drive apply-permission")
		if err != nil {
			return err
		}

		body := map[string]any{"perm": perm}
		if remark != "" {
			body["remark"] = remark
		}

		apiPath := fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members/apply", realToken)

		if dryRun {
			return printJSON(map[string]any{
				"dry_run": true,
				"method":  "POST",
				"path":    apiPath,
				"query":   map[string]string{"type": docType},
				"body":    body,
			})
		}

		cli, err := client.GetClient()
		if err != nil {
			return err
		}

		req := &larkcore.ApiReq{
			HttpMethod:  http.MethodPost,
			ApiPath:     apiPath,
			QueryParams: larkcore.QueryParams{},
			Body:        body,
			SupportedAccessTokenTypes: []larkcore.AccessTokenType{
				larkcore.AccessTokenTypeUser,
			},
		}
		req.QueryParams.Set("type", docType)

		fmt.Fprintf(cmd.ErrOrStderr(), "向所有者申请 %s 权限（%s %s）...\n", perm, docType, realToken)

		resp, err := cli.Do(client.Context(), req, larkcore.WithUserAccessToken(userToken))
		if err != nil {
			return fmt.Errorf("申请权限失败: %w", err)
		}

		// 直接打印响应（含 code/msg/data）
		fmt.Println(string(resp.RawBody))

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return nil
	},
}

func init() {
	driveCmd.AddCommand(drivePermApplyCmd)
	drivePermApplyCmd.Flags().String("token", "", "文档 token 或完整 URL（必填）")
	drivePermApplyCmd.Flags().String("type", "", "文档类型: "+strings.Join(permApplyTypes, "/"))
	drivePermApplyCmd.Flags().String("perm", "view", "申请权限: view / edit")
	drivePermApplyCmd.Flags().String("remark", "", "申请说明（可选）")
	drivePermApplyCmd.Flags().Bool("dry-run", false, "仅打印请求，不实际申请")
	drivePermApplyCmd.Flags().String("user-access-token", "", "User Access Token（必需）")
	mustMarkFlagRequired(drivePermApplyCmd, "token")
}
