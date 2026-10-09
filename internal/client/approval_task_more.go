package client

import (
	"fmt"
	"strings"
)

// 审批任务退回 / 加签 / 催办（对齐官方 lark-approval tasks rollback / add_sign / remind）
const (
	approvalTaskRollbackPath = "/open-apis/approval/v4/tasks/rollback"
	approvalTaskAddSignPath  = "/open-apis/approval/v4/tasks/add_sign"
	approvalInstanceRemind   = "/open-apis/approval/v4/instances/remind"
)

// RollbackApprovalTaskOptions 退回审批任务
type RollbackApprovalTaskOptions struct {
	InstanceCode string
	TaskID       string
	NodeIDs      []string // 退回目标节点 ID；发起节点为 START
	Comment      string
}

// AddSignApprovalTaskOptions 审批任务加签
type AddSignApprovalTaskOptions struct {
	InstanceCode   string
	TaskID         string
	AddSignType    int      // 1 前加签 / 2 后加签 / 3 并加签
	AddSignUserIDs []string // 被加签人，类型与 UserIDType 一致
	ApprovalMethod int      // 1 或签 / 2 会签 / 3 依次审批；仅前/后加签需要，0 表示不传
	Comment        string
	UserIDType     string
}

// RemindApprovalTaskOptions 催办审批任务
type RemindApprovalTaskOptions struct {
	InstanceCode string
	TaskIDs      []string
	Comment      string
}

// BuildRollbackApprovalTaskBody 构造退回请求体（dry-run 与执行共用）
func BuildRollbackApprovalTaskBody(opts RollbackApprovalTaskOptions) (map[string]any, error) {
	ic, tid := strings.TrimSpace(opts.InstanceCode), strings.TrimSpace(opts.TaskID)
	if ic == "" || tid == "" {
		return nil, fmt.Errorf("instance_code 与 task_id 都不能为空")
	}
	if len(opts.NodeIDs) == 0 {
		return nil, fmt.Errorf("node_ids 不能为空（退回到发起节点传 START）")
	}
	body := map[string]any{"instance_code": ic, "task_id": tid, "node_ids": opts.NodeIDs}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	return body, nil
}

// BuildAddSignApprovalTaskBody 构造加签请求体并校验枚举组合
func BuildAddSignApprovalTaskBody(opts AddSignApprovalTaskOptions) (map[string]any, string, error) {
	ic, tid := strings.TrimSpace(opts.InstanceCode), strings.TrimSpace(opts.TaskID)
	if ic == "" || tid == "" {
		return nil, "", fmt.Errorf("instance_code 与 task_id 都不能为空")
	}
	if opts.AddSignType < 1 || opts.AddSignType > 3 {
		return nil, "", fmt.Errorf("add_sign_type 仅支持 1(前加签) / 2(后加签) / 3(并加签)")
	}
	if len(opts.AddSignUserIDs) == 0 {
		return nil, "", fmt.Errorf("add_sign_user_ids 不能为空")
	}
	if opts.AddSignType == 3 && opts.ApprovalMethod != 0 {
		return nil, "", fmt.Errorf("并加签（add_sign_type=3）不需要 approval_method")
	}
	if opts.AddSignType != 3 {
		if opts.ApprovalMethod == 0 {
			if len(opts.AddSignUserIDs) > 1 {
				return nil, "", fmt.Errorf("多人前/后加签必须指定 approval_method：1 或签 / 2 会签 / 3 依次审批")
			}
			// 单人时或签与会签效果相同（对齐官方技能约定）
			opts.ApprovalMethod = 1
		}
		if opts.ApprovalMethod < 1 || opts.ApprovalMethod > 3 {
			return nil, "", fmt.Errorf("approval_method 仅支持 1(或签) / 2(会签) / 3(依次审批)")
		}
	}
	userIDType, err := normalizeApprovalWriteUserIDType(opts.UserIDType)
	if err != nil {
		return nil, "", err
	}
	body := map[string]any{
		"instance_code":     ic,
		"task_id":           tid,
		"add_sign_type":     opts.AddSignType,
		"add_sign_user_ids": opts.AddSignUserIDs,
	}
	if opts.ApprovalMethod != 0 {
		body["approval_method"] = opts.ApprovalMethod
	}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	return body, userIDType, nil
}

// BuildRemindApprovalTaskBody 构造催办请求体
func BuildRemindApprovalTaskBody(opts RemindApprovalTaskOptions) (map[string]any, error) {
	ic := strings.TrimSpace(opts.InstanceCode)
	if ic == "" {
		return nil, fmt.Errorf("instance_code 不能为空")
	}
	if len(opts.TaskIDs) == 0 {
		return nil, fmt.Errorf("task_ids 不能为空")
	}
	body := map[string]any{"instance_code": ic, "task_ids": opts.TaskIDs}
	if opts.Comment != "" {
		body["comment"] = opts.Comment
	}
	return body, nil
}

// ApprovalTaskRollbackPath / AddSignPath / RemindPath 供 dry-run 展示
func ApprovalTaskRollbackPath() string { return approvalTaskRollbackPath }
func ApprovalTaskAddSignPath() string  { return approvalTaskAddSignPath }
func ApprovalTaskRemindPath() string   { return approvalInstanceRemind }

// RollbackApprovalTask 退回审批任务到指定节点（User Token）
func RollbackApprovalTask(opts RollbackApprovalTaskOptions, userAccessToken string) error {
	body, err := BuildRollbackApprovalTaskBody(opts)
	if err != nil {
		return err
	}
	_, err = doApprovalUserPost(approvalTaskRollbackPath, body, "", userAccessToken, "退回审批任务")
	return err
}

// AddSignApprovalTask 审批任务加签（User Token）
func AddSignApprovalTask(opts AddSignApprovalTaskOptions, userAccessToken string) error {
	body, userIDType, err := BuildAddSignApprovalTaskBody(opts)
	if err != nil {
		return err
	}
	_, err = doApprovalUserPost(approvalTaskAddSignPath, body, userIDType, userAccessToken, "审批任务加签")
	return err
}

// RemindApprovalTask 催办审批任务（User Token）
func RemindApprovalTask(opts RemindApprovalTaskOptions, userAccessToken string) error {
	body, err := BuildRemindApprovalTaskBody(opts)
	if err != nil {
		return err
	}
	_, err = doApprovalUserPost(approvalInstanceRemind, body, "", userAccessToken, "催办审批任务")
	return err
}

// ApprovalWritePreview 审批写操作的请求预览（dry-run 用，不联网）
type ApprovalWritePreview struct {
	Path   string
	Params map[string]any
	Body   map[string]any
}

// PreviewApprovalWrite 按动作构造请求预览：approve / reject / transfer / cancel / cc / create
func PreviewApprovalWrite(action string, opts any) (*ApprovalWritePreview, error) {
	withType := func(path string, body map[string]any, t string, err error) (*ApprovalWritePreview, error) {
		if err != nil {
			return nil, err
		}
		p := &ApprovalWritePreview{Path: path, Body: body}
		if t != "" {
			p.Params = map[string]any{"user_id_type": t}
		}
		return p, nil
	}
	switch o := opts.(type) {
	case ApprovalTaskActionOptions:
		switch action {
		case "approve":
			b, err := buildApprovalTaskActionBody(o, true)
			return withType(approvalTaskPassPath, b, "", err)
		case "reject":
			b, err := buildApprovalTaskActionBody(o, false)
			return withType(approvalTaskRefusePath, b, "", err)
		}
	case TransferApprovalTaskOptions:
		b, t, err := buildTransferApprovalTaskBody(o)
		return withType(approvalTaskForwardPath, b, t, err)
	case CancelApprovalInstanceOptions:
		b, err := buildCancelApprovalInstanceBody(o)
		return withType(approvalInstanceRecallPath, b, "", err)
	case CCApprovalInstanceOptions:
		b, t, err := buildCCApprovalInstanceBody(o)
		return withType(approvalInstanceAddCCPath, b, t, err)
	case CreateApprovalInstanceOptions:
		b, err := buildCreateApprovalInstanceBody(o)
		return withType(approvalInstanceInitiatePath, b, "", err)
	}
	return nil, fmt.Errorf("不支持预览的审批动作 %q", action)
}
