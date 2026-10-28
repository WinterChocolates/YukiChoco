package main

import (
	"fmt"
	"strings"
)

// messageOverrides 让发送类端点生成更顺手的签名：
//
//	SendMessage(ctx, groupID types.GroupID, segments ...types.OutgoingSegment)
//
// 而不是要求调用方先构造请求结构体。
var messageOverrides = map[string]struct {
	IDField  string // 请求中的目标 ID 字段
	SegField string // 请求中的消息段字段
}{
	"send_group_message":   {IDField: "group_id", SegField: "message"},
	"send_private_message": {IDField: "user_id", SegField: "message"},
}

// serviceDesc 用于生成 service 的文档注释。
var serviceDesc = map[string]string{
	"System":  "系统",
	"Message": "消息",
	"Friend":  "好友",
	"Group":   "群聊",
	"File":    "文件",
}

// emitServices 生成 5 个 service 的接口与实现（package 名为根包）。
func (g *gen) emitServices(dir, pkgName, module string) error {
	typesImport := module + "/types"
	for _, svc := range serviceOrder {
		apis := g.apisOfService(svc)
		if len(apis) == 0 {
			return fmt.Errorf("service %s 没有任何端点", svc)
		}
		name := "service_" + strings.ToLower(svc) + "_gen.go"
		if err := g.file(dir, name, pkgName, []string{"context", typesImport}, func(w *writer) {
			if err := g.emitServiceBody(w, svc, apis); err != nil {
				panic(err)
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// apisOfService 按 IR 顺序取出属于某 service 的端点。
func (g *gen) apisOfService(service string) []API {
	var out []API
	for _, api := range g.ir.APIList() {
		if endpoints[api.Endpoint].Service == service {
			out = append(out, api)
		}
	}
	return out
}

// qualify 把 types 包内的类型名加上包前缀（生成到根包时需要）。
func qualify(typ string) string {
	switch {
	case typ == "int32", typ == "int64", typ == "bool", typ == "string":
		return typ
	case strings.HasPrefix(typ, "[]"):
		return "[]" + qualify(strings.TrimPrefix(typ, "[]"))
	case strings.HasPrefix(typ, "*"):
		return "*" + qualify(strings.TrimPrefix(typ, "*"))
	default:
		return "types." + typ
	}
}

// serviceParams 生成方法参数列表（不含 ctx）。
func (g *gen) serviceParams(api API) (string, error) {
	base := pascal(api.Endpoint)
	if ov, ok := messageOverrides[api.Endpoint]; ok {
		idField, ok := findFieldByJSON(api.RequestFields, ov.IDField)
		if !ok {
			return "", fmt.Errorf("%s: 请求中缺少字段 %q", api.Endpoint, ov.IDField)
		}
		segField, ok := findFieldByJSON(api.RequestFields, ov.SegField)
		if !ok {
			return "", fmt.Errorf("%s: 请求中缺少字段 %q", api.Endpoint, ov.SegField)
		}
		idType, err := g.goType(idField)
		if err != nil {
			return "", err
		}
		segElem, err := g.baseType(segField)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(", %s %s, segments ...%s",
			camel(ov.IDField), qualify(idType), qualify(segElem)), nil
	}
	if len(api.RequestFields) == 0 {
		return "", nil
	}
	return ", req *types." + base + "Request", nil
}

// emitServiceBody 输出一个 service 的接口与实现。
func (g *gen) emitServiceBody(w *writer, svc string, apis []API) error {
	impl := camel(svc) + "Service"

	w.p("// %sService 是%s相关的 Milky API 集合。", svc, serviceDesc[svc])
	w.p("//")
	w.p("// 方法全部接受 context；返回值是协议端信封 data 中的内容。")
	w.p("type %sService interface {", svc)
	for _, api := range apis {
		spec := endpoints[api.Endpoint]
		params, err := g.serviceParams(api)
		if err != nil {
			return err
		}
		w.p("\t// %s %s。", spec.Method, cleanText(api.Description))
		if api.Since != "" {
			w.p("\t// 自 Milky %s 起提供。", api.Since)
		}
		w.p("\t%s(ctx context.Context%s) (*types.%sResponse, error)", spec.Method, params, pascal(api.Endpoint))
	}
	w.p("}")
	w.p("")

	w.p("type %s struct{ c *Client }", impl)
	w.p("")
	w.p("func new%sService(c *Client) %sService { return &%s{c: c} }", svc, svc, impl)
	w.p("")

	for _, api := range apis {
		spec := endpoints[api.Endpoint]
		base := pascal(api.Endpoint)
		params, err := g.serviceParams(api)
		if err != nil {
			return err
		}
		w.p("// %s %s。", spec.Method, cleanText(api.Description))
		if api.Since != "" {
			w.p("// 自 Milky %s 起提供。", api.Since)
		}
		w.p("func (s *%s) %s(ctx context.Context%s) (*types.%sResponse, error) {", impl, spec.Method, params, base)
		switch {
		case messageOverrides[api.Endpoint].IDField != "":
			ov := messageOverrides[api.Endpoint]
			w.p("\treturn callEndpoint[types.%sRequest, types.%sResponse](ctx, s.c, types.Endpoint%s,", base, base, base)
			w.p("\t\t&types.%sRequest{%s: %s, %s: segments})",
				base, pascal(ov.IDField), camel(ov.IDField), pascal(ov.SegField))
		case len(api.RequestFields) == 0:
			w.p("\treturn callEndpoint[struct{}, types.%sResponse](ctx, s.c, types.Endpoint%s, nil)", base, base)
		default:
			w.p("\treturn callEndpoint[types.%sRequest, types.%sResponse](ctx, s.c, types.Endpoint%s, req)", base, base, base)
		}
		w.p("}")
		w.p("")
	}
	return nil
}
