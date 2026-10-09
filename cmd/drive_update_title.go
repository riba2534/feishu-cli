package cmd

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// drive update-title 支持的资源类型（base 归一为 bitable）。
var driveUpdateTitleTypes = []string{"docx", "sheet", "bitable", "slides", "file", "folder", "wiki"}

// 服务端对这些类型直接返回 981002 params error，本地提前拒绝。
var driveUpdateTitleRejectedTypes = []string{"doc", "mindnote"}

const (
	driveUpdateTitleExtKeep  = "keep"
	driveUpdateTitleExtAllow = "allow"
)

var driveUpdateTitleCmd = &cobra.Command{
	Use:   "update-title",
	Short: "重命名云盘文件/文件夹/在线文档/wiki 节点",
	Long: `通过 PATCH /open-apis/drive/v1/files/{token}?type=<type> 修改标题，适用于普通文件、文件夹、
docx/sheet/bitable(base)/slides 在线文档和 wiki 节点（wiki 节点与文档标题保持同步）。

输入（二选一）:
  --url           目标的飞书 URL（推荐，自动识别类型）
  --token         目标 token 或 URL；裸 token 必须配合 --type

必填:
  --title         新标题（去掉首尾空白后不能为空）

可选:
  --type                    docx | sheet | bitable | base | slides | file | folder | wiki
                            （URL 输入时可省略，提供时必须与 URL 类型一致；doc/mindnote 服务端不支持）
  --on-extension-mismatch   仅 --type file：keep（默认）新标题不带扩展名时自动沿用原扩展名、
                            改变扩展名时拒绝；allow 原样提交且不读取当前文件名
  --as                      bot | user | auto（默认 auto：User 优先，未配置回退 Bot）
  --dry-run                 只打印将要发出的请求

注意:
  - --type 必须是资源的真实类型，类型不符与 token 不存在一样返回 981003
  - wiki 节点请传 /wiki/ URL 中的节点 token 并用 --type wiki，不接受底层文档 token
  - 981004 表示当前身份对目标没有编辑权限（不是缺 scope）；批量重命名请串行，避免 99991400 限流

示例:
  feishu-cli drive update-title --url https://xxx.feishu.cn/docx/doxcnXXX --title "季度复盘"
  feishu-cli drive update-title --token boxcnXXX --type file --title "报告-v2"   # 自动保留原扩展名
  feishu-cli drive update-title --token fldcnXXX --type folder --title "归档"
  feishu-cli drive update-title --url https://xxx.feishu.cn/wiki/wikcnXXX --title "新标题" --as user`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		urlInput, _ := cmd.Flags().GetString("url")
		tokenInput, _ := cmd.Flags().GetString("token")
		explicitType, _ := cmd.Flags().GetString("type")
		title, _ := cmd.Flags().GetString("title")
		extPolicy, _ := cmd.Flags().GetString("on-extension-mismatch")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		token, docType, err := resolveDriveUpdateTitleTarget(urlInput, tokenInput, explicitType)
		if err != nil {
			return err
		}
		title = strings.TrimSpace(title)
		if title == "" {
			return clierr.Usagef("--title 不能为空（接口接受空标题并会把资源标题清空，请传入期望的标题）")
		}
		extPolicy = strings.ToLower(strings.TrimSpace(extPolicy))
		if extPolicy != driveUpdateTitleExtKeep && extPolicy != driveUpdateTitleExtAllow {
			return clierr.Usagef("--on-extension-mismatch 只能是 keep 或 allow，得到 %q", extPolicy)
		}
		if cmd.Flags().Changed("on-extension-mismatch") && docType != "file" {
			return clierr.Usagef("--on-extension-mismatch 只适用于 --type file，当前类型 %q", docType)
		}
		if err := validateIdentityAs(cmd); err != nil {
			return err
		}
		needsCurrent := docType == "file" && extPolicy == driveUpdateTitleExtKeep

		if dryRun {
			plan := map[string]any{
				"dry_run": true,
				"method":  "PATCH",
				"path":    "/open-apis/drive/v1/files/" + token,
				"params":  map[string]string{"type": docType},
				"body":    map[string]string{"new_title": title},
			}
			if needsCurrent {
				plan["pre_request"] = map[string]any{
					"method": "POST",
					"path":   "/open-apis/drive/v1/metas/batch_query",
					"desc":   "读取当前文件名，用于 --on-extension-mismatch=keep 扩展名保护",
				}
				plan["body"] = map[string]string{"new_title": "<按当前文件名的扩展名解析后的 --title>"}
			}
			return printJSON(plan)
		}

		userToken, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		finalTitle := title
		previous, appended := "", ""
		if needsCurrent {
			current, err := client.FetchDocMetaTitle(token, "file", userToken)
			if err != nil {
				return fmt.Errorf("%w\n扩展名保护需要读取当前文件名（drive:drive.metadata:readonly）；如需跳过请加 --on-extension-mismatch allow", err)
			}
			if strings.TrimSpace(current) == "" {
				return fmt.Errorf("未读取到当前文件名，无法执行扩展名保护；可加 --on-extension-mismatch allow 原样提交")
			}
			previous = current
			curExt, newExt := titleExtension(current), titleExtension(title)
			switch {
			case curExt == "" || strings.EqualFold(curExt, newExt):
			case newExt == "":
				finalTitle = title + curExt
				appended = curExt
				fmt.Fprintf(os.Stderr, "新标题没有扩展名，沿用当前文件名的 %s: %q\n", curExt, finalTitle)
			default:
				return clierr.Usagef("--title %q 会把扩展名从 %s 改成 %s（当前文件名 %q）；如确需修改请加 --on-extension-mismatch allow", title, curExt, newExt, current)
			}
		}

		if err := client.UpdateDriveTitle(token, docType, finalTitle, userToken); err != nil {
			return withUpdateTitleHint(err, docType)
		}

		result := map[string]any{
			"updated":    true,
			"file_token": token,
			"type":       docType,
			"title":      finalTitle,
		}
		if u := client.BuildResourceURL(docType, token); u != "" {
			result["url"] = u
		}
		if previous != "" {
			result["previous_title"] = previous
		}
		if appended != "" {
			result["extension_appended"] = appended
		}
		if output == "json" {
			return printJSON(result)
		}
		fmt.Printf("标题已更新: %s\n", finalTitle)
		if previous != "" {
			fmt.Printf("  原标题: %s\n", previous)
		}
		fmt.Printf("  token:  %s (%s)\n", token, docType)
		return nil
	},
}

// resolveDriveUpdateTitleTarget 解析 --url/--token/--type，得到 (token, type)。
func resolveDriveUpdateTitleTarget(urlInput, tokenInput, explicitType string) (string, string, error) {
	urlInput, tokenInput = strings.TrimSpace(urlInput), strings.TrimSpace(tokenInput)
	if urlInput != "" && tokenInput != "" {
		return "", "", clierr.Usagef("--url 与 --token 互斥，只能传一个")
	}
	raw, argName := urlInput, "--url"
	if raw == "" {
		raw, argName = tokenInput, "--token"
	}
	if raw == "" {
		return "", "", clierr.Usagef("请指定 --url 或 --token")
	}
	explicit := client.NormalizeResourceType(strings.ToLower(strings.TrimSpace(explicitType)))
	for _, rejected := range driveUpdateTitleRejectedTypes {
		if explicit == rejected {
			return "", "", clierr.Usagef("标题接口不支持 type=%s（服务端返回 981002），请在飞书客户端中重命名；支持的类型: %s", explicit, strings.Join(driveUpdateTitleTypes, ", "))
		}
	}
	res, err := parseResourceArg(raw, resourceArgOptions{ArgName: argName, ExplicitType: explicit})
	if err != nil {
		if !client.LooksLikeURL(raw) && explicit == "" {
			return "", "", clierr.Usagef("%s 是裸 token 时必须指定 --type（%s）", argName, strings.Join(driveUpdateTitleTypes, ", "))
		}
		return "", "", clierr.Usage(err)
	}
	docType := client.NormalizeResourceType(res.Type)
	for _, rejected := range driveUpdateTitleRejectedTypes {
		if docType == rejected {
			return "", "", clierr.Usagef("标题接口不支持 type=%s（服务端返回 981002），请在飞书客户端中重命名", docType)
		}
	}
	supported := false
	for _, t := range driveUpdateTitleTypes {
		if docType == t {
			supported = true
		}
	}
	if !supported {
		return "", "", clierr.Usagef("不支持的资源类型 %q；drive update-title 支持 %s", docType, strings.Join(driveUpdateTitleTypes, ", "))
	}
	return res.Token, docType, nil
}

// titleExtension 取文件名的扩展名（保留原大小写）；只有前导点的 dotfile 不算扩展名。
func titleExtension(title string) string {
	base := path.Base(strings.TrimSpace(title))
	if strings.LastIndex(base, ".") <= 0 {
		return ""
	}
	return path.Ext(base)
}

func withUpdateTitleHint(err error, docType string) error {
	apiErr, ok := client.AsAPIError(err)
	if !ok {
		return err
	}
	hint := ""
	switch apiErr.Code {
	case 981003:
		if docType == "wiki" {
			hint = "--type wiki 需要 /wiki/ URL 中的节点 token，不接受底层文档 token"
		} else {
			hint = fmt.Sprintf("确认 token 存在且 --type %s 是其真实类型（可用 feishu-cli drive inspect 识别）", docType)
		}
	case 981004:
		hint = "这是文档权限不足（不是缺 scope）：当前身份需要目标的编辑权限，请先授权或换有权限的身份"
	case 99991400:
		hint = "重命名被限流，请稍后带退避重试；批量重命名请串行执行"
	}
	if hint == "" {
		return err
	}
	return fmt.Errorf("%w（%s）", err, hint)
}

func init() {
	driveCmd.AddCommand(driveUpdateTitleCmd)
	driveUpdateTitleCmd.Flags().String("url", "", "目标的飞书 URL（推荐）")
	driveUpdateTitleCmd.Flags().String("token", "", "目标 token 或 URL（裸 token 需配合 --type）")
	driveUpdateTitleCmd.Flags().String("type", "", "资源类型: docx/sheet/bitable(base)/slides/file/folder/wiki")
	driveUpdateTitleCmd.Flags().String("title", "", "新标题（必填）")
	driveUpdateTitleCmd.Flags().String("on-extension-mismatch", driveUpdateTitleExtKeep, "仅 --type file：keep 自动保留原扩展名并拒绝修改扩展名；allow 原样提交")
	driveUpdateTitleCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求")
	addAsFlag(driveUpdateTitleCmd)
	driveUpdateTitleCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	driveUpdateTitleCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	mustMarkFlagRequired(driveUpdateTitleCmd, "title")
}
