package cmd

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 契约门禁：静态扫描 cmd/ 下的 RunE，拦截三类回归。
//
//	dryrun-order：RunE 里 config.Validate 或必需身份 helper 出现在 --dry-run 分支之前。
//	              预览不需要凭据，先校验会让无凭据环境下的 --dry-run 以 exit 3 失败。
//	usage-errorf：以 "--" 开头的 fmt.Errorf（参数错误）。参数错误应使用 clierr.Usagef（exit 2）。
//	raw-http：    使用 http.DefaultClient / http.Get 等，绕过 rawHTTPClient 的 host 白名单与可中断上下文。
//
// 存量例外登记在 testdata/contract_gate_allowlist.txt，一行一条 "<规则> <文件> <键>"，可在 # 后注明原因。
// 门禁只拦新增：新出现的违规会失败；allowlist 中已修复的条目同样会失败，提醒删除过期例外。

const contractGateAllowlist = "testdata/contract_gate_allowlist.txt"

// 必需凭据的调用：出现在 dry-run 判断之前即违规。
var dryRunBlockingCalls = map[string]bool{
	"config.Validate":          true,
	"requireUserToken":         true,
	"resolveRequiredUserToken": true,
}

type gateViolation struct {
	rule, file, key string
	pos             token.Position
}

func (v gateViolation) id() string { return v.rule + " " + v.file + " " + v.key }

func TestContractGate(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var found []gateViolation
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", name, err)
		}
		found = append(found, scanContractGate(fset, name, f)...)
	}

	allowed := loadContractGateAllowlist(t)
	seen := map[string]bool{}
	var fresh []string
	for _, v := range found {
		seen[v.id()] = true
		if !allowed[v.id()] {
			fresh = append(fresh, fmt.Sprintf("%s  (%s)", v.id(), v.pos))
		}
	}
	var stale []string
	for id := range allowed {
		if !seen[id] {
			stale = append(stale, id)
		}
	}
	sort.Strings(fresh)
	sort.Strings(stale)
	if len(fresh) > 0 {
		t.Errorf("新增 %d 处契约违规（标准顺序：参数校验 → --dry-run → config.Validate → 身份 → 确认 → 执行；参数错误用 clierr.Usagef；HTTP 用 rawHTTPClient）：\n  %s",
			len(fresh), strings.Join(fresh, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("%s 中有 %d 条例外已不再命中，请删除：\n  %s", contractGateAllowlist, len(stale), strings.Join(stale, "\n  "))
	}
}

func scanContractGate(fset *token.FileSet, file string, f *ast.File) []gateViolation {
	var out []gateViolation
	usageCount := map[string]int{}
	var firstUsage token.Position
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			use, runE := cobraUseAndRunE(x)
			if runE != nil {
				if v, ok := dryRunOrderViolation(fset, file, use, runE); ok {
					out = append(out, v)
				}
			}
		case *ast.CallExpr:
			if name := selectorName(x.Fun); name == "fmt.Errorf" && len(x.Args) > 0 {
				if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil && strings.HasPrefix(s, "--") {
						usageCount[file]++
						if usageCount[file] == 1 {
							firstUsage = fset.Position(x.Pos())
						}
					}
				}
			}
		case *ast.SelectorExpr:
			if name := selectorName(x); name == "http.DefaultClient" || name == "http.Get" || name == "http.Post" || name == "http.PostForm" || name == "http.Head" {
				out = append(out, gateViolation{"raw-http", file, name, fset.Position(x.Pos())})
			}
		}
		return true
	})
	// usage-errorf 按文件计数（棘轮）：数量增加即失败；修掉一部分后把 allowlist 中的计数改小。
	if n := usageCount[file]; n > 0 {
		out = append(out, gateViolation{"usage-errorf", file, fmt.Sprintf("count=%d", n), firstUsage})
	}
	return out
}

// cobraUseAndRunE 从 &cobra.Command{Use: ..., RunE: func...} 中取出 Use 与 RunE 函数体。
func cobraUseAndRunE(lit *ast.CompositeLit) (string, *ast.FuncLit) {
	if selectorName(lit.Type) != "cobra.Command" {
		return "", nil
	}
	var use string
	var runE *ast.FuncLit
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, _ := kv.Key.(*ast.Ident)
		if key == nil {
			continue
		}
		switch key.Name {
		case "Use":
			if b, ok := kv.Value.(*ast.BasicLit); ok {
				if s, err := strconv.Unquote(b.Value); err == nil {
					use = strings.Fields(s + " _")[0]
				}
			}
		case "RunE":
			runE, _ = kv.Value.(*ast.FuncLit)
		}
	}
	return use, runE
}

// dryRunOrderViolation：RunE 中首个必需凭据调用早于首个 "dry-run" 读取。
func dryRunOrderViolation(fset *token.FileSet, file, use string, fn *ast.FuncLit) (gateViolation, bool) {
	var firstBlock, firstDry token.Pos
	var blockName string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			name := selectorName(x.Fun)
			if id, ok := x.Fun.(*ast.Ident); ok {
				name = id.Name
			}
			if dryRunBlockingCalls[name] && (firstBlock == token.NoPos || x.Pos() < firstBlock) {
				firstBlock, blockName = x.Pos(), name
			}
		case *ast.BasicLit:
			if x.Kind == token.STRING && x.Value == `"dry-run"` && (firstDry == token.NoPos || x.Pos() < firstDry) {
				firstDry = x.Pos()
			}
		case *ast.Ident:
			if strings.EqualFold(x.Name, "dryRun") && (firstDry == token.NoPos || x.Pos() < firstDry) {
				firstDry = x.Pos()
			}
		}
		return true
	})
	if firstBlock == token.NoPos || firstDry == token.NoPos || firstBlock > firstDry {
		return gateViolation{}, false
	}
	return gateViolation{"dryrun-order", file, use + ":" + blockName, fset.Position(firstBlock)}, true
}

func selectorName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name + "." + x.Sel.Name
		}
	case *ast.StarExpr:
		return selectorName(x.X)
	}
	return ""
}

func loadContractGateAllowlist(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	fh, err := os.Open(contractGateAllowlist)
	if os.IsNotExist(err) {
		return out
	}
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if fields := strings.Fields(line); len(fields) == 3 {
			out[strings.Join(fields, " ")] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
