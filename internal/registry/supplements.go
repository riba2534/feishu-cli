package registry

import (
	"encoding/json"
	"strings"
)

// localSupplement 是 local_supplements.json 里的一条白名单：官方公开 meta 端点缺失、
// 但本项目专用命令在用的方法。定义本身取自内嵌 meta_data.json。
type localSupplement struct {
	Service   string   `json:"service"`
	Resources []string `json:"resources"`
	Method    string   `json:"method"`
	Reason    string   `json:"reason"`
}

// ID 返回 schema 路径形式的方法标识（service.resource[.sub].method）。
func (s localSupplement) ID() string {
	parts := append([]string{s.Service}, s.Resources...)
	return strings.Join(append(parts, s.Method), ".")
}

// appliedSupplements 记录本进程在 overlay 之后从内嵌基线补回的方法 ID。
var appliedSupplements []string

func loadLocalSupplements() []localSupplement {
	data, err := registryFS.ReadFile("local_supplements.json")
	if err != nil {
		return nil
	}
	var wrapper struct {
		Methods []localSupplement `json:"methods"`
	}
	if json.Unmarshal(data, &wrapper) != nil {
		return nil
	}
	return wrapper.Methods
}

// lookupMethod 按 resource 链精确查找方法定义（不做点号拆分，避免与带点的 resource 名混淆）。
func lookupMethod(svc map[string]interface{}, resources []string, method string) map[string]interface{} {
	if svc == nil || len(resources) == 0 {
		return nil
	}
	cur := svc
	for _, name := range resources {
		children, _ := cur["resources"].(map[string]interface{})
		next, _ := children[name].(map[string]interface{})
		if next == nil {
			return nil
		}
		cur = next
	}
	methods, _ := cur["methods"].(map[string]interface{})
	m, _ := methods[method].(map[string]interface{})
	return m
}

// ensureResource 在 svc 下按链取出 resource，缺失的层级新建空 map。
func ensureResource(svc map[string]interface{}, resources []string) map[string]interface{} {
	cur := svc
	for _, name := range resources {
		children, _ := cur["resources"].(map[string]interface{})
		if children == nil {
			children = make(map[string]interface{})
			cur["resources"] = children
		}
		next, _ := children[name].(map[string]interface{})
		if next == nil {
			next = make(map[string]interface{})
			children[name] = next
		}
		cur = next
	}
	return cur
}

// applyLocalSupplements 在 remote overlay 之后执行：overlay 按 service 整体替换，
// 会抹掉内嵌独有的方法；这里只把白名单内、且当前合并结果缺失的方法从内嵌基线补回。
// 远端已提供同名方法时以远端为准；内嵌也没有的条目跳过（由 sync 脚本与测试保证不会发生）。
// 刻意不做「内嵌独有全部保留」的合并，以免把远端已下线的陈旧方法带回联网模式。
func applyLocalSupplements() {
	appliedSupplements = nil
	for _, sup := range loadLocalSupplements() {
		def := lookupMethod(embeddedServices[sup.Service], sup.Resources, sup.Method)
		if def == nil {
			continue
		}
		svc := mergedServices[sup.Service]
		if svc == nil || lookupMethod(svc, sup.Resources, sup.Method) != nil {
			continue
		}
		res := ensureResource(svc, sup.Resources)
		methods, _ := res["methods"].(map[string]interface{})
		if methods == nil {
			methods = make(map[string]interface{})
			res["methods"] = methods
		}
		methods[sup.Method] = def
		appliedSupplements = append(appliedSupplements, sup.ID())
	}
}
