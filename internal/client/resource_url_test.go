package client

import (
	"strings"
	"testing"
)

func TestParseResourceURL_PathPrefixOnly(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		wantType  string
		wantToken string
	}{
		{"docx", "https://example.feishu.cn/docx/DocABC123", ResourceTypeDocx, "DocABC123"},
		// 回归：query 中出现 /wiki/ 不得劫持解析（旧实现解析成 wiki 节点 zzz）
		{"docx 带 from=/wiki/ query", "https://example.feishu.cn/docx/DocABC123?from=/wiki/zzz", ResourceTypeDocx, "DocABC123"},
		{"docx 带 fragment /wiki/", "https://example.feishu.cn/docx/DocABC123#/wiki/zzz", ResourceTypeDocx, "DocABC123"},
		{"wiki", "https://example.feishu.cn/wiki/WikNode1?fromScene=spaceOverview", ResourceTypeWiki, "WikNode1"},
		{"sheets 带 sheet 参数", "https://example.feishu.cn/sheets/ShtTok?sheet=abc", ResourceTypeSheet, "ShtTok"},
		{"spreadsheets 别名", "https://example.feishu.cn/spreadsheets/ShtTok", ResourceTypeSheet, "ShtTok"},
		{"base", "https://example.feishu.cn/base/BaseTok?table=tbl1", ResourceTypeBitable, "BaseTok"},
		{"bitable 别名", "https://example.feishu.cn/bitable/BaseTok", ResourceTypeBitable, "BaseTok"},
		{"slides", "https://example.feishu.cn/slides/SldTok", ResourceTypeSlides, "SldTok"},
		{"file", "https://example.feishu.cn/file/BoxTok", ResourceTypeFile, "BoxTok"},
		{"drive file", "https://example.feishu.cn/drive/file/BoxTok", ResourceTypeFile, "BoxTok"},
		{"drive folder", "https://example.feishu.cn/drive/folder/FldTok", ResourceTypeFolder, "FldTok"},
		{"drive shr 共享文件夹", "https://example.feishu.cn/drive/shr/FldTok", ResourceTypeFolder, "FldTok"},
		{"chat drive 群文件夹", "https://example.feishu.cn/chat/drive/FldTok", ResourceTypeFolder, "FldTok"},
		{"旧版 doc", "https://example.feishu.cn/doc/DoccnTok", ResourceTypeDoc, "DoccnTok"},
		{"旧版 docs", "https://example.feishu.cn/docs/DoccnTok", ResourceTypeDoc, "DoccnTok"},
		{"mindnote", "https://example.feishu.cn/mindnotes/BmnTok", ResourceTypeMindnote, "BmnTok"},
		{"尾部斜杠与后续段", "https://example.feishu.cn/docx/DocABC123/", ResourceTypeDocx, "DocABC123"},
		{"larksuite 品牌", "https://example.larksuite.com/docx/DocABC123", ResourceTypeDocx, "DocABC123"},
		{"larkoffice 域名", "https://example.larkoffice.com/wiki/WikNode1", ResourceTypeWiki, "WikNode1"},
		{"裸根域", "https://feishu.cn/docx/DocABC123", ResourceTypeDocx, "DocABC123"},
		{"大小写主机名", "https://Example.Feishu.CN/docx/DocABC123", ResourceTypeDocx, "DocABC123"},
		{"本地回环 http（测试）", "http://127.0.0.1:8080/docx/DocABC123", ResourceTypeDocx, "DocABC123"},
		{"导入型 office token", "https://example.feishu.cn/slides/fake_office_abc-1", ResourceTypeSlides, "fake_office_abc-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := ParseResourceURL(tc.url)
			if err != nil {
				t.Fatalf("ParseResourceURL(%q) 返回错误: %v", tc.url, err)
			}
			if ref.Type != tc.wantType || ref.Token != tc.wantToken {
				t.Fatalf("ParseResourceURL(%q) = %+v, 期望 type=%s token=%s", tc.url, ref, tc.wantType, tc.wantToken)
			}
		})
	}
}

func TestParseResourceURL_Rejects(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"伪造域名", "https://evilfeishu.cn/docx/DocABC123", "不支持的域名"},
		{"后缀伪造", "https://feishu.cn.evil.com/docx/DocABC123", "不支持的域名"},
		{"第三方域名", "https://example.com/docx/DocABC123", "不支持的域名"},
		{"非回环 http", "http://example.feishu.cn/docx/DocABC123", "https"},
		{"非 http 协议", "ftp://example.feishu.cn/docx/DocABC123", "协议"},
		{"userinfo", "https://user:pass@example.feishu.cn/docx/DocABC123", "用户信息"},
		{"未知路径", "https://example.feishu.cn/minutes/obcnXXX", "无法从 URL 路径"},
		{"标记只在 query 里", "https://example.feishu.cn/?next=/docx/DocABC123", "无法从 URL 路径"},
		{"路径非前缀", "https://example.feishu.cn/x/docx/DocABC123", "无法从 URL 路径"},
		{"缺少 token", "https://example.feishu.cn/docx/", "缺少 token"},
		{"转义斜杠", "https://example.feishu.cn/docx/Doc%2FABC", "转义分隔符"},
		{"非法 token 字符", "https://example.feishu.cn/docx/Doc.ABC", "格式无效"},
		{"不是 URL", "://bad", "URL 格式无效"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseResourceURL(tc.url)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseResourceURL(%q) 期望包含 %q 的错误，得到 %v", tc.url, tc.want, err)
			}
		})
	}
}

func TestParseResourceURL_CustomHostRequiresOptIn(t *testing.T) {
	t.Setenv("FEISHU_ALLOW_CUSTOM_BASE_URL", "")
	if _, err := ParseResourceURL("https://docs.private.example/docx/DocABC123"); err == nil {
		t.Fatal("未开启 allow_custom_base_url 时应拒绝私有域名")
	}
	t.Setenv("FEISHU_ALLOW_CUSTOM_BASE_URL", "true")
	ref, err := ParseResourceURL("https://docs.private.example/docx/DocABC123")
	if err != nil || ref.Token != "DocABC123" {
		t.Fatalf("开启 allow_custom_base_url 后应接受私有域名: ref=%+v err=%v", ref, err)
	}
}

func TestIsFeishuResourceHost(t *testing.T) {
	cases := map[string]bool{
		"feishu.cn": true, "a.feishu.cn": true, "a.b.feishu.cn": true, "feishu.cn.": true,
		"larksuite.com": true, "x.larksuite.com": true, "x.larkoffice.com": true,
		"evilfeishu.cn": false, "feishu.cn.evil.com": false, "larksuite.co": false, "": false,
	}
	for host, want := range cases {
		if got := IsFeishuResourceHost(host); got != want {
			t.Errorf("IsFeishuResourceHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestIsSafeResourceToken(t *testing.T) {
	ok := []string{"DocABC123", "fake_office_x-1", "a"}
	bad := []string{"", "a/b", "a?b", "a#b", "a%2f", "a b", "a..b", "a.b", "a\x00b", strings.Repeat("a", 129)}
	for _, s := range ok {
		if !IsSafeResourceToken(s) {
			t.Errorf("IsSafeResourceToken(%q) 应为 true", s)
		}
	}
	for _, s := range bad {
		if IsSafeResourceToken(s) {
			t.Errorf("IsSafeResourceToken(%q) 应为 false", s)
		}
	}
}

func TestNormalizeResourceType(t *testing.T) {
	cases := map[string]string{
		"DOCX": "docx", " sheets ": "sheet", "spreadsheet": "sheet", "base": "bitable",
		"slide": "slides", "mindnotes": "mindnote", "wiki": "wiki", "folder": "folder", "unknown": "unknown",
	}
	for in, want := range cases {
		if got := NormalizeResourceType(in); got != want {
			t.Errorf("NormalizeResourceType(%q) = %q, want %q", in, got, want)
		}
	}
}
