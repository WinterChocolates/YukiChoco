package main

import (
	"sort"
	"strings"
)

const jsonxImport = "github.com/WinterChocolates/YukiChoco/internal/jsonx"

// emitAll 生成 types 包的全部文件。
func (g *gen) emitAll(dir string) error {
	steps := []struct {
		name    string
		imports []string
		body    func(*writer)
	}{
		{"enum_gen.go", nil, g.emitEnums},
		{"entity_gen.go", nil, g.emitEntities},
		{"message_gen.go", []string{"encoding/json", "fmt", jsonxImport}, g.emitMessages},
		{"event_gen.go", []string{"encoding/json", "fmt", jsonxImport}, g.emitEvents},
		{"api_model_gen.go", nil, g.emitAPIModels},
		{"endpoint_gen.go", nil, g.emitEndpoints},
	}
	for _, s := range steps {
		if err := g.file(dir, s.name, "types", s.imports, s.body); err != nil {
			return err
		}
	}
	return nil
}

// ───────────────────────────── 枚举 ─────────────────────────────

func (g *gen) emitEnums(w *writer) {
	type enumEntry struct {
		spec enumSpec
	}
	entries := make([]enumEntry, 0, len(enumByField))
	for _, spec := range enumByField {
		entries = append(entries, enumEntry{spec: spec})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].spec.TypeName < entries[j].spec.TypeName })
	for _, e := range entries {
		g.emitEnumType(w, e.spec.TypeName, e.spec.Desc, e.spec.TypeName, e.spec.Values)
	}
}

// ───────────────────────────── 实体 ─────────────────────────────

func (g *gen) emitEntities(w *writer) {
	for i := range g.ir.CommonStructs {
		s := &g.ir.CommonStructs[i]
		if s.StructType != "simple" {
			continue
		}
		specs, err := g.fieldSpecs(s.Fields)
		if err != nil {
			panic(err) // validate 已保证不会发生
		}
		g.emitStruct(w, s.Name, s.Description, specs)
	}
}

// ───────────────────────────── API 模型 ─────────────────────────────

func (g *gen) emitAPIModels(w *writer) {
	for _, api := range g.ir.APIList() {
		base := pascal(api.Endpoint)
		doc := cleanText(api.Description)
		if len(api.RequestFields) > 0 {
			specs, err := g.fieldSpecs(api.RequestFields)
			if err != nil {
				panic(err)
			}
			g.emitStruct(w, base+"Request", doc+"的请求参数", specs)
		}
		specs, err := g.fieldSpecs(api.ResponseFields)
		if err != nil {
			panic(err)
		}
		g.emitStruct(w, base+"Response", doc+"的响应数据", specs)
	}
}

// ───────────────────────────── 端点与版本 ─────────────────────────────

func (g *gen) emitEndpoints(w *writer) {
	apis := g.ir.APIList()

	w.p("// ProtocolVersion 是本 SDK 对齐的 Milky 协议主版本。")
	w.p("const ProtocolVersion = %q", g.ir.MilkyVersion)
	w.p("")
	w.p("// ProtocolPackageVersion 是生成本包时使用的协议定义包版本。")
	w.p("const ProtocolPackageVersion = %q", g.ir.MilkyPackageVersion)
	w.p("")

	w.p("// 端点名称，用于 Client.Call 等泛化调用。")
	w.p("const (")
	for _, api := range apis {
		w.p("\t// Endpoint%s 对应 %s（%s）。", pascal(api.Endpoint), api.Endpoint, cleanText(api.Description))
		w.p("\tEndpoint%s = %q", pascal(api.Endpoint), api.Endpoint)
	}
	w.p(")")
	w.p("")

	sorted := make([]API, len(apis))
	copy(sorted, apis)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Endpoint < sorted[j].Endpoint })

	w.p("// AllEndpoints 是 Milky %s 的全部端点名（按字典序）。", g.ir.MilkyVersion)
	w.p("var AllEndpoints = []string{")
	for _, api := range sorted {
		w.p("\t%q,", api.Endpoint)
	}
	w.p("}")
	w.p("")

	w.p("// APISince 记录引入版本高于 1.0 的端点，用于能力探测与降级。")
	w.p("var APISince = map[string]string{")
	for _, api := range sorted {
		if api.Since != "" {
			w.p("\t%q: %q,", api.Endpoint, api.Since)
		}
	}
	w.p("}")
	w.p("")

	// 端点 → service 方法的映射表，供上层与测试使用。
	w.p("// EndpointService 记录端点归属的 SDK service 与方法名。")
	w.p("var EndpointService = map[string]struct{ Service, Method string }{")
	for _, api := range sorted {
		spec := endpoints[api.Endpoint]
		w.p("\t%q: {%q, %q},", api.Endpoint, spec.Service, spec.Method)
	}
	w.p("}")
}

// helper：联合类型分支的判别取值。
func tagValues(variants []Variant) []string {
	out := make([]string, 0, len(variants))
	for _, v := range variants {
		out = append(out, v.TagValue)
	}
	return out
}

// helper：把 []string 去重并保持顺序。
func dedupStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// helper：生成常量名列表，用于 switch 的 case 聚合。
func constNames(prefix string, values []string) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, prefix+pascal(v))
	}
	return strings.Join(out, ", ")
}
