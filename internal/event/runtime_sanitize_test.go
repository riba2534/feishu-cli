package event

import "testing"

// TestSanitizeEventID 验证 event_id allowlist 防路径穿越（codex review finding #4）
func TestSanitizeEventID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc123", "abc123"},
		{"abc-XYZ_42", "abc-XYZ_42"},
		{"../../etc/passwd", "etcpasswd"},
		{"a/b/c", "abc"},
		{"id with spaces!", "idwithspaces"},
		{"", ""},
		{"....", ""},
	}
	for _, tc := range cases {
		got := sanitizeEventID(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeEventID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// 长度上限
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	got := sanitizeEventID(string(long))
	if len(got) > 128 {
		t.Errorf("sanitizeEventID length = %d, want <= 128", len(got))
	}
}
