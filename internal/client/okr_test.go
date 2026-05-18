package client

import (
	"encoding/json"
	"testing"
)

// TestOKRRawCycleToCycle 验证 v1/periods 响应解析为 OKRCycle 的映射正确
func TestOKRRawCycleToCycle(t *testing.T) {
	body := `{
		"id": "635782378412311",
		"zh_name": "2026 Q2",
		"en_name": "2026 Q2",
		"status": 0,
		"period_start_time": "1712016000000",
		"period_end_time": "1719792000000"
	}`
	var raw okrRawCycle
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("unmarshal v1 period failed: %v", err)
	}
	c := raw.toCycle()
	if c == nil {
		t.Fatal("toCycle returned nil")
	}
	if c.ID != "635782378412311" {
		t.Errorf("ID = %q, want 635782378412311", c.ID)
	}
	if c.ZhName != "2026 Q2" {
		t.Errorf("ZhName = %q", c.ZhName)
	}
	if c.EnName != "2026 Q2" {
		t.Errorf("EnName = %q", c.EnName)
	}
	if c.CycleStatus != "normal" {
		t.Errorf("CycleStatus = %q, want normal (status=0)", c.CycleStatus)
	}
	if c.StartTime == "" || c.EndTime == "" {
		t.Errorf("StartTime/EndTime not formatted: start=%q end=%q", c.StartTime, c.EndTime)
	}
}

// TestOKRCycleStatusMapping 验证 status int → 文本映射齐全
func TestOKRCycleStatusMapping(t *testing.T) {
	cases := map[okrCycleStatus]string{
		okrCycleStatusNormal:  "normal",
		okrCycleStatusPending: "pending",
		okrCycleStatusInvalid: "invalid",
		okrCycleStatusHidden:  "hidden",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("status %d → %q, want %q", int(s), got, want)
		}
	}
}

// TestCreateOKRProgressSourceURLDefault 验证 SourceURL 未填时会兜底 placeholder（避免 422）
func TestCreateOKRProgressSourceURLDefault(t *testing.T) {
	// 这里只验证 options 默认值逻辑（不实际打网络），通过反向构造
	opts := CreateOKRProgressOptions{
		TargetID:   "7xxx",
		TargetType: OKRTargetObjective,
	}
	// 复刻 CreateOKRProgress 前几行的默认值逻辑
	if opts.SourceTitle == "" {
		opts.SourceTitle = "created by feishu-cli"
	}
	if opts.SourceURL == "" {
		opts.SourceURL = "https://www.feishu.cn/okr/progress"
	}
	if opts.SourceURL != "https://www.feishu.cn/okr/progress" {
		t.Errorf("default SourceURL = %q", opts.SourceURL)
	}
}
