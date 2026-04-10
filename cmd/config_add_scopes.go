package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// 预定义的权限域，方便用户按域批量申请
var scopeDomains = map[string][]string{
	"calendar": {
		"calendar:calendar",
		"calendar:calendar:readonly",
		"calendar:calendar.event:read",
		"calendar:calendar.event:write",
	},
	"task": {
		"task:task:read",
		"task:task:write",
		"task:comment:read",
		"task:comment:write",
		"task:tasklist:read",
		"task:tasklist:write",
	},
	"vc": {
		"vc:meeting:readonly",
		"vc:room:readonly",
	},
	"minutes": {
		"minutes:minutes:readonly",
	},
	"doc": {
		"docx:document",
		"docx:document:readonly",
		"wiki:wiki:readonly",
		"drive:drive",
	},
	"im": {
		"im:message",
		"im:message:send_as_bot",
		"im:message:readonly",
		"im:chat",
		"im:chat:readonly",
	},
	"bitable": {
		"bitable:app",
	},
	"sheet": {
		"sheets:spreadsheet",
	},
	"contact": {
		"contact:user.base:readonly",
		"contact:contact.base:readonly",
	},
	"search": {
		"search:docs:read",
		"search:message",
	},
	"export": {
		"drive:export:readonly",
		"docs:document:export",
	},
	"all": {}, // 特殊标记，使用所有域
}

var configAddScopesCmd = &cobra.Command{
	Use:   "add-scopes",
	Short: "为应用申请开通权限",
	Long: `生成飞书开放平台的权限申请链接，在浏览器中打开即可申请开通。

支持两种方式:
  1. 按域名批量申请: --domain calendar,task,vc
  2. 指定具体 scope: --scopes "calendar:calendar:readonly task:task:read"

可用的域名:
  calendar  日历（日程读写）
  task      任务（任务/评论/清单读写）
  vc        视频会议（会议记录查询）
  minutes   妙记（妙记信息读取）
  doc       文档（文档/知识库/云空间读写）
  im        消息（消息收发/群聊管理）
  bitable   多维表格
  sheet     电子表格
  contact   通讯录（用户信息查询）
  search    搜索（文档/消息搜索）
  export    导出（文档/表格导出）
  all       申请所有常用权限

示例:
  # 申请日历和任务权限
  feishu-cli config add-scopes --domain calendar,task

  # 申请所有常用权限
  feishu-cli config add-scopes --domain all

  # 申请指定 scope
  feishu-cli config add-scopes --scopes "vc:meeting:readonly minutes:minutes:readonly"

  # 只输出链接不打开浏览器
  feishu-cli config add-scopes --domain calendar --print-only`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		scopes, _ := cmd.Flags().GetString("scopes")
		printOnly, _ := cmd.Flags().GetBool("print-only")

		// 获取 app_id
		cfg := config.Get()
		appID := cfg.AppID
		if appID == "" {
			appID = os.Getenv("FEISHU_APP_ID")
		}
		if appID == "" {
			return fmt.Errorf("缺少 app_id，请先配置应用:\n  feishu-cli config create-app\n  或 export FEISHU_APP_ID=xxx")
		}

		// 收集需要申请的 scopes
		var scopeList []string
		if scopes != "" {
			scopeList = strings.Fields(scopes)
		}
		if domain != "" {
			domains := strings.Split(domain, ",")
			for _, d := range domains {
				d = strings.TrimSpace(d)
				if d == "all" {
					// 收集所有域的 scope
					for name, ss := range scopeDomains {
						if name != "all" {
							scopeList = append(scopeList, ss...)
						}
					}
					break
				}
				if ss, ok := scopeDomains[d]; ok {
					scopeList = append(scopeList, ss...)
				} else {
					return fmt.Errorf("未知的域名: %s\n可用域名: calendar, task, vc, minutes, doc, im, bitable, sheet, contact, search, export, all", d)
				}
			}
		}

		if len(scopeList) == 0 {
			return fmt.Errorf("请通过 --domain 或 --scopes 指定需要申请的权限")
		}

		// 去重
		seen := make(map[string]bool)
		var unique []string
		for _, s := range scopeList {
			if !seen[s] {
				seen[s] = true
				unique = append(unique, s)
			}
		}

		// 构建权限申请链接
		// 优先使用 /page/scope-apply（自注册应用专用），降级到开放平台后台
		host := "open.feishu.cn"
		baseURL := cfg.BaseURL
		if strings.Contains(baseURL, "larksuite.com") {
			host = "open.larksuite.com"
		}
		scopeStr := strings.Join(unique, ",")

		// /page/scope-apply 仅支持自注册应用（PersonalAgent），其他应用用后台链接
		scopeApplyURL := fmt.Sprintf("https://%s/page/scope-apply?clientID=%s&scopes=%s",
			host, url.QueryEscape(appID), url.QueryEscape(strings.Join(unique, " ")))
		consoleURL := fmt.Sprintf("https://%s/app/%s/auth?q=%s&op_from=openapi&token_type=tenant",
			host, appID, url.QueryEscape(scopeStr))

		// 默认使用后台链接（更通用），同时输出 scope-apply 链接供自注册应用使用
		authURL := consoleURL

		fmt.Fprintf(os.Stderr, "需要申请 %d 个权限:\n", len(unique))
		for _, s := range unique {
			fmt.Fprintf(os.Stderr, "  · %s\n", s)
		}
		fmt.Fprintln(os.Stderr)

		if printOnly {
			fmt.Println(authURL)
			return nil
		}

		fmt.Fprintln(os.Stderr, "请在浏览器中打开以下链接，点击「开通权限」:")
		fmt.Fprintf(os.Stderr, "\n  后台链接: %s\n", consoleURL)
		fmt.Fprintf(os.Stderr, "  一键申请: %s\n\n", scopeApplyURL)

		// 尝试自动打开浏览器
		if err := auth.TryOpenBrowser(authURL); err == nil {
			fmt.Fprintln(os.Stderr, "已自动打开浏览器")
		}

		return nil
	},
}

func init() {
	configCmd.AddCommand(configAddScopesCmd)
	configAddScopesCmd.Flags().String("domain", "", "权限域（逗号分隔）: calendar,task,vc,minutes,doc,im,bitable,sheet,contact,search,export,all")
	configAddScopesCmd.Flags().String("scopes", "", "具体 scope 列表（空格分隔）")
	configAddScopesCmd.Flags().Bool("print-only", false, "只输出链接不打开浏览器")
}
