package cmd

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var validWikiNodeDeleteObjTypes = map[string]bool{
	"wiki":     true,
	"doc":      true,
	"docx":     true,
	"sheet":    true,
	"bitable":  true,
	"mindnote": true,
	"slides":   true,
	"file":     true,
}

var wikiURLMarkers = []struct {
	Marker  string
	ObjType string
}{
	{"/wiki/", "wiki"},
	{"/docx/", "docx"},
	{"/sheets/", "sheet"},
	{"/base/", "bitable"},
	{"/bitable/", "bitable"},
	{"/mindnote/", "mindnote"},
	{"/slides/", "slides"},
	{"/file/", "file"},
	{"/doc/", "doc"},
}

// isLoopbackHostname 判断 hostname 是否为回环地址（localhost 或回环 IP）。
//
// 必须用 net.ParseIP 而不是字符串前缀匹配：`strings.HasPrefix(h, "127.0.0.")`
// 会把攻击者可注册的 127.0.0.evil.com 当成本地地址放行，绕过
// 「非本地地址必须用 HTTPS」的约束。
// 与 internal/registry 的 isLoopbackHost 同义（该函数为包私有，无法复用）。
func isLoopbackHostname(hostname string) bool {
	h := strings.Trim(strings.TrimSpace(hostname), "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// parseWikiDeleteInput 对齐官方输入契约：严格使用 u.Hostname()，HTTPS 域名白名单，
// URL 路径推断 obj_type，裸 token 必须显式传 --obj-type
func parseWikiDeleteInput(rawInput, flagObjType string) (token, objType string, err error) {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return "", "", fmt.Errorf("<node_token> 不能为空")
	}

	flagObjType = strings.ToLower(strings.TrimSpace(flagObjType))

	if strings.Contains(rawInput, "://") {
		u, err := url.Parse(rawInput)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "", "", fmt.Errorf("URL 格式无效: %q", rawInput)
		}
		if u.User != nil {
			return "", "", fmt.Errorf("URL 包含非法的用户信息 (userinfo): %q", rawInput)
		}

		hostname := strings.ToLower(strings.TrimSpace(u.Hostname()))
		scheme := strings.ToLower(u.Scheme)

		// 协议与域名规则：HTTP 仅允许 loopback 测试，官方域名必须是 HTTPS
		if scheme == "http" {
			// 用 net.ParseIP 精确判定回环地址：
			// strings.HasPrefix(hostname, "127.0.0.") 会命中 127.0.0.evil.com 这类
			// 攻击者可注册的域名，把外部 host 当成本地测试地址放行。
			if !isLoopbackHostname(hostname) {
				return "", "", fmt.Errorf("非本地测试地址必须使用 HTTPS 协议: %q", rawInput)
			}
		} else if scheme == "https" {
			// 校验官方域名白名单（精准校验 hostname，严防 evilfeishu.cn 等前缀/伪造域名）
			valid := hostname == "feishu.cn" || strings.HasSuffix(hostname, ".feishu.cn") ||
				hostname == "larksuite.com" || strings.HasSuffix(hostname, ".larksuite.com") ||
				hostname == "larkoffice.com" || strings.HasSuffix(hostname, ".larkoffice.com") ||
				hostname == "localhost" || hostname == "127.0.0.1"
			if !valid {
				return "", "", fmt.Errorf("不支持的域名 %q，仅接受飞书/Lark 官方文档域名 (*.feishu.cn, *.larksuite.com, *.larkoffice.com)", hostname)
			}
		} else {
			return "", "", fmt.Errorf("不支持的 URL 协议 %q，仅支持 https (本地测试支持 http)", scheme)
		}

		// 检查原始 Path 中是否包含 encoded slash (%2f/%2F) 或 percent / control 字符
		if strings.Contains(u.RawPath, "%2f") || strings.Contains(u.RawPath, "%2F") {
			return "", "", fmt.Errorf("URL 路径包含非法的转义斜杠 %%2f")
		}
		if strings.Contains(u.Path, "%") || unsafeResourceChars.MatchString(u.Path) {
			return "", "", fmt.Errorf("URL 路径包含非法字符")
		}

		// 严格单 segment marker/token：按 / 拆分，去除两端斜杠后必须恰好为 2 个 segment [marker, token]
		trimmedPath := strings.Trim(u.Path, "/")
		segments := strings.Split(trimmedPath, "/")
		if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
			return "", "", fmt.Errorf("URL 路径格式无效: %q，必须为标准的 '/<type>/<token>' 形式（且不能包含多余路径段）", u.Path)
		}

		marker := "/" + segments[0] + "/"
		inferredType := ""
		for _, m := range wikiURLMarkers {
			if m.Marker == marker {
				inferredType = m.ObjType
				break
			}
		}
		if inferredType == "" {
			return "", "", fmt.Errorf("不支持的 URL 路径前缀 %q，期望以 /wiki/, /docx/, /sheets/, /base/, /mindnote/, /slides/, /file/, /doc/ 开头", marker)
		}

		token = segments[1]
		// 对 URL 提取的 token 执行 ResourceName 等价校验
		if err := validateResourceIdentifier(token, "URL 中的文档 token"); err != nil {
			return "", "", err
		}

		if flagObjType != "" && flagObjType != inferredType {
			return "", "", fmt.Errorf("--obj-type %q 与从 URL 推断的文档类型 %q 冲突；请二选一", flagObjType, inferredType)
		}
		objType = inferredType
	} else {
		token = rawInput
		if err := validateResourceIdentifier(token, "--node-token"); err != nil {
			return "", "", err
		}
		if flagObjType == "" {
			return "", "", fmt.Errorf("当输入为裸 token 时，--obj-type 为必填项（无法从 URL 自动推断文档类型）；可选值: wiki, doc, docx, sheet, bitable, mindnote, slides, file")
		}
		objType = flagObjType
	}

	if !validWikiNodeDeleteObjTypes[objType] {
		return "", "", fmt.Errorf("不支持的 --obj-type %q；可选值: wiki, doc, docx, sheet, bitable, mindnote, slides, file", objType)
	}
	return token, objType, nil
}

var (
	wikiDeleteNodePollAttempts = 30
	wikiDeleteNodePollInterval = 2 * time.Second
)

var deleteWikiNodeCmd = &cobra.Command{
	Use:   "delete <node_token>",
	Short: "删除知识库节点",
	Long: `删除知识库节点（通过官方 Wiki 节点删除 API）。
URL 输入（/wiki/, /docx/, /sheets/ 等）自动推断文档类型；裸 token 输入必须显式指定 --obj-type。
若节点包含子节点或数据量较大，后端可能转为异步任务，本命令会自动轮询任务直至完成。

参数:
  node_token    节点 Token 或知识库完整 URL（必填）

可选参数:
  --space-id            知识空间 ID（可选，未指定时自动通过 get_node 解析）
  --obj-type            文档类型（裸 token 必填，URL 输入自动推断；可选: wiki, doc, docx, sheet, bitable, mindnote, slides, file）
  --include-children    是否级联删除子节点（默认 true）
  --force, -f           跳过确认直接删除
  --output, -o          输出格式 (json)

示例:
  # 通过 URL 删除节点（自动推断空间和类型）
  feishu-cli wiki delete https://sample.feishu.cn/wiki/wikcnXXXXXX

  # 通过裸 token 删除（必须指定 --obj-type）
  feishu-cli wiki delete wikcnXXXXXX --obj-type wiki

  # 指定空间 ID 删除并跳过确认
  feishu-cli wiki delete wikcnXXXXXX --obj-type wiki --space-id 7012345678901234567 -f

  # JSON 格式输出
  feishu-cli wiki delete https://sample.feishu.cn/wiki/wikcnXXXXXX -o json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		output = strings.ToLower(strings.TrimSpace(output))
		if output != "" && output != "json" {
			return fmt.Errorf("不支持的 --output %q，仅支持 json（或留空使用默认格式）", output)
		}

		rawObjType, _ := cmd.Flags().GetString("obj-type")
		nodeToken, objType, err := parseWikiDeleteInput(args[0], rawObjType)
		if err != nil {
			return err
		}

		spaceID, _ := cmd.Flags().GetString("space-id")
		spaceID = strings.TrimSpace(spaceID)
		if spaceID != "" {
			if err := validateResourceIdentifier(spaceID, "--space-id"); err != nil {
				return err
			}
		}
		includeChildren, _ := cmd.Flags().GetBool("include-children")
		force, _ := cmd.Flags().GetBool("force")

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		identity := "bot"
		if token != "" {
			identity = "user"
		}

		nodeTitle := ""
		if spaceID == "" {
			// 未指定 space-id 时通过 get_node 解析 space_id（对 wiki token 省略 obj_type，对 non-wiki token 传 obj_type）
			node, err := client.GetWikiNodeWithOptions(nodeToken, objType, token)
			if err != nil {
				return fmt.Errorf("获取节点信息失败: %w", err)
			}
			spaceID = strings.TrimSpace(node.SpaceID)
			if spaceID == "" {
				return fmt.Errorf("未能通过 get_node 获取 space_id，请通过 --space-id 显式指定")
			}
			if err := validateResourceIdentifier(spaceID, "从节点解析出的 space_id"); err != nil {
				return fmt.Errorf("节点所属 space_id 非法: %w", err)
			}
			nodeTitle = node.Title
		}

		// 危险操作确认
		if !force {
			target := nodeToken
			if nodeTitle != "" {
				target = fmt.Sprintf("%q (%s)", nodeTitle, nodeToken)
			}
			// 必须点明级联范围：--include-children 默认 true，
			// 用户以为只删一个节点，实际会连带销毁整棵子树。
			scope := "此操作不可恢复"
			if includeChildren {
				scope = "将级联删除该节点及其**全部子节点**，此操作不可恢复"
			} else {
				scope = "仅删除该节点本身（--include-children=false），此操作不可恢复"
			}
			if err := confirmDangerousAction(cmd, fmt.Sprintf("确定要删除知识库节点 %s 吗？%s", target, scope)); err != nil {
				return err
			}
		}

		fmt.Fprintf(os.Stderr, "提交删除知识库节点请求 space_id=%s, node_token=%s ...\n", spaceID, nodeToken)
		taskID, err := client.DeleteWikiNode(spaceID, nodeToken, objType, includeChildren, token)
		if err != nil {
			return err
		}

		result := map[string]any{
			"space_id":         spaceID,
			"node_token":       nodeToken,
			"obj_type":         objType,
			"include_children": includeChildren,
			"ready":            false,
			"failed":           false,
			"status":           "success",
		}

		if taskID == "" {
			// 同步删除完成
			result["ready"] = true
			result["status"] = "success"
			return printDeleteWikiNodeResult(result, output)
		}

		if err := validateResourceIdentifier(taskID, "task_id"); err != nil {
			return fmt.Errorf("服务端返回非法的 task_id: %w", err)
		}

		// 异步任务：轮询
		result["task_id"] = taskID
		result["status"] = "processing"
		fmt.Fprintf(os.Stderr, "后端转为异步任务 task_id=%s，开始轮询...\n", taskID)

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		status, err := pollDeleteWikiNodeTask(ctx, taskID, token, identity)
		if err != nil {
			return err
		}
		// 用 status.Ready() 判定而非硬置 true：轮询虽在失败态提前返回 error，
		// 但若服务端出现非 success 的终态（或未来新增状态），ready 必须如实反映，
		// 不能让 JSON 消费方把未完成的删除当成已完成。
		result["ready"] = status.Ready()
		result["failed"] = !status.Ready()
		result["status"] = status.Status
		if status.StatusMsg != "" {
			result["status_msg"] = status.StatusMsg
		}
		return printDeleteWikiNodeResult(result, output)
	},
}

// quotePOSIXShell 为字符串生成 POSIX shell 单引号包裹格式，确保 $()、反引号、美元变量均作为字面量且可安全无损还原
func quotePOSIXShell(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func buildWikiDeleteNodeResumeCmd(taskID, identity string) string {
	return fmt.Sprintf("feishu-cli drive task-result --scenario wiki_delete_node --task-id %s --as %s", quotePOSIXShell(taskID), identity)
}

func pollDeleteWikiNodeTask(ctx context.Context, taskID, userToken, identity string) (*client.WikiDeleteNodeTaskStatus, error) {
	resumeCmd := buildWikiDeleteNodeResumeCmd(taskID, identity)
	var last client.WikiDeleteNodeTaskStatus
	var lastErr error
	hadSuccessfulPoll := false

	for attempt := 1; attempt <= wikiDeleteNodePollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return &last, fmt.Errorf("知识库节点删除轮询被取消 (task_id=%s): %w\n可通过以下命令继续查询: %s", taskID, ctx.Err(), resumeCmd)
			case <-time.After(wikiDeleteNodePollInterval):
			}
		}
		st, err := client.GetWikiDeleteNodeTask(taskID, userToken)
		if err != nil {
			lastErr = err
			fmt.Fprintf(os.Stderr, "  [%d/%d] 查询失败: %v\n", attempt, wikiDeleteNodePollAttempts, err)
			continue
		}
		last = *st
		hadSuccessfulPoll = true

		if st.Ready() {
			fmt.Fprintf(os.Stderr, "任务完成 ✅\n")
			return st, nil
		}
		if st.Failed() {
			return st, fmt.Errorf("delete_node 任务失败 (task_id=%s): status=%s, msg=%s", taskID, st.Status, st.StatusMsg)
		}
		fmt.Fprintf(os.Stderr, "  [%d/%d] status=%s\n", attempt, wikiDeleteNodePollAttempts, st.Status)
	}

	if !hadSuccessfulPoll && lastErr != nil {
		return &last, fmt.Errorf("知识库节点删除任务已提交，但状态查询全部失败 (task_id=%s): %w\n后续可通过以下命令查询任务状态: %s", taskID, lastErr, resumeCmd)
	}

	return &last, fmt.Errorf("知识库节点删除任务仍在执行中或轮询超时 (task_id=%s, 当前状态=%s)\n请勿重复提交，可通过以下命令继续查询: %s", taskID, last.Status, resumeCmd)
}

func printDeleteWikiNodeResult(result map[string]any, output string) error {
	if output == "json" {
		return printJSON(result)
	}
	fmt.Printf("知识库节点删除成功！\n")
	fmt.Printf("  空间 ID:    %s\n", result["space_id"])
	fmt.Printf("  节点 Token: %s\n", result["node_token"])
	if tid, ok := result["task_id"].(string); ok && tid != "" {
		fmt.Printf("  任务 ID:    %s\n", tid)
		fmt.Printf("  任务状态:   %v\n", result["status"])
	}
	return nil
}

func init() {
	wikiCmd.AddCommand(deleteWikiNodeCmd)
	deleteWikiNodeCmd.Flags().String("space-id", "", "知识空间 ID（可选，未指定时自动解析）")
	deleteWikiNodeCmd.Flags().String("obj-type", "", "文档类型（裸 token 必填，URL 输入自动推断；可选: wiki, doc, docx, sheet, bitable, mindnote, slides, file）")
	deleteWikiNodeCmd.Flags().String("as", "auto", "操作身份：bot|user|auto（默认 auto: User 优先，回退 Bot）")
	deleteWikiNodeCmd.Flags().Bool("include-children", true, "是否级联删除子节点（默认 true）")
	deleteWikiNodeCmd.Flags().BoolP("force", "f", false, "跳过确认直接删除")
	deleteWikiNodeCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
	deleteWikiNodeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")
}
