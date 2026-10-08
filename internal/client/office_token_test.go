package client

import "testing"

// TestIsLocalOfficeToken 固定 slides/sheets 共享的导入型 Office token 形状。
// 负例与正例同等重要：原生 token 误判为 Office 时上传仍成功，但图片之后静默不渲染。
func TestIsLocalOfficeToken(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  bool
	}{
		{"空 token", "", false},
		{"原生短 token", "pptcnABC123", false},

		{"fake_office 前缀", "fake_office_abc123", true},
		{"仅 fake_office 前缀", FakeOfficeTokenPrefix, true},
		{"local_office 前缀", "local_office_abc123", true},
		{"仅 local_office 前缀", LocalOfficeTokenPrefix, true},
		{"fake_office 出现在中间不算前缀", "pptfake_office_abc", false},
		{"local_office 出现在中间不算前缀", "pptlocal_office_abc", false},

		{"交织 OFL0X 25 位（标记恰好填满）", "aaaaOaaaaFaaaaLaaaa0aaaaX", true},
		{"交织 OFL0X 27 位（当前导入格式）", "aaaaOaaaaFaaaaLaaaa0aaaaXaa", true},
		{"交织 OFL0X 28 位（旧实现唯一认的长度）", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa", true},
		{"交织 OFL0X 28 位带 ppt 类型枚举", "ccccOccccFccccLcccc0ccccXccP", true},
		{"交织 OFL0X 29 位", "aaaaOaaaaFaaaaLaaaa0aaaaXaaaa", true},
		{"交织 OFL0X 24 位（容纳不下标记）", "aaaaOaaaaFaaaaLaaaa0aaaa", false},

		{"交织 pptcn 原生 token", "abcdpefghpijkltmnopcqrstnuv", false},
		{"交织 shtcn 原生 token", "abcdsefghhijkltmnopcqrstnuv", false},
		{"OFL0X 出现但不在固定偏移", "OFL0Xaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"标记整体错位一位", "aaaaaOaaaaFaaaaLaaaa0aaaaX", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsLocalOfficeToken(tc.token); got != tc.want {
				t.Fatalf("IsLocalOfficeToken(%q) = %v, want %v", tc.token, got, tc.want)
			}
		})
	}
}

// TestOfficeTokenMinLenMatchesMarkerOffsets 保证长度下限由偏移推导：
// 下限高于最后偏移会拒绝合法短 token，低于则会越界。
func TestOfficeTokenMinLenMatchesMarkerOffsets(t *testing.T) {
	last := officeTokenMarkerOffsets[len(officeTokenMarkerOffsets)-1]
	if officeTokenMinLen != last+1 {
		t.Fatalf("officeTokenMinLen = %d, want %d（最后一个标记偏移 %d 之后一位）", officeTokenMinLen, last+1, last)
	}
}
