package auth

// PartitionScopes 一次遍历 granted，把 required 分成"已授权"和"缺失"两部分。
// 两个返回值都保持 required 原顺序。granted 是空格（或逗号）分隔的 scope 字符串。
func PartitionScopes(granted string, required []string) (matched, missing []string) {
	set := make(map[string]struct{}, 32)
	for _, s := range SplitScopes(granted) {
		set[s] = struct{}{}
	}
	for _, s := range required {
		if _, ok := set[s]; ok {
			matched = append(matched, s)
		} else {
			missing = append(missing, s)
		}
	}
	return
}
