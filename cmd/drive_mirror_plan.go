package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

// drive push/pull --dry-run 的差集计划。
//
// 计划预览类：需要凭据，只读列举远端文件夹（push 还只读扫描本地），按与真实执行相同的
// --if-exists / --on-duplicate-remote / --delete-* 规则算出每个条目的动作，但不上传、不下载、
// 不建文件夹、不删除。真实执行时删除阶段只在上传/下载阶段零失败时才会进行，计划里照样列出，
// 由 note 提示这一前提。

// 计划动作取值
const (
	drivePlanCreateFolder = "create_folder"
	drivePlanUpload       = "upload"
	drivePlanOverwrite    = "overwrite"
	drivePlanDownload     = "download"
	drivePlanSkip         = "skip"
	drivePlanConflict     = "conflict"
	drivePlanDeleteRemote = "delete_remote"
	drivePlanDeleteLocal  = "delete_local"
)

type driveMirrorPlanItem struct {
	RelPath   string `json:"rel_path"`
	Action    string `json:"action"`
	FileToken string `json:"file_token,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type driveMirrorPlan struct {
	Items []driveMirrorPlanItem
	Note  string
}

// planDrivePush 计算 push 的差集计划（与 drive push 真实执行的判定一致）。
func planDrivePush(localDirs []string, localFiles map[string]string, snapshots map[string]os.FileInfo,
	remoteFiles map[string]driveMirrorFile, folderCache map[string]string, ifExists string, deleteRemote bool) driveMirrorPlan {
	var plan driveMirrorPlan
	dirs := append([]string(nil), localDirs...)
	sort.Strings(dirs)
	for _, rel := range dirs {
		if _, ok := folderCache[rel]; ok {
			continue
		}
		plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanCreateFolder})
	}

	rels := make([]string, 0, len(localFiles))
	for rel := range localFiles {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		existing, has := remoteFiles[rel]
		switch {
		case !has:
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanUpload})
		case ifExists == driveMirrorIfExistsSkip:
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanSkip, FileToken: existing.FileToken, Reason: "远端已存在（--if-exists skip）"})
		case ifExists == driveMirrorIfExistsSmart && pushRemoteUpToDate(existing, snapshots[rel]):
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanSkip, FileToken: existing.FileToken, Reason: "远端不早于本地（--if-exists smart）"})
		default:
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanOverwrite, FileToken: existing.FileToken})
		}
	}

	if deleteRemote {
		remoteRels := make([]string, 0, len(remoteFiles))
		for rel := range remoteFiles {
			remoteRels = append(remoteRels, rel)
		}
		sort.Strings(remoteRels)
		for _, rel := range remoteRels {
			if _, has := localFiles[rel]; has {
				continue
			}
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanDeleteRemote, FileToken: remoteFiles[rel].FileToken})
		}
		plan.Note = "真实执行时 delete_remote 只在上传阶段零失败时进行；需加 --yes"
	}
	return plan
}

func pushRemoteUpToDate(remote driveMirrorFile, snap os.FileInfo) bool {
	if snap == nil {
		return false
	}
	c, ok := compareRemoteModifiedToLocal(remote.ModifiedTime, snap.ModTime())
	return ok && c >= 0
}

// planDrivePull 计算 pull 的差集计划（与 drive pull 真实执行的判定一致）；只读 stat/遍历本地，不写盘。
func planDrivePull(safeRoot string, remoteFiles map[string]driveMirrorFile, remotePaths map[string]struct{},
	ifExists string, deleteLocal bool) (driveMirrorPlan, error) {
	var plan driveMirrorPlan
	rels := make([]string, 0, len(remoteFiles))
	for rel := range remoteFiles {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		f := remoteFiles[rel]
		target, pathErr := safeMirrorTarget(safeRoot, rel)
		if pathErr != nil {
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanConflict, FileToken: f.FileToken, Reason: pathErr.Error()})
			continue
		}
		info, statErr := os.Stat(target)
		switch {
		case statErr != nil:
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanDownload, FileToken: f.FileToken})
		case info.IsDir():
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanConflict, FileToken: f.FileToken, Reason: "本地同路径是目录，远端是文件"})
		case ifExists == driveMirrorIfExistsSkip:
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanSkip, FileToken: f.FileToken, Reason: "本地已存在（--if-exists skip）"})
		case ifExists == driveMirrorIfExistsSmart && localUpToDate(info.ModTime(), f.ModifiedTime):
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanSkip, FileToken: f.FileToken, Reason: "本地不早于远端（--if-exists smart）"})
		default:
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanOverwrite, FileToken: f.FileToken})
		}
	}

	if deleteLocal {
		localFiles, err := walkLocalRegularFiles(safeRoot)
		if err != nil {
			return plan, err
		}
		locals := make([]string, 0, len(localFiles))
		for rel := range localFiles {
			locals = append(locals, rel)
		}
		sort.Strings(locals)
		for _, rel := range locals {
			if _, ok := remotePaths[rel]; ok {
				continue
			}
			plan.Items = append(plan.Items, driveMirrorPlanItem{RelPath: rel, Action: drivePlanDeleteLocal})
		}
		plan.Note = "真实执行时 delete_local 只在下载阶段零失败时进行；需加 --yes"
	}
	return plan, nil
}

// printDriveMirrorPlan 输出差集计划：-o json 为结构化 JSON，否则为文本摘要。
func printDriveMirrorPlan(cmd *cobra.Command, output, command, folderToken, localDir string, plan driveMirrorPlan) error {
	summary := map[string]int{}
	for _, it := range plan.Items {
		summary[it.Action]++
	}
	items := plan.Items
	if items == nil {
		items = []driveMirrorPlanItem{}
	}
	if output == "json" {
		payload := map[string]any{
			"dry_run":      true,
			"command":      command,
			"folder_token": folderToken,
			"local_dir":    localDir,
			"summary":      summary,
			"items":        items,
		}
		if plan.Note != "" {
			payload["note"] = plan.Note
		}
		return printJSON(payload)
	}
	fmt.Printf("[dry-run] %s 计划（未执行任何写操作）: folder=%s local=%s\n", command, folderToken, localDir)
	actions := make([]string, 0, len(summary))
	for a := range summary {
		actions = append(actions, a)
	}
	sort.Strings(actions)
	for _, a := range actions {
		fmt.Printf("  %-14s %d\n", a, summary[a])
	}
	for _, it := range plan.Items {
		if it.Action == drivePlanSkip {
			continue
		}
		line := fmt.Sprintf("  %-14s %s", it.Action, it.RelPath)
		if it.Reason != "" {
			line += " -- " + it.Reason
		}
		fmt.Println(line)
	}
	if plan.Note != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "注意：%s\n", plan.Note)
	}
	return nil
}
