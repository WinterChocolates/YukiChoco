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
		{"api_model_gen.go", g.apiModelImports(), g.emitAPIModels},
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

// apiModelImports 判断 api_model_gen.go 是否需要用 encoding/json 与 fmt
// （仅当有响应结构体包含联合类型字段时）。
func (g *gen) apiModelImports() []string {
	for _, api := range g.ir.APIList() {
		if len(g.unionFields(api.ResponseFields)) > 0 {
			return []string{"encoding/json", "fmt", jsonxImport}
		}
	}
	return nil
}

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

		// 响应里含联合类型（接口）字段时，需要自定义解码。
		if uf := g.unionFields(api.ResponseFields); len(uf) > 0 {
			g.emitUnionDecode(w, base+"Response", specs, uf)
		}
	}
}

// emitUnionDecode 为含联合类型字段的结构体生成 UnmarshalJSON。
//
// 联合类型（IncomingMessage / GroupNotification）是接口，encoding/json 无法直接
// 填充。做法：用一个「影子结构体」把联合字段降级成 json.RawMessage 解码其余字段，
// 再逐一按判别字段解码联合字段（数组中的非法元素跳过，契约 C3）。
func (g *gen) emitUnionDecode(w *writer, typeName string, specs []fieldSpec, unionFields []Field) {
	unionByName := make(map[string]Field, len(unionFields))
	for _, f := range unionFields {
		unionByName[f.Name] = f
	}

	w.p("// UnmarshalJSON 解码联合类型字段（联合是接口，encoding/json 不能直接填充）。")
	w.p("func (v *%s) UnmarshalJSON(b []byte) error {", typeName)
	w.p("\ttype shadow struct {")
	for _, sp := range specs {
		name := jsonTagName(sp.tag)
		if uf, ok := unionByName[name]; ok {
			if uf.IsArray {
				w.p("\t\t%s []json.RawMessage `json:\"%s\"`", sp.name, name)
				continue
			}
			w.p("\t\t%s json.RawMessage `json:\"%s\"`", sp.name, name)
			continue
		}
		w.p("\t\t%s %s `json:\"%s\"`", sp.name, sp.typ, sp.tag)
	}
	w.p("\t}")
	w.p("\tvar s shadow")
	w.p("\tif err := json.Unmarshal(b, &s); err != nil {")
	w.p("\t\treturn err")
	w.p("\t}")

	// 普通字段直接搬运。
	for _, sp := range specs {
		if _, ok := unionByName[jsonTagName(sp.tag)]; ok {
			continue
		}
		w.p("\tv.%s = s.%s", sp.name, sp.name)
	}

	// 联合字段逐个解码。
	for _, f := range unionFields {
		name := pascal(f.Name)
		if f.IsArray {
			w.p("\tif len(s.%s) > 0 {", name)
			w.p("\t\tv.%s = make([]%s, 0, len(s.%s))", name, f.RefStructName, name)
			w.p("\t\tfor i, item := range s.%s {", name)
			w.p("\t\t\tif len(item) == 0 || string(item) == \"null\" {")
			w.p("\t\t\t\tcontinue")
			w.p("\t\t\t}")
			w.p("\t\t\tdecoded, err := Unmarshal%s(item)", f.RefStructName)
			w.p("\t\t\tif err != nil {")
			w.p("\t\t\t\tjsonx.Report(err, fmt.Sprintf(\"%s.%s[%%d]\", i))", typeName, name)
			w.p("\t\t\t\tcontinue")
			w.p("\t\t\t}")
			w.p("\t\t\tv.%s = append(v.%s, decoded)", name, name)
			w.p("\t\t}")
			w.p("\t}")
			continue
		}
		w.p("\tif len(s.%s) > 0 && string(s.%s) != \"null\" {", name, name)
		w.p("\t\tdecoded, err := Unmarshal%s(s.%s)", f.RefStructName, name)
		w.p("\t\tif err != nil {")
		w.p("\t\t\treturn fmt.Errorf(\"types: 解码 %s.%s 失败: %%w\", err)", typeName, name)
		w.p("\t\t}")
		w.p("\t\tv.%s = decoded", name)
		w.p("\t}")
	}
	w.p("\treturn nil")
	w.p("}")
	w.p("")
}

// jsonTagName 取出结构体标签里的 JSON 字段名。
func jsonTagName(tag string) string {
	if i := strings.IndexByte(tag, ','); i >= 0 {
		return tag[:i]
	}
	return tag
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
