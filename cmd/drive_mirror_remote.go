package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

// 远端同一相对路径出现多个条目时的处理策略（--on-duplicate-remote）。
const (
	driveDuplicateRemoteFail   = "fail"
	driveDuplicateRemoteRename = "rename" // 仅 pull：多余的副本以稳定哈希后缀另存
	driveDuplicateRemoteNewest = "newest"
	driveDuplicateRemoteOldest = "oldest"
)

const driveMirrorIfExistsSmart = "smart"

// driveMirrorFile 是镜像视图中的一个远端 type=file 条目。
type driveMirrorFile struct {
	FileToken    string
	ModifiedTime string
	// SourceID 仅 rename 策略下的重名副本使用（稳定哈希标识，避免在输出中暴露被改名的 token 归属歧义）
	SourceID string
}

// driveMirrorView 是远端 listing 的两种视图：
//   - Files：可下载/可比较的 type=file 子集（rel_path → 文件）；
//   - Paths：远端占用的全部 rel_path（含 folder/在线文档/shortcut），--delete-local 用来判断"本地孤儿"。
type driveMirrorView struct {
	Files   map[string]driveMirrorFile
	Paths   map[string]struct{}
	Folders map[string]string // rel_path → folder_token
}

func validateDuplicateRemotePolicy(policy string, allowRename bool) error {
	switch policy {
	case driveDuplicateRemoteFail, driveDuplicateRemoteNewest, driveDuplicateRemoteOldest:
		return nil
	case driveDuplicateRemoteRename:
		if allowRename {
			return nil
		}
	}
	allowed := "fail|newest|oldest"
	if allowRename {
		allowed = "fail|rename|newest|oldest"
	}
	return clierr.Usagef("--on-duplicate-remote 只能是 %s，得到 %q", allowed, policy)
}

// buildDriveMirrorView 按重名策略把远端条目整理成镜像视图。
//
// 重名分两类：
//   - 全部是 type=file 的重名：按策略处理（fail 报错 / newest|oldest 选一个 / rename 全部保留并给副本加后缀）；
//   - 涉及 folder/在线文档的重名：无法安全合并，任何策略下都报错。
func buildDriveMirrorView(entries []client.DriveRemoteEntry, policy string) (*driveMirrorView, error) {
	groups := make(map[string][]client.DriveRemoteEntry)
	var order []string
	for _, e := range entries {
		if _, ok := groups[e.RelPath]; !ok {
			order = append(order, e.RelPath)
		}
		groups[e.RelPath] = append(groups[e.RelPath], e)
	}
	sort.Strings(order)

	view := &driveMirrorView{
		Files:   make(map[string]driveMirrorFile),
		Paths:   make(map[string]struct{}, len(entries)),
		Folders: make(map[string]string),
	}
	occupied := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		occupied[e.RelPath] = struct{}{}
	}

	var conflicts []string
	for _, rel := range order {
		group := groups[rel]
		view.Paths[rel] = struct{}{}
		if len(group) == 1 {
			e := group[0]
			switch e.Type {
			case "file":
				view.Files[rel] = driveMirrorFile{FileToken: e.FileToken, ModifiedTime: e.ModifiedTime}
			case "folder":
				view.Folders[rel] = e.FileToken
			}
			continue
		}
		allFiles := true
		for _, e := range group {
			if e.Type != "file" {
				allFiles = false
				break
			}
		}
		if !allFiles || policy == driveDuplicateRemoteFail {
			conflicts = append(conflicts, describeDuplicate(rel, group))
			continue
		}
		switch policy {
		case driveDuplicateRemoteNewest, driveDuplicateRemoteOldest:
			chosen := chooseDuplicateFile(group, policy)
			view.Files[rel] = driveMirrorFile{FileToken: chosen.FileToken, ModifiedTime: chosen.ModifiedTime}
		case driveDuplicateRemoteRename:
			sorted := append([]client.DriveRemoteEntry(nil), group...)
			sortDuplicateFiles(sorted, driveDuplicateRemoteOldest)
			for i, e := range sorted {
				target := rel
				sourceID := ""
				if i > 0 {
					var err error
					target, err = relPathWithTokenSuffix(rel, e.FileToken, occupied)
					if err != nil {
						return nil, err
					}
					sourceID = stableTokenIdentifier(e.FileToken)
				}
				view.Files[target] = driveMirrorFile{FileToken: e.FileToken, ModifiedTime: e.ModifiedTime, SourceID: sourceID}
				view.Paths[target] = struct{}{}
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, clierr.Usagef("远端存在重复相对路径（%d 处对应多个云盘条目），为防止静默覆盖已中止:\n  %s\n处理方式：删除多余的远端文件，或对纯文件重名使用 --on-duplicate-remote newest|oldest（pull 还支持 rename：副本加哈希后缀另存）；folder/在线文档与文件重名无法自动处理",
			len(conflicts), strings.Join(conflicts, "\n  "))
	}
	return view, nil
}

func describeDuplicate(rel string, group []client.DriveRemoteEntry) string {
	parts := make([]string, 0, len(group))
	for _, e := range group {
		parts = append(parts, fmt.Sprintf("%s[%s]", e.FileToken, e.Type))
	}
	return fmt.Sprintf("%s: %s", rel, strings.Join(parts, ", "))
}

func chooseDuplicateFile(group []client.DriveRemoteEntry, policy string) client.DriveRemoteEntry {
	sorted := append([]client.DriveRemoteEntry(nil), group...)
	sortDuplicateFiles(sorted, policy)
	return sorted[0]
}

// sortDuplicateFiles：newest 按 modified_time 降序（其次 created_time），oldest 按 created_time 升序（其次 modified_time）；
// 时间不可解析或相同时按 token 排序，保证结果稳定。
func sortDuplicateFiles(files []client.DriveRemoteEntry, policy string) {
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if policy == driveDuplicateRemoteNewest {
			if c, ok := compareDriveTimes(a.ModifiedTime, b.ModifiedTime); ok && c != 0 {
				return c > 0
			}
			if c, ok := compareDriveTimes(a.CreatedTime, b.CreatedTime); ok && c != 0 {
				return c > 0
			}
		} else {
			if c, ok := compareDriveTimes(a.CreatedTime, b.CreatedTime); ok && c != 0 {
				return c < 0
			}
			if c, ok := compareDriveTimes(a.ModifiedTime, b.ModifiedTime); ok && c != 0 {
				return c < 0
			}
		}
		return a.FileToken < b.FileToken
	})
}

func stableTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func stableTokenIdentifier(token string) string {
	return "hash_" + stableTokenHash(token)[:12]
}

func relPathWithSuffix(rel, suffix string) string {
	dir, base := path.Split(rel)
	ext := path.Ext(base)
	if ext == base {
		return dir + base + suffix
	}
	return dir + strings.TrimSuffix(base, ext) + suffix + ext
}

// relPathWithTokenSuffix 为重名副本生成稳定且不冲突的 rel_path：name__lark_<token 哈希>.ext。
func relPathWithTokenSuffix(rel, token string, occupied map[string]struct{}) (string, error) {
	h := stableTokenHash(token)
	for _, n := range []int{12, 24, len(h)} {
		candidate := relPathWithSuffix(rel, "__lark_"+h[:n])
		if _, exists := occupied[candidate]; !exists {
			occupied[candidate] = struct{}{}
			return candidate, nil
		}
	}
	for i := 2; i <= 1024; i++ {
		candidate := relPathWithSuffix(rel, "__lark_"+h+"_"+strconv.Itoa(i))
		if _, exists := occupied[candidate]; !exists {
			occupied[candidate] = struct{}{}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("无法为重名文件 %q 生成唯一路径", rel)
}

// parseDriveEpoch 解析云盘 epoch 字符串（按量级识别秒/毫秒/微秒），返回时间与分辨率。
func parseDriveEpoch(raw string) (time.Time, time.Duration, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	switch {
	case v > 1e14 || v < -1e14:
		return time.UnixMicro(v), time.Microsecond, true
	case v > 1e11 || v < -1e11:
		return time.UnixMilli(v), time.Millisecond, true
	default:
		return time.Unix(v, 0), time.Second, true
	}
}

func compareDriveTimes(a, b string) (int, bool) {
	at, _, aok := parseDriveEpoch(a)
	bt, _, bok := parseDriveEpoch(b)
	if !aok || !bok {
		return 0, false
	}
	switch {
	case at.Before(bt):
		return -1, true
	case at.After(bt):
		return 1, true
	}
	return 0, true
}

// compareRemoteModifiedToLocal 按远端时间分辨率比较远端 modified_time 与本地 mtime：
// -1 远端更旧、0 相同、1 远端更新；远端时间不可解析时 ok=false。
func compareRemoteModifiedToLocal(remote string, local time.Time) (int, bool) {
	rt, res, ok := parseDriveEpoch(remote)
	if !ok {
		return 0, false
	}
	lt := local.Truncate(res)
	switch {
	case rt.Before(lt):
		return -1, true
	case rt.After(lt):
		return 1, true
	}
	return 0, true
}

// applyRemoteModifiedTime 下载后把本地文件 mtime 对齐为远端 modified_time（尽力而为）。
var applyRemoteModifiedTime = func(target, remote string) error {
	rt, _, ok := parseDriveEpoch(remote)
	if !ok {
		return nil
	}
	return os.Chtimes(target, rt, rt)
}
