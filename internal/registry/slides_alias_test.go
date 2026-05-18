package registry

import "testing"

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
