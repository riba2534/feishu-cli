package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// ==================== table copy / copy-status：复制单张数据表 ====================
// 对齐官方 base +table-copy / +table-copy-status（shortcuts/base/table_copy*.go）：
//   POST /bases/{bt}/tables/{table}/copy      body {name, range}；range=schema 同步完成，range=all 返回异步 task_id
//   POST /bases/{bt}/copy_table_state         body {task_id} 查询一次任务状态
// 提交请求的响应丢失（网络错误）时结果未知：目标表可能已建好，禁止自动重试。

const (
	tableCopyRangeSchema = "schema"
	tableCopyRangeAll    = "all"
	tableCopyTimeoutMax  = 30 * time.Minute
	tableCopyTaskIDMax   = 1024
	tableCopyStateInit   = "init"
	tableCopyStateProc   = "process"
	tableCopyStateOK     = "success"
	// tableCopyInvalidTaskCode copy_table_state 对无效 task_id 的业务码（官方归为参数错误）。
	tableCopyInvalidTaskCode = 800010109
)

// 轮询间隔：3s 起步指数退避，封顶 30s（测试可替换）。
var (
	tableCopyPollInitial = 3 * time.Second
	tableCopyPollMax     = 30 * time.Second
)

var bitableTableCopyCmd = &cobra.Command{
	Use:   "copy",
	Short: "复制单张数据表（默认只复制结构）",
	Long: `POST /open-apis/base/v3/bases/{base_token}/tables/{table_id}/copy

在同一个多维表格内复制一张数据表（复制整个多维表格用 bitable copy）。

必填:
  --table-id   源数据表 ID 或名称
  --name       新数据表名称
可选:
  --range      schema（默认，只复制结构，同步完成）| all（连同记录，异步任务）
  --wait       仅 --range all：本地轮询任务直到完成（3s 起指数退避，封顶 30s）
  --timeout    --wait 的最长等待时间（默认 5m，上限 30m）

输出 {table:{id,name},range,state,completed,task_id,...}；任务未完成时带 next_command，
用 bitable table copy-status 续查。提交请求遇网络错误时结果未知（表可能已建好）：
不要直接重试，先用 bitable table list 确认。scope: base:table:create

示例:
  feishu-cli bitable table copy --base-token bascnxxx --table-id 任务 --name "任务 副本"
  feishu-cli bitable table copy --base-token bascnxxx --table-id tblxxx --name "任务 全量" --range all --wait`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tableID, _ := cmd.Flags().GetString("table-id")
		tableID = strings.TrimSpace(tableID)
		name, _ := cmd.Flags().GetString("name")
		rangeValue, _ := cmd.Flags().GetString("range")
		wait, _ := cmd.Flags().GetBool("wait")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		if err := validateTableCopyArgs(cmd, tableID, name, rangeValue, wait, timeout); err != nil {
			return err
		}
		body := map[string]any{"name": name, "range": rangeValue}
		baseToken := ""
		return bitableExecute(cmd, func(bt string) bitableReq {
			baseToken = bt
			req := bitableReq{method: "POST", path: bitableTablePath(bt, tableID, "copy"), body: body}
			if wait {
				req.steps = []map[string]any{
					{"method": "POST", "path": req.path, "body": body, "desc": "[1] 提交数据表复制"},
					{"method": "POST", "path": client.BaseV3Path("bases", bt, "copy_table_state"), "body": map[string]any{"task_id": "<task_id_from_step_1>"}, "desc": "[2] 轮询任务状态：3s 起指数退避，封顶 30s"},
				}
				req.preview = map[string]any{"wait": true, "timeout": timeout.String()}
			}
			return req
		}, func(req bitableReq, token string) (any, error) {
			data, err := client.BaseV3Call(req.method, req.path, nil, req.body, token)
			if err != nil {
				return nil, tableCopySubmitError(err)
			}
			submit, err := projectTableCopySubmit(data)
			if err != nil {
				return nil, err
			}
			out := map[string]any{"table": submit.table, "range": rangeValue, "state": submit.state, "completed": submit.state == tableCopyStateOK}
			if submit.taskID != "" {
				out["task_id"] = submit.taskID
			}
			if submit.state == tableCopyStateOK {
				fmt.Fprintln(os.Stderr, "数据表复制完成: success")
				return out, nil
			}
			if rangeValue == tableCopyRangeSchema {
				return nil, fmt.Errorf("结构复制返回了非 success 状态 %q", submit.state)
			}
			if submit.taskID == "" {
				return nil, fmt.Errorf("全量复制的响应缺少 task_id")
			}
			next := tableCopyNextCommand(cmd, baseToken, submit.taskID)
			if !wait {
				out["next_action"] = "poll_status"
				out["next_command"] = next
				fmt.Fprintln(os.Stderr, "数据表复制在后台执行；用输出中的 next_command 查询进度")
				return out, nil
			}
			fmt.Fprintf(os.Stderr, "数据表复制已提交: %s, task_id=%s\n", submit.state, submit.taskID)
			state, timedOut, perr := pollTableCopy(client.Context(), timeout, func() (string, error) {
				st, err := queryTableCopyState(baseToken, submit.taskID, token)
				if err != nil {
					return "", err
				}
				fmt.Fprintf(os.Stderr, "数据表复制状态: %s\n", st.state)
				return st.state, nil
			})
			if state == "" {
				state = submit.state
			}
			out["state"] = state
			out["completed"] = state == tableCopyStateOK
			if perr != nil {
				// 任务已提交：先把可续查的坐标打到 stdout，再返回错误
				out["next_action"] = "poll_status"
				out["next_command"] = next
				if rerr := printJSON(out); rerr != nil {
					return nil, rerr
				}
				return nil, fmt.Errorf("轮询复制任务状态失败（任务已提交，请勿重复提交；用 next_command 续查）: %w", perr)
			}
			if timedOut {
				out["timed_out"] = true
			}
			if state != tableCopyStateOK {
				out["next_action"] = "poll_status"
				out["next_command"] = next
				fmt.Fprintln(os.Stderr, "数据表复制尚未完成；用输出中的 next_command 继续查询")
			}
			return out, nil
		})
	},
}

func validateTableCopyArgs(cmd *cobra.Command, tableID, name, rangeValue string, wait bool, timeout time.Duration) error {
	if tableID == "" {
		return clierr.Usagef("--table-id 必填")
	}
	if strings.TrimSpace(name) == "" {
		return clierr.Usagef("--name 必填")
	}
	if err := validateEnum(rangeValue, " --range", []string{tableCopyRangeSchema, tableCopyRangeAll}); err != nil {
		return err
	}
	timeoutChanged := cmd.Flags().Changed("timeout")
	if wait && rangeValue != tableCopyRangeAll {
		return clierr.Usagef("--wait 需要配合 --range all（结构复制是同步完成的）")
	}
	if timeoutChanged && !wait {
		return clierr.Usagef("--timeout 需要配合 --wait（以及 --range all）")
	}
	if wait && (timeout <= 0 || timeout > tableCopyTimeoutMax) {
		return clierr.Usagef("--timeout 必须大于 0 且不超过 30m，当前 %s", timeout)
	}
	return nil
}

// tableCopySubmitError 提交请求的响应丢失（网络错误）时，复制结果未知：明确告知不要重试。
// 保留网络错误链（exit 4），但文案强调先人工确认，避免重复建表。
func tableCopySubmitError(err error) error {
	if !isNetworkError(err) {
		return err
	}
	return fmt.Errorf("提交数据表复制时网络中断，结果未知：目标数据表可能已经创建。请勿直接重试，先用 `feishu-cli bitable table list` 确认是否已有新表再决定: %w", err)
}

type tableCopySubmit struct {
	table  map[string]any
	taskID string
	state  string
}

func projectTableCopySubmit(data map[string]any) (tableCopySubmit, error) {
	tbl, _ := data["table"].(map[string]any)
	out := tableCopySubmit{
		table:  map[string]any{"id": bitableStr(tbl, "id")},
		taskID: bitableStr(data, "task_id"),
		state:  strings.ToLower(bitableStr(data, "state")),
	}
	if name := bitableStr(tbl, "name"); name != "" {
		out.table["name"] = name
	}
	if out.table["id"] == "" {
		return out, fmt.Errorf("数据表复制响应缺少 table.id")
	}
	if len(out.taskID) > tableCopyTaskIDMax {
		return out, fmt.Errorf("数据表复制响应的 task_id 超过 %d 字节", tableCopyTaskIDMax)
	}
	switch out.state {
	case tableCopyStateInit, tableCopyStateProc, tableCopyStateOK:
		return out, nil
	default:
		return out, fmt.Errorf("数据表复制响应的状态无效: %q", out.state)
	}
}

type tableCopyState struct {
	tableID string
	state   string
}

func queryTableCopyState(baseToken, taskID, token string) (tableCopyState, error) {
	data, err := client.BaseV3Call("POST", client.BaseV3Path("bases", baseToken, "copy_table_state"), nil, map[string]any{"task_id": taskID}, token)
	if err != nil {
		if client.HasAPICode(err, tableCopyInvalidTaskCode) {
			return tableCopyState{}, clierr.Usage(fmt.Errorf("--task-id 无效: %w", err))
		}
		return tableCopyState{}, err
	}
	st := tableCopyState{tableID: bitableStr(data, "table_id"), state: strings.ToLower(bitableStr(data, "state"))}
	if st.tableID == "" {
		return st, fmt.Errorf("复制任务状态响应缺少 table_id")
	}
	switch st.state {
	case tableCopyStateInit, tableCopyStateProc, tableCopyStateOK:
		return st, nil
	case "failed":
		return st, fmt.Errorf("数据表复制任务失败（state=failed）")
	default:
		return st, fmt.Errorf("复制任务状态无效: %q", st.state)
	}
}

// pollTableCopy 轮询直到 success、超时或 ctx 取消。网络类错误视为瞬时、继续轮询；其余错误立即返回。
// 返回最后一次拿到的状态与是否超时。
func pollTableCopy(ctx context.Context, timeout time.Duration, fetch func() (string, error)) (string, bool, error) {
	deadline := time.Now().Add(timeout)
	delay := tableCopyPollInitial
	last := ""
	var lastErr error
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if last == "" && lastErr != nil {
				return "", false, lastErr
			}
			return last, true, nil
		}
		if delay > remaining {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, false, ctx.Err()
		case <-timer.C:
		}
		state, err := fetch()
		if err != nil {
			if !isNetworkError(err) {
				return last, false, err
			}
			lastErr = err
		} else {
			last = state
			if state == tableCopyStateOK {
				return state, false, nil
			}
		}
		delay *= 2
		if delay > tableCopyPollMax {
			delay = tableCopyPollMax
		}
	}
}

// tableCopyNextCommand 生成续查命令（保留 --profile 与 --as，值按 shell 安全引用）。
func tableCopyNextCommand(cmd *cobra.Command, baseToken, taskID string) string {
	parts := []string{"feishu-cli"}
	if profileFlag != "" {
		parts = append(parts, "--profile", shellQuoteArg(profileFlag))
	}
	as, _ := cmd.Flags().GetString("as")
	if strings.TrimSpace(as) == "" {
		as = "auto"
	}
	parts = append(parts, "bitable", "table", "copy-status",
		"--base-token", shellQuoteArg(baseToken), "--task-id", shellQuoteArg(taskID), "--as", shellQuoteArg(as))
	return strings.Join(parts, " ")
}

// shellQuoteArg 只含安全字符时原样返回，否则用单引号包裹。
func shellQuoteArg(v string) string {
	if v != "" && strings.IndexFunc(v, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("._~-", r)
	}) == -1 {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'"'"'`) + "'"
}

var bitableTableCopyStatusCmd = &cobra.Command{
	Use:   "copy-status",
	Short: "查询一次数据表复制任务的状态",
	Long: `POST /open-apis/base/v3/bases/{base_token}/copy_table_state  body {"task_id":"..."}

用 bitable table copy 返回的 task_id 查询一次任务状态；state 为 init/process 时稍后再查。
无效的 task_id（800010109）按参数错误返回（exit 2）。scope: base:table:create

示例:
  feishu-cli bitable table copy-status --base-token bascnxxx --task-id <task_id>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID, _ := cmd.Flags().GetString("task-id")
		if strings.TrimSpace(taskID) == "" {
			return clierr.Usagef("--task-id 必填")
		}
		if len(taskID) > tableCopyTaskIDMax {
			return clierr.Usagef("--task-id 不能超过 %d 字节", tableCopyTaskIDMax)
		}
		resolvedBT := ""
		return bitableExecute(cmd, func(bt string) bitableReq {
			resolvedBT = bt
			return bitableReq{method: "POST", path: client.BaseV3Path("bases", bt, "copy_table_state"), body: map[string]any{"task_id": taskID}}
		}, func(req bitableReq, token string) (any, error) {
			st, err := queryTableCopyState(resolvedBT, taskID, token)
			if err != nil {
				return nil, err
			}
			out := map[string]any{"table": map[string]any{"id": st.tableID}, "state": st.state, "completed": st.state == tableCopyStateOK, "task_id": taskID}
			if st.state != tableCopyStateOK {
				out["next_action"] = "poll_status"
				out["next_command"] = tableCopyNextCommand(cmd, resolvedBT, taskID)
			}
			fmt.Fprintf(os.Stderr, "数据表复制状态: %s\n", st.state)
			return out, nil
		})
	},
}

func init() {
	bitableTableCmd.AddCommand(bitableTableCopyCmd)
	addBitableWriteFlags(bitableTableCopyCmd)
	bitableTableCopyCmd.Flags().String("table-id", "", "源数据表 ID 或名称（必填）")
	bitableTableCopyCmd.Flags().String("name", "", "新数据表名称（必填）")
	bitableTableCopyCmd.Flags().String("range", tableCopyRangeSchema, "复制范围: schema（只复制结构）| all（连同记录，异步）")
	bitableTableCopyCmd.Flags().Bool("wait", false, "仅 --range all：等待复制任务完成")
	bitableTableCopyCmd.Flags().Duration("timeout", 5*time.Minute, "--wait 的最长等待时间（上限 30m）")
	mustMarkFlagRequired(bitableTableCopyCmd, "table-id", "name")

	bitableTableCmd.AddCommand(bitableTableCopyStatusCmd)
	addBitableWriteFlags(bitableTableCopyStatusCmd)
	bitableTableCopyStatusCmd.Flags().String("task-id", "", "table copy 返回的 task_id（必填）")
	mustMarkFlagRequired(bitableTableCopyStatusCmd, "task-id")
}
