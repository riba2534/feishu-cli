package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

// appsIconMaxBytes --icon-file 大小上限（对齐官方 appIconMaxBytes，2MB）。
var appsIconMaxBytes int64 = 2 * 1024 * 1024

// appsIconUnsupportedCode 应用类型不支持改图标时服务端返回的业务码（对齐官方 appIconUnsupportedCode）。
const appsIconUnsupportedCode = 40002

var appsIconContentTypes = map[string]bool{"image/png": true, "image/jpeg": true}

var appsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "修改妙搭应用的名称 / 描述 / 图标（部分更新）",
	Long: `部分更新一个妙搭（Miaoda）应用，只发送提供的字段（--name / --description / --icon-url / --icon-file 至少一个）。

图标:
  --icon-url    http(s) 图片 URL（必须带主机名，不能含 user:password@）
  --icon-file   本地 png / jpeg 文件（≤2MB，按文件内容识别类型）；先上传拿到 icon_url 再写入
  两者互斥。改图标时先 GET 应用确认类型支持改图标，避免上传后才失败留下孤儿图标。

--dry-run 只做本地校验（含读取 --icon-file 检查类型与大小）并打印计划，不获取 token、不访问网络。

权限: User Access Token + spark:app:write（改图标还需 spark:app:read）

示例:
  feishu-cli apps update --app-id app_xxx --name "新名字"
  feishu-cli apps update --app-id app_xxx --description "更新后的描述"
  feishu-cli apps update --app-id app_xxx --icon-url https://example.com/icon.png
  feishu-cli apps update --app-id app_xxx --icon-file ./icon.png --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		appID := strings.TrimSpace(flagString(cmd, "app-id"))
		if appID == "" {
			return clierr.Usagef("--app-id 不能为空")
		}

		body := map[string]any{}
		if v := strings.TrimSpace(flagString(cmd, "name")); v != "" {
			body["name"] = v
		}
		if v := strings.TrimSpace(flagString(cmd, "description")); v != "" {
			body["description"] = v
		}
		iconURL := strings.TrimSpace(flagString(cmd, "icon-url"))
		iconFile := strings.TrimSpace(flagString(cmd, "icon-file"))
		if iconURL != "" && iconFile != "" {
			return clierr.Usagef("--icon-url 与 --icon-file 互斥，只能提供一个")
		}
		if len(body) == 0 && iconURL == "" && iconFile == "" {
			return clierr.Usagef("至少提供 --name、--description、--icon-url 或 --icon-file 之一")
		}
		if iconURL != "" {
			if err := validateAppsIconURL(iconURL); err != nil {
				return err
			}
			body["icon_url"] = iconURL
		}
		var iconContent []byte
		if iconFile != "" {
			content, err := readAppsIconFile(iconFile)
			if err != nil {
				return err
			}
			iconContent = content
		}
		changesIcon := iconURL != "" || iconFile != ""

		path := appsAppPath(appID, "")
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			if !changesIcon {
				return appsDryRun(cmd, "PATCH", path, nil, body)
			}
			return appsUpdateIconDryRun(cmd, path, body, iconFile)
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "apps update")
		if err != nil {
			return err
		}
		if changesIcon {
			// 预检：确认应用存在且类型支持改图标，再上传（避免孤儿图标）
			if _, err := client.SparkCall("GET", path, nil, nil, token); err != nil {
				return appsWithIconHint(err)
			}
		}
		if iconContent != nil {
			uploaded, err := client.SparkUploadIcon(iconContent, token)
			if err != nil {
				return appsWithIconHint(err)
			}
			body["icon_url"] = uploaded
		}
		data, err := client.SparkCall("PATCH", path, nil, body, token)
		if err != nil {
			if changesIcon {
				return appsWithIconHint(err)
			}
			return err
		}
		if want, ok := body["icon_url"].(string); ok {
			app, _ := data["app"].(map[string]any)
			if got := sparkStringValue(app["icon_url"]); got != want {
				fmt.Fprintf(os.Stderr, "警告：更新响应中的 icon_url（%q）与发送的（%q）不一致；用 `feishu-cli apps get --app-id %s` 核对\n", got, want, appID)
			}
		}
		return renderAppsResult(cmd, data)
	},
}

// appsUpdateIconDryRun 改图标时的多步计划：GET 预检 →（--icon-file 时）multipart 上传 → PATCH。
func appsUpdateIconDryRun(cmd *cobra.Command, path string, body map[string]any, iconFile string) error {
	o, err := output.ParseOptions(cmd)
	if err != nil {
		return err
	}
	patchBody := cloneAnyMap(body)
	steps := []map[string]any{{
		"method": "GET",
		"path":   path,
		"desc":   "预检：确认应用类型支持改图标",
	}}
	if iconFile != "" {
		steps = append(steps, map[string]any{
			"method": "POST",
			"path":   client.SparkIconUploadPath(),
			"desc":   "multipart 上传 --icon-file（字段 file）",
			"body":   map[string]any{"file": "<contents of " + iconFile + ">"},
		})
		patchBody["icon_url"] = "<icon_url from upload response>"
	}
	steps = append(steps, map[string]any{"method": "PATCH", "path": path, "body": patchBody})
	return output.Render(o, map[string]any{"dry_run": true, "steps": steps})
}

// validateAppsIconURL 要求绝对 http(s) URL、带主机名、不含 userinfo（对齐官方 validateAppIconURL）。
func validateAppsIconURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return clierr.Usagef("--icon-url 必须是带主机名的绝对 http(s) URL，得到 %q\n提示：传可公开访问的图片 URL，或用 --icon-file 上传本地 png/jpeg", raw)
	}
	if u.User != nil {
		return clierr.Usagef("--icon-url 不能包含 user:password@ 部分；图标 URL 必须无需凭证即可公开访问")
	}
	return nil
}

// readAppsIconFile 读取并校验 --icon-file：路径安全、≤2MB、内容识别为 png / jpeg。
func readAppsIconFile(path string) ([]byte, error) {
	info, err := safefile.StatInputFile(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > appsIconMaxBytes {
		return nil, clierr.Usagef("--icon-file 超过 2MB 上限（文件 %d 字节）", info.Size())
	}
	content, err := readLocalInputFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > appsIconMaxBytes {
		return nil, clierr.Usagef("--icon-file 超过 2MB 上限（文件 %d 字节）", len(content))
	}
	if ct := http.DetectContentType(content); !appsIconContentTypes[ct] {
		return nil, clierr.Usagef("--icon-file 必须是 png 或 jpeg 图片（按文件内容识别为 %s）", ct)
	}
	return content, nil
}

// appsWithIconHint 改图标失败时的提示：40002 表示该应用类型不能用 CLI 改图标。
func appsWithIconHint(err error) error {
	if client.HasAPICode(err, appsIconUnsupportedCode) {
		return appsWithHint(err, "该应用类型（如 OpenClaw 应用）不能通过 CLI 修改图标，请在妙搭编辑器中修改")
	}
	return appsWithHint(err, appsAppIDListHint)
}

func init() {
	appsCmd.AddCommand(appsUpdateCmd)
	appsUpdateCmd.Flags().String("app-id", "", "妙搭应用 ID（必填）")
	appsUpdateCmd.Flags().String("name", "", "新的应用显示名称")
	appsUpdateCmd.Flags().String("description", "", "新的应用描述")
	appsUpdateCmd.Flags().String("icon-url", "", "新的应用图标 http(s) URL（与 --icon-file 互斥）")
	appsUpdateCmd.Flags().String("icon-file", "", "新的应用图标本地 png/jpeg 文件（≤2MB，先上传再写入；与 --icon-url 互斥）")
	addAppsWriteFlags(appsUpdateCmd)
}
