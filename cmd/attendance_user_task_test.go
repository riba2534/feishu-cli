package cmd

import (
	"testing"
)

// TestAttendanceUserTaskCmdRegistered 验证 attendance user-task 命令组注册
func TestAttendanceUserTaskCmdRegistered(t *testing.T) {
	if attendanceUserTaskCmd.Use != "user-task" {
		t.Fatalf("attendanceUserTaskCmd.Use = %q, want user-task", attendanceUserTaskCmd.Use)
	}
	// 别名应包含 task / user-tasks
	hasTask := false
	for _, a := range attendanceUserTaskCmd.Aliases {
		if a == "task" {
			hasTask = true
		}
	}
	if !hasTask {
		t.Fatalf("attendanceUserTaskCmd should have alias 'task', got %v", attendanceUserTaskCmd.Aliases)
	}
	found := false
	for _, sub := range attendanceCmd.Commands() {
		if sub == attendanceUserTaskCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("attendanceUserTaskCmd should be registered as child of attendanceCmd")
	}
}

// TestAttendanceUserTaskQueryCmdRegistered 验证 query 子命令注册
func TestAttendanceUserTaskQueryCmdRegistered(t *testing.T) {
	if attendanceUserTaskQueryCmd.Use != "query" {
		t.Fatalf("attendanceUserTaskQueryCmd.Use = %q, want query", attendanceUserTaskQueryCmd.Use)
	}
	if attendanceUserTaskQueryCmd.Short == "" {
		t.Fatal("attendanceUserTaskQueryCmd.Short should not be empty")
	}
	found := false
	for _, sub := range attendanceUserTaskCmd.Commands() {
		if sub == attendanceUserTaskQueryCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("attendanceUserTaskQueryCmd should be registered as child of attendanceUserTaskCmd")
	}
}

// TestAttendanceUserTaskQueryFlags 校验关键 flag 注册及默认值
func TestAttendanceUserTaskQueryFlags(t *testing.T) {
	empType := attendanceUserTaskQueryCmd.Flags().Lookup("employee-type")
	if empType == nil {
		t.Fatal("--employee-type flag missing")
	}
	if empType.DefValue != "employee_id" {
		t.Fatalf("--employee-type default = %q, want employee_id", empType.DefValue)
	}
	for _, want := range []string{"user-ids", "start", "end", "need-overtime", "ignore-invalid-users", "include-terminated", "output"} {
		if attendanceUserTaskQueryCmd.Flags().Lookup(want) == nil {
			t.Errorf("--%s flag missing", want)
		}
	}
	// output 默认 text
	output := attendanceUserTaskQueryCmd.Flags().Lookup("output")
	if output.DefValue != "text" {
		t.Fatalf("--output default = %q, want text", output.DefValue)
	}
	// ignore-invalid-users 默认 true
	ignore := attendanceUserTaskQueryCmd.Flags().Lookup("ignore-invalid-users")
	if ignore.DefValue != "true" {
		t.Fatalf("--ignore-invalid-users default = %q, want true", ignore.DefValue)
	}
}

// TestAttendanceUserTaskQueryRequiredFlags 校验 required flag 注解
func TestAttendanceUserTaskQueryRequiredFlags(t *testing.T) {
	for _, name := range []string{"user-ids", "start", "end"} {
		flag := attendanceUserTaskQueryCmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("--%s flag missing", name)
			continue
		}
		anno := flag.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		if len(anno) != 1 || anno[0] != "true" {
			t.Errorf("--%s should be marked required, got annotations %#v", name, flag.Annotations)
		}
	}
}
