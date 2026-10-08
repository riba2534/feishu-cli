package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// driveInspectTypes 是 inspect 接受的文档类型枚举
var driveInspectTypes = []string{
	"doc", "docx", "sheet", "bitable", "wiki", "file", "folder", "mindnote", "slides",
}

// driveAPICall 内部 helper：用当前 token 发 raw API 调用
func driveAPICall(method, path string, query map[string]string, body any, userToken string) ([]byte, int, error) {
	cli, err := client.GetClient()
	if err != nil {
		return nil, 0, err
	}
	q := larkcore.QueryParams{}
	for k, v := range query {
		q.Set(k, v)
	}
	req := &larkcore.ApiReq{
		HttpMethod:  method,
		ApiPath:     path,
		QueryParams: q,
		Body:        body,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{
			larkcore.AccessTokenTypeTenant,
			larkcore.AccessTokenTypeUser,
		},
	}
	var opts []larkcore.RequestOptionFunc
	if userToken != "" {
		opts = append(opts, larkcore.WithUserAccessToken(userToken))
	}
	resp, err := cli.Do(client.Context(), req, opts...)
	if err != nil {
		return nil, 0, err
	}
	return resp.RawBody, resp.StatusCode, nil
}

// inspectFetchTitle 调 drive/v1/metas/batch_query 拿 title
func inspectFetchTitle(docToken, docType, userToken string) (title string, err error) {
	body, status, err := driveAPICall(http.MethodPost,
		"/open-apis/drive/v1/metas/batch_query",
		nil,
		map[string]any{
			"request_docs": []map[string]string{
				{"doc_token": docToken, "doc_type": docType},
			},
		},
		userToken)
	if err != nil {
		return "", fmt.Errorf("metas/batch_query 失败: %w", err)
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("metas/batch_query HTTP %d: %s", status, string(body))
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Metas []struct {
				Title    string `json:"title"`
				DocToken string `json:"doc_token"`
				DocType  string `json:"doc_type"`
				OwnerID  string `json:"owner_id"`
				URL      string `json:"url"`
			} `json:"metas"`
			FailedList []struct {
				Token string `json:"token"`
				Code  int    `json:"code"`
			} `json:"failed_list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析 metas/batch_query 响应失败: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("metas/batch_query code=%d msg=%s", resp.Code, resp.Msg)
	}
	if len(resp.Data.FailedList) > 0 {
		return "", fmt.Errorf("文档不可见（可能无权限或不存在）: code=%d token=%s",
			resp.Data.FailedList[0].Code, resp.Data.FailedList[0].Token)
	}
	if len(resp.Data.Metas) == 0 {
		return "", fmt.Errorf("metas/batch_query 返回空 metas")
	}
	return resp.Data.Metas[0].Title, nil
}

var driveInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "解析文档 URL → 输出 type/title/token/canonical URL（自动展开 wiki 节点）",
	Long: `给定文档 URL 或裸 token，统一输出 type / title / token / canonical URL。

特别功能：
  - URL 只按路径前缀推断类型（?from=/wiki/... 等查询参数不会劫持解析）；--type 与 URL 冲突时报错
  - URL 中带 /wiki/ 时，自动调 wiki node_by_token 拆出底层文档的 obj_type/obj_token
  - 裸 token 未传 --type 时，自动调 drive query_by_token 识别类型（wiki 节点同样自动展开）
  - 自动检测无权限、不存在等异常
  - 默认 auto Token（User 优先，回退 Bot）

参数:
  --url           文档 URL 或裸 token（必填）
  --type          文档类型（可选；URL 自动推断，裸 token 省略时通过 query_by_token 自动识别）
                  可选: doc / docx / sheet / bitable / wiki / file / folder / mindnote / slides
  --output, -o    输出格式 (json)

示例:
  # 解析 docx URL
  feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"

  # 解析 wiki URL（自动展开到底层文档）
  feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx"

  # 裸 token（自动识别类型）/ 裸 token + type
  feishu-cli drive inspect --url doxcnxxx
  feishu-cli drive inspect --url doxcnxxx --type docx

  # JSON 输出
  feishu-cli drive inspect --url <url> -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		rawURL, _ := cmd.Flags().GetString("url")
		explicitType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")

		// 统一解析：只按 URL 路径前缀推断类型；--type 与 URL 冲突时报错（wiki URL + 非 wiki 类型视为对底层类型的断言）
		opts := resourceArgOptions{
			ArgName:        "--url",
			ExplicitType:   explicitType,
			Allowed:        driveInspectTypes,
			ResolveWiki:    true,
			DetectBareType: true, // 裸 token 未传 --type 时用 query_by_token 自动识别
		}
		res, err := parseResourceArg(rawURL, opts)
		if err != nil {
			return err
		}

		// auto token：User 优先，回退 Bot
		userToken := resolveOptionalUserTokenWithFallback(cmd)
		opts.UserAccessToken = userToken

		// Step 0: 裸 token 未指定 --type 时，通过 query_by_token 识别真实类型（wiki 节点自动识别）
		if res.InputType == "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "未指定 --type，通过 query_by_token 识别类型: %s ...\n", res.InputToken)
			if err := detectResourceType(res, opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "识别为 %s\n", res.InputType)
		}

		result := map[string]any{
			"input_url": strings.TrimSpace(rawURL),
			"type":      res.InputType,
			"token":     res.InputToken,
		}
		if res.DetectedBy != "" {
			result["detected_by"] = res.DetectedBy
			if res.TokenStatus != 0 {
				result["token_status"] = res.TokenStatus
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ 该节点状态异常（status=%d：1=在回收站，2=已删除）\n", res.TokenStatus)
			}
		}

		// Step 1: 如果是 wiki，先通过 node_by_token 展开
		if res.InputType == client.ResourceTypeWiki {
			fmt.Fprintf(cmd.ErrOrStderr(), "Wiki 节点展开中: %s ...\n", res.InputToken)
			if err := resolveWikiInResource(res, opts); err != nil {
				return err
			}
			result["wiki_node"] = map[string]string{
				"space_id":   res.WikiNode.SpaceID,
				"node_token": res.WikiNode.NodeToken,
				"obj_type":   res.Type,
				"obj_token":  res.Token,
			}
			result["type"] = res.Type
			result["token"] = res.Token
			fmt.Fprintf(cmd.ErrOrStderr(), "Wiki 已展开为 %s: %s\n", res.Type, res.Token)
		}
		docType, docToken := res.Type, res.Token

		// Step 2: 查 title（除了 folder 类型，folder 没 title API）
		if docType != "folder" {
			title, err := inspectFetchTitle(docToken, docType, userToken)
			if err != nil {
				// title 拿不到不致命，作为 warning
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ 获取 title 失败: %v\n", err)
				result["title_error"] = err.Error()
			} else {
				result["title"] = title
			}
		}

		if output == "json" {
			return printJSON(result)
		}

		// 文本输出
		fmt.Printf("Type:  %s\n", docType)
		if title, ok := result["title"].(string); ok && title != "" {
			fmt.Printf("Title: %s\n", title)
		}
		fmt.Printf("Token: %s\n", docToken)
		if wn, ok := result["wiki_node"].(map[string]string); ok {
			fmt.Printf("Wiki:  space_id=%s, node_token=%s\n", wn["space_id"], wn["node_token"])
		}
		return nil
	},
}

func init() {
	driveCmd.AddCommand(driveInspectCmd)
	driveInspectCmd.Flags().String("url", "", "文档 URL 或裸 token（必填）")
	driveInspectCmd.Flags().String("type", "", "文档类型（可选；URL 按路径推断，裸 token 省略时自动识别）")
	driveInspectCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	driveInspectCmd.Flags().String("user-access-token", "", "User Access Token（auto 时可选）")
	mustMarkFlagRequired(driveInspectCmd, "url")
}
