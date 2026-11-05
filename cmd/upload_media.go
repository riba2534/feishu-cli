package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
)

var uploadMediaCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "上传素材文件",
	Long: `上传素材文件（图片、视频等）到飞书云空间。

参数:
  --parent-type   父节点类型（默认: docx_image）
  --parent-node   父节点 token，即文档 ID（必填）
  --name          文件名（默认使用原文件名）
  --doc-id        素材所属文档 ID 或 URL（可选；写入 extra 的 drive_route_token，素材按该文档路由鉴权，
                  上传到文档块下的图片/附件建议携带；/wiki/ URL 以 App 身份解析为底层 docx）
  --output, -o    输出格式（json）
  --dry-run       只打印将要发出的上传请求，不上传。--doc-id 为裸 token 或 /docx/ 链接时不读取凭据、不联网；
                  为 /wiki/ 链接时需要 App 凭据，先只读解析 wiki 节点再输出计划

父节点类型:
  docx_image     新版文档图片（推荐）
  docx_file      新版文档文件
  doc_image      旧版文档图片
  doc_file       旧版文档文件

示例:
  # 上传图片到文档
  feishu-cli media upload image.png --parent-type docx_image --parent-node DOC_ID

  # 上传文件
  feishu-cli media upload document.pdf --parent-type docx_file --parent-node DOC_ID

  # 指定文件名
  feishu-cli media upload photo.jpg --parent-node DOC_ID --name "封面图.jpg"

  # 上传到文档图片块下，并按文档路由鉴权
  feishu-cli media upload image.png --parent-type docx_image --parent-node BLOCK_ID --doc-id DOC_ID

  # 预览请求（不上传）
  feishu-cli media upload image.png --parent-node DOC_ID --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		parentType, _ := cmd.Flags().GetString("parent-type")
		parentNode, _ := cmd.Flags().GetString("parent-node")
		fileName, _ := cmd.Flags().GetString("name")
		docID, _ := cmd.Flags().GetString("doc-id")

		if parentType == "" {
			parentType = "docx_image"
		}

		if parentNode == "" {
			return clierr.Usagef("必须指定 --parent-node（文档ID）")
		}

		if fileName == "" {
			fileName = filepath.Base(filePath)
		}
		// 本地文件在任何网络请求之前校验：敏感目录、不存在、是目录、无权限读取均为用法错误
		stat, err := safefile.StatInputFile(filePath)
		if err != nil {
			return err
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// --doc-id 离线预检：裸 token / docx URL 直接得到路由 token；只有 wiki URL 需要联网解包
		routeToken, needWikiResolve := "", false
		if cmd.Flags().Changed("doc-id") {
			routeToken, needWikiResolve, err = parseMediaRouteDocIDOffline(docID)
			if err != nil {
				return err
			}
		}
		if dryRun && !needWikiResolve {
			return printMediaUploadDryRun(cmd, filePath, fileName, parentType, parentNode, routeToken, stat.Size())
		}

		if err := config.Validate(); err != nil {
			return err
		}

		extra := ""
		if cmd.Flags().Changed("doc-id") {
			if needWikiResolve {
				routeToken, err = resolveMediaRouteDocID(docID)
				if err != nil {
					return err
				}
			}
			b, _ := json.Marshal(map[string]string{"drive_route_token": routeToken})
			extra = string(b)
		}
		if dryRun {
			// 计划预览：wiki 已以 App 身份只读解包，不上传
			return printMediaUploadDryRun(cmd, filePath, fileName, parentType, parentNode, routeToken, stat.Size())
		}

		var token string
		if extra == "" {
			token, _, err = client.UploadMedia(filePath, parentType, parentNode, fileName)
		} else {
			token, _, err = client.UploadMediaWithExtra(filePath, parentType, parentNode, fileName, extra)
		}
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(map[string]string{
				"file_token": token,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("上传成功！\n")
			fmt.Printf("  文件 Token: %s\n", token)
		}

		return nil
	},
}

// parseMediaRouteDocIDOffline 离线解析 --doc-id：裸 token 与 docx URL 直接返回路由 token；
// wiki URL 返回 needWiki=true，由 resolveMediaRouteDocID 联网解包。格式错误一律用法错误。
func parseMediaRouteDocIDOffline(raw string) (token string, needWiki bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, clierr.Usagef("--doc-id 不能为空")
	}
	if !client.LooksLikeURL(raw) {
		if !client.IsSafeResourceToken(raw) {
			return "", false, clierr.Usagef("--doc-id 不是有效的文档 ID（只允许字母、数字、_ 和 -，长度 1-128）: %q", raw)
		}
		return raw, false, nil
	}
	res, err := parseResourceArg(raw, resourceArgOptions{
		ArgName:     "--doc-id",
		DefaultType: client.ResourceTypeDocx,
		Allowed:     []string{client.ResourceTypeDocx},
		ResolveWiki: true,
	})
	if err != nil {
		return "", false, clierr.Usage(err)
	}
	if res.InputType == client.ResourceTypeWiki {
		return "", true, nil
	}
	return res.Token, false, nil
}

// printMediaUploadDryRun 打印 medias/upload_all 预览（与 client.UploadMediaWithExtra 的请求一致）。
func printMediaUploadDryRun(cmd *cobra.Command, filePath, fileName, parentType, parentNode, routeToken string, size int64) error {
	extra := ""
	if routeToken != "" {
		b, _ := json.Marshal(map[string]string{"drive_route_token": routeToken})
		extra = string(b)
	}
	return printDryRunPlan(cmd, "Upload media file", map[string]any{
		"file":       filePath,
		"size_bytes": size,
	}, []dryRunStep{mediaUploadAllStep(filePath, fileName, parentType, parentNode, extra, size)})
}

// resolveMediaRouteDocID 解析 --doc-id：裸 token 原样使用（与官方一致，不限文档类型）；
// URL 按 docx 解析（/wiki/ URL 以 App 身份换出底层 docx，与本命令的上传身份一致）。
func resolveMediaRouteDocID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", clierr.Usagef("--doc-id 不能为空")
	}
	if !client.LooksLikeURL(raw) {
		if !client.IsSafeResourceToken(raw) {
			return "", clierr.Usagef("--doc-id 不是有效的文档 ID（只允许字母、数字、_ 和 -，长度 1-128）: %q", raw)
		}
		return raw, nil
	}
	if _, err := parseResourceArg(raw, resourceArgOptions{
		ArgName:     "--doc-id",
		DefaultType: client.ResourceTypeDocx,
		Allowed:     []string{client.ResourceTypeDocx},
		ResolveWiki: true,
	}); err != nil {
		return "", clierr.Usage(err)
	}
	return resolveDocxArg(raw, "--doc-id", "")
}

func init() {
	mediaCmd.AddCommand(uploadMediaCmd)
	uploadMediaCmd.Flags().String("parent-type", "docx_image", "父节点类型（docx_image/docx_file/doc_image/doc_file）")
	uploadMediaCmd.Flags().String("parent-node", "", "父节点 token（文档ID）")
	uploadMediaCmd.Flags().Bool("dry-run", false, "只打印将要发出的请求（--doc-id 为 wiki 链接时需凭据做只读解析）")
	uploadMediaCmd.Flags().String("name", "", "文件名（默认使用原文件名）")
	uploadMediaCmd.Flags().String("doc-id", "", "素材所属文档 ID 或 URL（写入 extra.drive_route_token）")
	uploadMediaCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mustMarkFlagRequired(uploadMediaCmd, "parent-node")
}
