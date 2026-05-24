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

// driveInspectURLMarkers 文档 URL 路径片段 → type 映射（与 apply-permission 复用同一套）
// 加上 /folder/ 这种 drive-only 的标记
var driveInspectURLMarkers = []struct {
	Marker string
	Type   string
}{
	{"/wiki/", "wiki"},
	{"/docx/", "docx"},
	{"/sheets/", "sheet"},
	{"/base/", "bitable"},
	{"/bitable/", "bitable"},
	{"/file/", "file"},
	{"/folder/", "folder"},
	{"/mindnote/", "mindnote"},
	{"/slides/", "slides"},
	{"/doc/", "doc"},
}

// parseDriveURL 从 URL 抽 (type, token)；非 URL 时返回 (explicitType, raw)
func parseDriveURL(raw, explicitType string) (docType, token string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("--url 不能为空")
	}

	if strings.Contains(raw, "://") {
		for _, m := range driveInspectURLMarkers {
			if idx := strings.Index(raw, m.Marker); idx >= 0 {
				rest := raw[idx+len(m.Marker):]
				for _, sep := range []string{"?", "#", "/"} {
					if i := strings.Index(rest, sep); i >= 0 {
						rest = rest[:i]
					}
				}
				if rest != "" {
					token = rest
					docType = m.Type
					break
				}
			}
		}
		if token == "" {
			return "", "", fmt.Errorf("无法从 URL 推断 token: %q\n支持的 URL: /docx/、/sheets/、/base/、/file/、/folder/、/wiki/、/doc/、/mindnote/、/slides/", raw)
		}
		if explicitType != "" {
			docType = explicitType
		}
	} else {
		// 裸 token 必须传 --type
		if explicitType == "" {
			return "", "", fmt.Errorf("--type 必填（当 --url 是裸 token 时）。可选: %s",
				strings.Join(driveInspectTypes, ", "))
		}
		token = raw
		docType = explicitType
	}
	return docType, token, nil
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

// inspectFetchWikiNode 调 wiki/v2/spaces/get_node 拆出 obj_type/obj_token
func inspectFetchWikiNode(token, userToken string) (objType, objToken, spaceID, nodeToken string, err error) {
	body, status, err := driveAPICall(http.MethodGet,
		"/open-apis/wiki/v2/spaces/get_node",
		map[string]string{"token": token, "obj_type": "wiki"},
		nil, userToken)
	if err != nil {
		return "", "", "", "", fmt.Errorf("wiki get_node 失败: %w", err)
	}
	if status < 200 || status >= 300 {
		return "", "", "", "", fmt.Errorf("wiki get_node HTTP %d: %s", status, string(body))
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Node struct {
				ObjType   string `json:"obj_type"`
				ObjToken  string `json:"obj_token"`
				SpaceID   string `json:"space_id"`
				NodeToken string `json:"node_token"`
			} `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", "", "", fmt.Errorf("解析 wiki get_node 响应失败: %w", err)
	}
	if resp.Code != 0 {
		return "", "", "", "", fmt.Errorf("wiki get_node code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data.Node.ObjToken == "" {
		return "", "", "", "", fmt.Errorf("wiki get_node 返回 obj_token 为空")
	}
	return resp.Data.Node.ObjType, resp.Data.Node.ObjToken, resp.Data.Node.SpaceID, resp.Data.Node.NodeToken, nil
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
	Long: `给定文档 URL 或裸 token + type，统一输出 type / title / token / canonical URL。

特别功能：
  - URL 中带 /wiki/ 时，自动调 wiki get_node 拆出底层文档的 obj_type/obj_token
  - 自动检测无权限、不存在等异常
  - 默认 auto Token（User 优先，回退 Bot）

参数:
  --url           文档 URL 或裸 token（必填）
  --type          文档类型（裸 token 必填；URL 自动推断）
                  可选: doc / docx / sheet / bitable / wiki / file / folder / mindnote / slides
  --output, -o    输出格式 (json)

示例:
  # 解析 docx URL
  feishu-cli drive inspect --url "https://xxx.feishu.cn/docx/doxcnxxx"

  # 解析 wiki URL（自动展开到底层文档）
  feishu-cli drive inspect --url "https://xxx.feishu.cn/wiki/wikcnxxx"

  # 裸 token + type
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

		docType, docToken, err := parseDriveURL(rawURL, explicitType)
		if err != nil {
			return err
		}

		// auto token：User 优先，回退 Bot
		userToken := resolveOptionalUserTokenWithFallback(cmd)

		result := map[string]any{
			"input_url": rawURL,
			"type":      docType,
			"token":     docToken,
		}

		// Step 1: 如果是 wiki，先展开
		if docType == "wiki" {
			fmt.Fprintf(cmd.ErrOrStderr(), "Wiki 节点展开中: %s ...\n", docToken)
			objType, objToken, spaceID, nodeToken, err := inspectFetchWikiNode(docToken, userToken)
			if err != nil {
				return err
			}
			result["wiki_node"] = map[string]string{
				"space_id":   spaceID,
				"node_token": nodeToken,
				"obj_type":   objType,
				"obj_token":  objToken,
			}
			docType = objType
			docToken = objToken
			result["type"] = docType
			result["token"] = docToken
			fmt.Fprintf(cmd.ErrOrStderr(), "Wiki 已展开为 %s: %s\n", docType, docToken)
		}

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
	driveInspectCmd.Flags().String("type", "", "文档类型（裸 token 必填；URL 可自动推断）")
	driveInspectCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	driveInspectCmd.Flags().String("user-access-token", "", "User Access Token（auto 时可选）")
	mustMarkFlagRequired(driveInspectCmd, "url")
}
