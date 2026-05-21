package cmd

import (
	"testing"
)

// TestAttendanceUserStatsCmdRegistered 验证 attendance user-stats 命令组注册
func TestAttendanceUserStatsCmdRegistered(t *testing.T) {
	if attendanceUserStatsCmd.Use != "user-stats" {
		t.Fatalf("attendanceUserStatsCmd.Use = %q, want user-stats", attendanceUserStatsCmd.Use)
	}
	// 别名应包含 stats
	hasStats := false
	for _, a := range attendanceUserStatsCmd.Aliases {
		if a == "stats" {
			hasStats = true
		}
	}
	if !hasStats {
		t.Fatalf("attendanceUserStatsCmd should have alias 'stats', got %v", attendanceUserStatsCmd.Aliases)
	}
	found := false
	for _, sub := range attendanceCmd.Commands() {
		if sub == attendanceUserStatsCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("attendanceUserStatsCmd should be registered as child of attendanceCmd")
	}
}

// TestAttendanceUserStatsQueryFlags 校验 query 关键 flag 注册及默认值
func TestAttendanceUserStatsQueryFlags(t *testing.T) {
	empType := attendanceUserStatsQueryCmd.Flags().Lookup("employee-type")
	if empType == nil {
		t.Fatal("--employee-type flag missing")
	}
	if empType.DefValue != "employee_id" {
		t.Fatalf("--employee-type default = %q, want employee_id", empType.DefValue)
	}
	statsType := attendanceUserStatsQueryCmd.Flags().Lookup("stats-type")
	if statsType == nil {
		t.Fatal("--stats-type flag missing")
	}
	if statsType.DefValue != "daily" {
		t.Fatalf("--stats-type default = %q, want daily", statsType.DefValue)
	}
	for _, want := range []string{"user-ids", "current-user-id", "start", "end", "locale", "need-history", "current-group-only", "output"} {
		if attendanceUserStatsQueryCmd.Flags().Lookup(want) == nil {
			t.Errorf("--%s flag missing", want)
		}
	}
}

// TestAttendanceUserStatsQueryRequiredFlags 校验 required flag 注解
func TestAttendanceUserStatsQueryRequiredFlags(t *testing.T) {
	for _, name := range []string{"user-ids", "start", "end"} {
		flag := attendanceUserStatsQueryCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("--%s flag missing", name)
			continue
		}
		anno := flag.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		if len(anno) != 1 || anno[0] != "true" {
			t.Errorf("--%s should be marked required, got %#v", name, flag.Annotations)
		}
	}
}

// TestAttendanceUserStatsQueryArgsNoArgs 校验 cobra.NoArgs
func TestAttendanceUserStatsQueryArgsNoArgs(t *testing.T) {
	if err := attendanceUserStatsQueryCmd.Args(attendanceUserStatsQueryCmd, []string{}); err != nil {
		t.Fatalf("expected no error for 0 args, got %v", err)
	}
	if err := attendanceUserStatsQueryCmd.Args(attendanceUserStatsQueryCmd, []string{"unexpected"}); err == nil {
		t.Fatal("expected error when extra positional arg provided")
	}
}
