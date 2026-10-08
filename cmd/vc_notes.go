package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var vcNotesCmd = &cobra.Command{
	Use:   "notes",
	Short: "获取会议纪要（三路径）",
	Long: `获取会议纪要文档引用与妙记信息。

支持三种输入方式（互斥，均支持逗号分隔批量，最多 50 条）:
  --meeting-ids         通过会议 ID 查
  --minute-tokens       通过妙记 token 查
  --calendar-event-ids  通过日历事件实例 ID 查（自动反查 meeting_ids + meeting_notes）

可选开关:
  --with-artifacts      额外获取 AI 产物（summary / todos / chapters）
  --download-transcript 下载逐字稿 txt 到 --output-dir
  --output-dir          逐字稿落盘目录（默认当前目录）
  --overwrite           覆盖已存在的逐字稿文件

权限:
  - 默认 User 身份（--as user），可用 --as bot|auto 切换
  - 基础: vc:note:read
  - meeting-ids 路径: vc:meeting.meetingevent:read
  - minute-tokens 路径: minutes:minutes:readonly
    - --with-artifacts: + minutes:minutes.artifacts:read
    - --download-transcript: + minutes:minutes.transcript:export
  - calendar-event-ids 路径: + calendar:calendar:read / calendar:calendar.event:read

示例:
  # 通过会议 ID 查
  feishu-cli vc notes --meeting-ids 69xxxx

  # 通过妙记 token 查 + 获取 AI 产物
  feishu-cli vc notes --minute-tokens obcnxxxx --with-artifacts

  # 从日历事件直达妙记并下载逐字稿
  feishu-cli vc notes --calendar-event-ids <event_id> --download-transcript --output-dir ./notes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		token, err := resolveVCReadIdentity(cmd)
		if err != nil {
			return err
		}

		meetingRaw, _ := cmd.Flags().GetString("meeting-ids")
		minuteRaw, _ := cmd.Flags().GetString("minute-tokens")
		calendarRaw, _ := cmd.Flags().GetString("calendar-event-ids")
		withArtifacts, _ := cmd.Flags().GetBool("with-artifacts")
		downloadTranscript, _ := cmd.Flags().GetBool("download-transcript")
		outputDir, _ := cmd.Flags().GetString("output-dir")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		output, _ := cmd.Flags().GetString("output")

		if err := exactlyOneNonEmpty(
			[]string{"meeting-ids", "minute-tokens", "calendar-event-ids"},
			[]string{meetingRaw, minuteRaw, calendarRaw},
		); err != nil {
			return err
		}

		if outputDir == "" {
			outputDir = "."
		}
		if downloadTranscript {
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("创建 --output-dir 失败: %w", err)
			}
		}

		opts := &notesOptions{
			Token:              token,
			WithArtifacts:      withArtifacts,
			DownloadTranscript: downloadTranscript,
			OutputDir:          outputDir,
			Overwrite:          overwrite,
			seenTranscripts:    make(map[string]string),
		}

		var items []vcBatchItem

		switch {
		case meetingRaw != "":
			ids, err := parseCSVIDs(meetingRaw, "meeting-ids")
			if err != nil {
				return err
			}
			items = runNotesBatch(ids, func(id string) (*noteView, error) {
				return processMeetingID(id, opts)
			})
		case minuteRaw != "":
			tokens, err := parseCSVIDs(minuteRaw, "minute-tokens")
			if err != nil {
				return err
			}
			for _, t := range tokens {
				if err := ensureMinuteToken(t); err != nil {
					return err
				}
			}
			items = runNotesBatch(tokens, func(mt string) (*noteView, error) {
				return processMinuteToken(mt, opts)
			})
		case calendarRaw != "":
			ids, err := parseCSVIDs(calendarRaw, "calendar-event-ids")
			if err != nil {
				return err
			}
			items = runCalendarNotesBatch(ids, opts)
		}

		summary := summarizeBatch(items)

		if output == "json" {
			return printJSON(map[string]any{
				"items":   items,
				"summary": summary,
			})
		}

		printNotesText(items, summary)

		if summary.Failed > 0 && summary.Succeeded == 0 {
			return fmt.Errorf("全部请求失败")
		}
		return nil
	},
}

// notesOptions vc notes 命令共享参数
type notesOptions struct {
	Token              string
	WithArtifacts      bool
	DownloadTranscript bool
	OutputDir          string
	Overwrite          bool
	seenTranscripts    map[string]string // minute_token → savedPath（批量去重）
}

// noteView vc notes 命令的单条输出视图
type noteView struct {
	Source          string   `json:"source"` // meeting_id / minute_token / calendar_event_id
	MeetingID       string   `json:"meeting_id,omitempty"`
	MinuteToken     string   `json:"minute_token,omitempty"`
	NoteID          string   `json:"note_id,omitempty"`
	NoteDisplayType string   `json:"note_display_type,omitempty"` // normal / unified / unknown
	Hint            string   `json:"hint,omitempty"`
	Title           string   `json:"title,omitempty"`
	MinuteURL       string   `json:"minute_url,omitempty"`
	CreateTime      string   `json:"create_time,omitempty"`
	NoteDoc         string   `json:"note_doc,omitempty"`
	VerbatimDoc     string   `json:"verbatim_doc,omitempty"`
	SharedDocs      []string `json:"shared_docs,omitempty"`
	Artifacts       any      `json:"artifacts,omitempty"`
	TranscriptPath  string   `json:"transcript_path,omitempty"`
}

// runNotesBatch 串行处理一批 ID
func runNotesBatch[T any](ids []string, fn func(string) (*T, error)) []vcBatchItem {
	out := make([]vcBatchItem, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			time.Sleep(vcBatchDelay)
		}
		data, err := fn(id)
		if err != nil {
			out = append(out, vcBatchItem{ID: id, OK: false, Error: err.Error()})
			continue
		}
		out = append(out, vcBatchItem{ID: id, OK: true, Data: data})
	}
	return out
}

// runCalendarNotesBatch 处理 calendar-event-ids 路径（先反查关联再逐个走 meeting/minute 路径）
func runCalendarNotesBatch(eventIDs []string, opts *notesOptions) []vcBatchItem {
	out := make([]vcBatchItem, 0, len(eventIDs))

	cal, err := client.GetPrimaryCalendar(opts.Token)
	if err != nil {
		// 全部事件标记失败
		for _, id := range eventIDs {
			out = append(out, vcBatchItem{ID: id, OK: false, Error: "获取主日历失败: " + err.Error()})
		}
		return out
	}

	rel, err := client.MgetInstanceRelationInfo(cal.CalendarID, eventIDs, true, opts.Token)
	if err != nil {
		for _, id := range eventIDs {
			out = append(out, vcBatchItem{ID: id, OK: false, Error: "查询事件关联失败: " + err.Error()})
		}
		return out
	}

	for _, eventID := range eventIDs {
		info := rel[eventID]
		if info == nil || (len(info.MeetingInstanceIDs) == 0 && len(info.MeetingNotes) == 0) {
			out = append(out, vcBatchItem{ID: eventID, OK: false, Error: "未找到关联的会议或妙记"})
			continue
		}

		// 聚合一个 event 的多条 noteView
		var subViews []*noteView

		// 先走 meeting_id 路径
		mids := dedupStrings(info.MeetingInstanceIDs)
		notesFromMeeting := make(map[string]struct{})
		for _, mid := range mids {
			time.Sleep(vcBatchDelay)
			v, err := processMeetingID(mid, opts)
			if err != nil {
				subViews = append(subViews, &noteView{
					Source:    "meeting_id",
					MeetingID: mid,
					Title:     "错误: " + err.Error(),
				})
				continue
			}
			v.Source = "meeting_id"
			subViews = append(subViews, v)
			if v.MinuteToken != "" {
				notesFromMeeting[v.MinuteToken] = struct{}{}
			}
		}

		// 再走 minute_token 路径（过滤掉 meeting 路径已经覆盖的 token）
		for _, mt := range dedupStrings(info.MeetingNotes) {
			if _, ok := notesFromMeeting[mt]; ok {
				continue
			}
			if err := ensureMinuteToken(mt); err != nil {
				subViews = append(subViews, &noteView{
					Source:      "minute_token",
					MinuteToken: mt,
					Title:       "错误: " + err.Error(),
				})
				continue
			}
			time.Sleep(vcBatchDelay)
			v, err := processMinuteToken(mt, opts)
			if err != nil {
				subViews = append(subViews, &noteView{
					Source:      "minute_token",
					MinuteToken: mt,
					Title:       "错误: " + err.Error(),
				})
				continue
			}
			v.Source = "minute_token"
			subViews = append(subViews, v)
		}

		out = append(out, vcBatchItem{
			ID:   eventID,
			OK:   len(subViews) > 0,
			Data: subViews,
		})
	}
	return out
}

// processMeetingID meeting-id 路径处理
func processMeetingID(meetingID string, opts *notesOptions) (*noteView, error) {
	data, err := client.GetMeeting(meetingID, opts.Token)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Meeting struct {
			MeetingID string `json:"meeting_id"`
			Topic     string `json:"topic"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			NoteID    string `json:"note_id"`
		} `json:"meeting"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("解析会议信息失败: %w", err)
	}

	view := &noteView{
		Source:     "meeting_id",
		MeetingID:  meetingID,
		Title:      parsed.Meeting.Topic,
		CreateTime: formatVCTime(parsed.Meeting.StartTime),
	}
	// 与官方 vc +notes 一致：无论有无纪要，都尝试通过录制接口拿 minute_token，
	// 方便后续 minutes get / vc notes --minute-tokens（best-effort，失败记入 hint）
	if recData, recErr := client.GetMeetingRecording(meetingID, opts.Token); recErr != nil {
		view.addHint(recErr.Error())
	} else if rv := parseRecordingData(recData); rv.MinuteToken != "" {
		view.MinuteToken = rv.MinuteToken
	}
	if parsed.Meeting.NoteID == "" {
		view.addHint("该会议没有智能纪要")
		return view, nil
	}
	applyNoteDocs(view, parsed.Meeting.NoteID, opts)
	return view, nil
}

// addHint 追加一条说明（多条用 "; " 连接）
func (v *noteView) addHint(msg string) {
	if msg == "" {
		return
	}
	if v.Hint == "" {
		v.Hint = msg
		return
	}
	v.Hint += "; " + msg
}

// processMinuteToken minute-token 路径处理
//
// GetMinute（minutes:minutes:readonly）失败时不会立即返回错误：
// 仅 --download-transcript 场景只需 transcript:export 权限，不依赖 minute 基础信息，
// 这种情况下退化为最小 view 让 transcript 下载流程仍能跑通。
func processMinuteToken(minuteToken string, opts *notesOptions) (*noteView, error) {
	view := &noteView{
		Source:      "minute_token",
		MinuteToken: minuteToken,
	}

	data, err := client.GetMinute(minuteToken, opts.Token)
	if err != nil {
		if !opts.DownloadTranscript {
			return nil, err
		}
		view.Title = minuteToken
	} else {
		var parsed struct {
			Minute struct {
				Title      string `json:"title"`
				URL        string `json:"url"`
				NoteID     string `json:"note_id"`
				CreateTime string `json:"create_time"`
				Duration   string `json:"duration"`
			} `json:"minute"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("解析妙记信息失败: %w", err)
		}
		view.Title = parsed.Minute.Title
		view.MinuteURL = parsed.Minute.URL
		view.CreateTime = formatVCTime(parsed.Minute.CreateTime)
		if parsed.Minute.NoteID != "" {
			applyNoteDocs(view, parsed.Minute.NoteID, opts)
		}
	}

	if opts.WithArtifacts {
		if art, err := client.GetMinuteArtifacts(minuteToken, opts.Token); err != nil {
			view.Artifacts = map[string]string{"error": err.Error()}
		} else {
			var artData any
			if err := json.Unmarshal(art, &artData); err != nil {
				view.Artifacts = map[string]string{"error": "解析 artifacts 失败: " + err.Error()}
			} else {
				view.Artifacts = artData
			}
		}
	}

	if opts.DownloadTranscript {
		if existing, ok := opts.seenTranscripts[minuteToken]; ok {
			view.TranscriptPath = existing
		} else {
			path, err := downloadTranscriptFile(minuteToken, view.Title, opts)
			if err != nil {
				view.TranscriptPath = "下载失败: " + err.Error()
			} else {
				view.TranscriptPath = path
				opts.seenTranscripts[minuteToken] = path
			}
		}
	}

	return view, nil
}

// applyNoteDocs 调用 GetMeetingNote 并把纪要类型、文档 token 填入 view；
// 失败（如 121005 无纪要权限）不中断整条结果，记入 hint。
func applyNoteDocs(view *noteView, noteID string, opts *notesOptions) {
	view.NoteID = noteID
	data, err := client.GetMeetingNote(noteID, opts.Token)
	if err != nil {
		view.addHint(decorateNoteError(err).Error())
		return
	}
	d, err := parseNoteDetail(noteID, data)
	if err != nil {
		view.addHint(err.Error())
		return
	}
	view.NoteDisplayType = d.displayTypeName()
	if view.CreateTime == "" && d.CreateTime != "" {
		view.CreateTime = formatVCTime(d.CreateTime)
	}
	view.NoteDoc = d.NoteDocToken
	view.VerbatimDoc = d.VerbatimDocToken
	view.SharedDocs = append(view.SharedDocs, d.SharedDocTokens...)
}

// downloadTranscriptFile 下载逐字稿到 {outputDir}/artifact-{sanitizedTitle}-{token}/transcript.txt
func downloadTranscriptFile(minuteToken, title string, opts *notesOptions) (string, error) {
	body, err := client.GetMinuteTranscript(minuteToken, opts.Token)
	if err != nil {
		return "", err
	}

	sanitized := safeOutputPath(title, "")
	if sanitized == "" {
		sanitized = "untitled"
	}
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}
	dirName := fmt.Sprintf("artifact-%s-%s", sanitized, minuteToken)
	dir := filepath.Join(opts.OutputDir, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	path := filepath.Join(dir, "transcript.txt")
	if _, statErr := os.Stat(path); statErr == nil && !opts.Overwrite {
		return "", fmt.Errorf("文件已存在: %s（使用 --overwrite 覆盖）", path)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("写文件失败: %w", err)
	}
	return path, nil
}

// printNotesText 按文本模式打印 items
func printNotesText(items []vcBatchItem, summary vcBatchSummary) {
	for i, it := range items {
		fmt.Printf("[%d] %s\n", i+1, it.ID)
		if !it.OK {
			fmt.Printf("    FAIL: %s\n", it.Error)
			continue
		}
		switch v := it.Data.(type) {
		case *noteView:
			printOneNoteView(v, "    ")
		case []*noteView:
			for j, sub := range v {
				fmt.Printf("    (%d)\n", j+1)
				printOneNoteView(sub, "        ")
			}
		default:
			fmt.Printf("    (未知数据类型)\n")
		}
		fmt.Println()
	}
	fmt.Printf("合计: %d / 成功 %d / 失败 %d\n", summary.Total, summary.Succeeded, summary.Failed)
}

func printOneNoteView(v *noteView, indent string) {
	if v.Title != "" {
		fmt.Printf("%s标题:        %s\n", indent, v.Title)
	}
	if v.MeetingID != "" {
		fmt.Printf("%smeeting_id:  %s\n", indent, v.MeetingID)
	}
	if v.MinuteToken != "" {
		fmt.Printf("%sminute_token:%s\n", indent, v.MinuteToken)
	}
	if v.NoteID != "" {
		fmt.Printf("%snote_id:     %s\n", indent, v.NoteID)
	}
	if v.NoteDisplayType != "" {
		fmt.Printf("%s纪要类型:    %s\n", indent, v.NoteDisplayType)
	}
	if v.CreateTime != "" {
		fmt.Printf("%screate_time: %s\n", indent, v.CreateTime)
	}
	if v.MinuteURL != "" {
		fmt.Printf("%sminute_url:  %s\n", indent, v.MinuteURL)
	}
	if v.NoteDoc != "" {
		fmt.Printf("%snote_doc:    %s\n", indent, v.NoteDoc)
	}
	if v.VerbatimDoc != "" {
		fmt.Printf("%sverbatim:    %s\n", indent, v.VerbatimDoc)
	}
	if len(v.SharedDocs) > 0 {
		fmt.Printf("%sshared_docs: %s\n", indent, strings.Join(v.SharedDocs, ", "))
	}
	if v.TranscriptPath != "" {
		fmt.Printf("%stranscript:  %s\n", indent, v.TranscriptPath)
	}
	if v.Hint != "" {
		fmt.Printf("%s说明:        %s\n", indent, v.Hint)
	}
	if v.Artifacts != nil {
		if b, err := json.MarshalIndent(v.Artifacts, indent, "  "); err == nil {
			fmt.Printf("%sartifacts:\n%s%s\n", indent, indent, string(b))
		}
	}
}

func init() {
	vcCmd.AddCommand(vcNotesCmd)
	vcNotesCmd.Flags().String("meeting-ids", "", "会议 ID 列表，逗号分隔（最多 50 条）")
	vcNotesCmd.Flags().String("minute-tokens", "", "妙记 token 列表，逗号分隔（最多 50 条）")
	vcNotesCmd.Flags().String("calendar-event-ids", "", "日历事件实例 ID 列表，逗号分隔（最多 50 条）")
	vcNotesCmd.Flags().Bool("with-artifacts", false, "获取 AI 产物（summary/todos/chapters）")
	vcNotesCmd.Flags().Bool("download-transcript", false, "下载逐字稿 txt 到 --output-dir")
	vcNotesCmd.Flags().String("output-dir", "", "逐字稿输出目录（默认当前目录）")
	vcNotesCmd.Flags().Bool("overwrite", false, "覆盖已存在的逐字稿文件")
	vcNotesCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	vcNotesCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
	addVCReadAsFlag(vcNotesCmd)
}
