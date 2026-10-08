package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// permApplyTypes 是 /drive/v1/permissions/:token/members/apply 接口接受的 type 枚举。
// 支持的申请权限类型。
var permApplyTypes = []string{
	"doc", "sheet", "file", "wiki", "bitable", "docx", "mindnote", "slides",
}

// resolvePermApplyTarget 从 --token（可以是 token 或完整 URL）+ 可选 --type 推断 (token, type)。
// 只按 URL 路径前缀推断类型（query 中的 /wiki/ 等字样不会劫持解析）；
// --type 与 URL 推断的类型冲突时报错，不再静默以 --type 覆盖。wiki 是申请接口的合法 type，不做解包。
func resolvePermApplyTarget(raw, explicitType string) (token, docType string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", fmt.Errorf("--token 必填（可以是文档 token 或完整 URL）")
	}
	if !client.LooksLikeURL(raw) && strings.TrimSpace(explicitType) == "" {
		return "", "", fmt.Errorf("--type 必填（当 --token 是裸 token 时）。可选值: %s", strings.Join(permApplyTypes, ", "))
	}
	res, err := parseResourceArg(raw, resourceArgOptions{
		ArgName:      "--token",
		ExplicitType: explicitType,
		Allowed:      permApplyTypes,
	})
	if err != nil {
		return "", "", err
	}
	return res.Token, res.Type, nil
}

var drivePermApplyCmd = &cobra.Command{
	Use:   "apply-permission",
	Short: "以用户身份向文档所有者发起权限申请（view / edit）",
	Long: `向飞书云文档所有者发起协作权限申请。所有者会收到一张审批卡片，由其点同意/拒绝。

⚠️ 这是飞书的「埋藏 API」——官方文档站未收录，但服务端真实可用（已实测）。
端点: POST /open-apis/drive/v1/permissions/:token/members/apply
必需 scope: docs:permission.member:apply（或 drive:drive / docs:doc 等任一大权限）
必需 token: User Access Token（Bot 身份会被拒绝）

参数:
  --token      文档 token 或完整 URL（必填）
               支持 URL: /docx/、/sheets/、/base/、/bitable/、/file/、/wiki/、/doc/、/mindnote/、/slides/
  --type       文档类型（裸 token 时必填；传 URL 时按路径推断，与 --type 冲突会报错）
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

		apiPath := fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members/apply", url.PathEscape(realToken))

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

		// 先解析业务信封再看 HTTP 状态：飞书业务错误常随 HTTP 400 下发，
		// HTTP 200 + code!=0 也必须判失败（过去会打印响应后 exit 0）。
		if err := client.CheckAPIResponse("申请权限", resp); err != nil {
			return decoratePermApplyError(err)
		}

		// 成功：直接打印响应（含 code/msg/data）
		fmt.Println(string(resp.RawBody))
		return nil
	},
}

// decoratePermApplyError 为申请权限的典型业务码追加中文指引（对齐官方 permApplyErrorGuidance），保留原错误链。
func decoratePermApplyError(err error) error {
	if guidance := permApplyErrorGuidance(err); guidance != "" {
		return fmt.Errorf("%w\n提示：%s", err, guidance)
	}
	return err
}

func permApplyErrorGuidance(err error) string {
	switch {
	case client.HasAPICode(err, 1063006):
		return "已达到申请次数上限：同一用户对同一文档每天最多申请 5 次，请等次日额度重置后再试"
	case client.HasAPICode(err, 1063007):
		return "该文档不接受权限申请（可能已关闭申请入口或申请的权限不适用），请核对目标文档与申请的权限，或直接联系文档所有者"
	}
	return ""
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
