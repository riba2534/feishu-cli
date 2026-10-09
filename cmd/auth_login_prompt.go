package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
)

type interactiveLoginScopeSelection struct {
	Domains   []string
	Recommend bool
}

func canPromptLoginScope() bool {
	in, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	errOut, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (in.Mode()&os.ModeCharDevice) != 0 && (errOut.Mode()&os.ModeCharDevice) != 0
}

func runInteractiveLoginScopePrompt() (*interactiveLoginScopeSelection, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stderr, "未指定授权范围，请先选择业务域。")
	fmt.Fprintf(os.Stderr, "可选 domain: %s, all\n", strings.Join(auth.KnownScopeDomainNames(), ", "))
	fmt.Fprint(os.Stderr, "domain（逗号分隔）> ")
	domainLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("读取 domain 输入失败: %w", err)
	}

	domains, err := auth.ParseScopeDomains([]string{strings.TrimSpace(domainLine)})
	if err != nil {
		return nil, err
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("未选择任何 domain，请使用 --scope 或 --domain 显式指定授权范围")
	}

	fmt.Fprint(os.Stderr, "scope 级别 [recommended/all]（默认 recommended）> ")
	levelLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("读取 scope 级别失败: %w", err)
	}
	level := strings.ToLower(strings.TrimSpace(levelLine))
	recommend := true
	switch level {
	case "", "recommended", "recommend", "r":
		recommend = true
	case "all", "a":
		recommend = false
	default:
		return nil, fmt.Errorf("不支持的 scope 级别 %q，可选值: recommended / all", level)
	}

	return &interactiveLoginScopeSelection{
		Domains:   domains,
		Recommend: recommend,
	}, nil
}
