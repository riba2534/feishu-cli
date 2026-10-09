package registry

import (
	"slices"
	"testing"
)

// TestSlidesDomainAliasIncludesMediaUpload 验证 slides domain 含 docs:document.media:upload
// 修复 codex review P2 finding：auth login --domain slides --recommend 后用 media-upload 应不 403
func TestSlidesDomainAliasIncludesMediaUpload(t *testing.T) {
	scopes, ok := extraDomainScopes["slides"]
	if !ok {
		t.Fatal("slides domain alias missing")
	}
	want := "docs:document.media:upload"
	for _, s := range scopes {
		if s == want {
			return
		}
	}
	t.Errorf("slides domain alias 缺少 %q, got %v", want, scopes)
}

// slidesCommandScopes 是 slides 各子命令实际所需的 scope（与命令 help 的"权限"行、官方 shortcuts 一致）。
var slidesCommandScopes = map[string][]string{
	"create":        {"slides:presentation:create", "slides:presentation:write_only", "docs:document.media:upload"},
	"add-slide":     {"slides:presentation:update", "slides:presentation:write_only"},
	"delete-slide":  {"slides:presentation:update", "slides:presentation:write_only"},
	"replace-slide": {"slides:presentation:update", "slides:presentation:write_only"},
	"update-slide":  {"slides:presentation:update", "slides:presentation:write_only"},
	"get":           {"slides:presentation:read"},
	"screenshot":    {"slides:presentation:screenshot"},
	"media-upload":  {"docs:document.media:upload"},
}

// TestSlidesDomainCoversCommandScopes --domain slides（含 --recommend）必须覆盖全部 slides 子命令所需 scope，
// 且不能依赖 meta / 运行时 overlay 是否收录对应方法（extraDomainScopes 自身就要完整）。
func TestSlidesDomainCoversCommandScopes(t *testing.T) {
	extra := extraDomainScopes["slides"]
	all := CollectDomainScopes([]string{"slides"}, false)
	recommended := CollectDomainScopes([]string{"slides"}, true)
	for cmd, scopes := range slidesCommandScopes {
		for _, s := range scopes {
			if !slices.Contains(extra, s) {
				t.Errorf("extraDomainScopes[slides] 缺少 slides %s 所需的 %s", cmd, s)
			}
			if !slices.Contains(all, s) {
				t.Errorf("--domain slides 缺少 slides %s 所需的 %s: %v", cmd, s, all)
			}
			if !slices.Contains(recommended, s) {
				t.Errorf("--domain slides --recommend 缺少 slides %s 所需的 %s: %v", cmd, s, recommended)
			}
		}
	}
}
