package event

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
)

// SubscriptionCall 描述一次服务端订阅注册请求（dry-run 预览与真实注册共用）。
type SubscriptionCall struct {
	Key             string `json:"event_key"`
	Method          string `json:"method"`
	Path            string `json:"path"`
	Body            any    `json:"body,omitempty"`
	UnsubscribePath string `json:"unsubscribe_path,omitempty"` // 非空表示进程退出时（最后一个 consumer）注销
	Label           string `json:"-"`
}

// ParseParams 解析 --param k=v（可重复），按 defs 声明校验：未知参数、枚举值、必填，
// 并补齐默认值。Multi 参数允许重复传入或逗号分隔，结果规整为逗号连接。
// 返回的错误均为用法错误（调用方包装为 exit 2）。
func ParseParams(defs []KeyDefinition, raw []string) (map[string]string, error) {
	declared := map[string]ParamDef{}
	var names []string
	for _, def := range defs {
		for _, p := range def.Params {
			if _, ok := declared[p.Name]; !ok {
				names = append(names, p.Name)
			}
			declared[p.Name] = p
		}
	}
	params := map[string]string{}
	for _, item := range raw {
		k, v, ok := strings.Cut(item, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if !ok || k == "" {
			return nil, fmt.Errorf("--param 格式应为 key=value，得到 %q", item)
		}
		p, known := declared[k]
		if !known {
			if len(names) == 0 {
				return nil, fmt.Errorf("--param %s: 所选 EventKey 不接受任何参数", k)
			}
			return nil, fmt.Errorf("--param %s: 未知参数（可用: %s）", k, strings.Join(names, ", "))
		}
		if v == "" {
			return nil, fmt.Errorf("--param %s 的值不能为空", k)
		}
		if prev, dup := params[k]; dup {
			if !p.Multi {
				return nil, fmt.Errorf("--param %s 只能指定一次", k)
			}
			v = prev + "," + v
		}
		params[k] = v
	}
	for name, v := range params {
		p := declared[name]
		if len(p.Values) == 0 {
			continue
		}
		values := []string{v}
		if p.Multi {
			values = splitMulti(v)
		}
		for _, one := range values {
			if !slices.Contains(p.Values, one) {
				return nil, fmt.Errorf("--param %s 取值 %q 无效（可选: %s）", name, one, strings.Join(p.Values, ", "))
			}
		}
		if p.Multi {
			params[name] = strings.Join(dedupInOrder(values, p.Values), ",")
		}
	}
	for _, def := range defs {
		for _, p := range def.Params {
			if _, ok := params[p.Name]; ok {
				continue
			}
			if p.Default != "" {
				params[p.Name] = p.Default
				continue
			}
			if p.Required {
				return nil, fmt.Errorf("EventKey %s 需要 --param %s=<值>（%s）", def.Key, p.Name, p.Description)
			}
		}
	}
	return params, nil
}

// SubscriptionPlan 计算某个 EventKey 在给定参数下的订阅请求；不需要订阅时返回 nil。
func SubscriptionPlan(def KeyDefinition, params map[string]string) ([]SubscriptionCall, error) {
	if def.SubscribePath == "" && def.UnsubscribePath == "" {
		return nil, nil
	}
	subPath, err := expandPathParams(def.SubscribePath, params)
	if err != nil {
		return nil, fmt.Errorf("EventKey %s: %w", def.Key, err)
	}
	unsubPath := ""
	if def.UnsubscribePath != "" {
		if unsubPath, err = expandPathParams(def.UnsubscribePath, params); err != nil {
			return nil, fmt.Errorf("EventKey %s: %w", def.Key, err)
		}
	}
	newCall := func(body any, label string) SubscriptionCall {
		return SubscriptionCall{Key: def.Key, Method: "POST", Path: subPath, Body: body, UnsubscribePath: unsubPath, Label: label}
	}
	switch {
	case def.SubscribeBody != nil:
		return []SubscriptionCall{newCall(def.SubscribeBody, bodyLabel(def.SubscribeBody))}, nil
	case def.SubscribeNoBody:
		return []SubscriptionCall{newCall(nil, "")}, nil
	}
	bodies := subscriptionRequestBodies(def)
	if selected := params["subscription_type"]; selected != "" && len(def.SubscribeTypes) > 0 && !def.SubscribeEventType {
		want := splitMulti(selected)
		filtered := bodies[:0:0]
		for _, b := range bodies {
			if slices.Contains(want, b["subscription_type"]) {
				filtered = append(filtered, b)
			}
		}
		bodies = filtered
	}
	out := make([]SubscriptionCall, 0, len(bodies))
	for _, b := range bodies {
		out = append(out, newCall(b, subscriptionBodyLabel(b)))
	}
	return out, nil
}

// ResolveAutoIdentity 为 --as auto 选择订阅身份：存在需要订阅的 key 时，
// 已配置 User 且所有订阅 key 允许 user → user；否则所有订阅 key 允许 bot → bot；
// 否则仍为 user（交给运行期给出「需要 auth login」的明确报错，与旧版一致）。
// 没有任何订阅 key 时返回空字符串（身份无关）。
func ResolveAutoIdentity(defs []KeyDefinition, hasUser bool) string {
	allowUser, allowBot, needed := true, true, false
	for _, def := range defs {
		if !def.HasSubscription() {
			continue
		}
		needed = true
		allowUser = allowUser && def.AllowsIdentity("user")
		allowBot = allowBot && def.AllowsIdentity("bot")
	}
	switch {
	case !needed:
		return ""
	case hasUser && allowUser:
		return "user"
	case allowBot:
		return "bot"
	default:
		return "user"
	}
}

// CheckIdentity 校验显式 --as user|bot 与每个 EventKey 声明的身份一致。
func CheckIdentity(defs []KeyDefinition, identity string) error {
	for _, def := range defs {
		if !def.AllowsIdentity(identity) {
			return fmt.Errorf("EventKey %s 不支持 --as %s（支持: %s）", def.Key, identity, strings.Join(def.IdentityTypes(), ", "))
		}
	}
	return nil
}

// AnySupportsBot 判断订阅类 key 中是否存在可用 bot 身份的（决定 auto 是否 fail-closed）。
func AnySupportsBot(defs []KeyDefinition) bool {
	for _, def := range defs {
		if def.HasSubscription() && def.AllowsIdentity("bot") {
			return true
		}
	}
	return false
}

// matchWhiteboardID 只放行 event.whiteboard_id 等于 --param whiteboard_id 的事件。
func matchWhiteboardID(payload []byte, params map[string]string) bool {
	want := params["whiteboard_id"]
	if want == "" {
		return false
	}
	var envelope struct {
		Event struct {
			WhiteboardID string `json:"whiteboard_id"`
		} `json:"event"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return false
	}
	return envelope.Event.WhiteboardID == want
}

// expandPathParams 把路径中的 {name} 替换为路径段转义后的参数值。
func expandPathParams(path string, params map[string]string) (string, error) {
	for {
		start := strings.Index(path, "{")
		if start < 0 {
			return path, nil
		}
		end := strings.Index(path[start:], "}")
		if end < 0 {
			return path, nil
		}
		name := path[start+1 : start+end]
		v := params[name]
		if v == "" {
			return "", fmt.Errorf("缺少 --param %s=<值>", name)
		}
		path = path[:start] + url.PathEscape(v) + path[start+end+1:]
	}
}

func bodyLabel(body map[string]any) string {
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, body[k]))
	}
	return strings.Join(parts, " ")
}

func splitMulti(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// dedupInOrder 去重并按 order 中的声明顺序排序（与官方一致：结果顺序不取决于传入顺序）。
func dedupInOrder(values, order []string) []string {
	var out []string
	for _, o := range order {
		if slices.Contains(values, o) {
			out = append(out, o)
		}
	}
	return out
}
