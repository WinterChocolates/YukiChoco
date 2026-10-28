package main

import (
	"fmt"
)

// accessor 描述 plain 联合类型在接口上暴露的访问器。
type accessor struct {
	Method string
	Field  string
	Doc    string
}

var plainUnionAccessors = map[string][]accessor{
	"IncomingMessage": {
		{Method: "Scene", Field: "message_scene", Doc: "返回消息场景。"},
		{Method: "Peer", Field: "peer_id", Doc: "返回会话 ID（好友 QQ 号或群号）。"},
		{Method: "Seq", Field: "message_seq", Doc: "返回消息序列号。"},
		{Method: "Sender", Field: "sender_id", Doc: "返回发送者 QQ 号。"},
		{Method: "MessageSegments", Field: "segments", Doc: "返回消息段列表。"},
	},
	"GroupNotification": {
		{Method: "Kind", Field: "type", Doc: "返回通知类型。"},
		{Method: "Group", Field: "group_id", Doc: "返回群号。"},
	},
}

func findFieldByJSON(fields []Field, jsonName string) (Field, bool) {
	for _, f := range fields {
		if f.Name == jsonName {
			return f, true
		}
	}
	return Field{}, false
}

// accessorType 解析访问器的返回类型：判别字段用枚举类型，其余按 IR 推断。
func (g *gen) accessorType(s *Struct, variants []Variant, tagType, jsonName string) (string, error) {
	if jsonName == s.TagFieldName {
		return tagType, nil
	}
	for _, v := range variants {
		if f, ok := findFieldByJSON(v.Fields, jsonName); ok {
			return g.goType(f)
		}
	}
	return "", fmt.Errorf("%s: 访问器字段 %q 在所有分支中都不存在", s.Name, jsonName)
}

// variantSpecs 生成分支结构体字段：plain 联合需要把判别字段注入进去。
func (g *gen) variantSpecs(s *Struct, v Variant, tagType string) ([]fieldSpec, error) {
	specs := []fieldSpec{{
		name: pascal(s.TagFieldName),
		typ:  tagType,
		tag:  s.TagFieldName,
		doc:  []string{fmt.Sprintf("是 %s 的判别字段。", s.Name)},
	}}
	rest, err := g.fieldSpecs(v.Fields)
	if err != nil {
		return nil, err
	}
	return append(specs, rest...), nil
}

// isEnumType 报告 name 是否为本包生成的字符串枚举。
func (g *gen) isEnumType(name string) bool {
	for _, spec := range enumByField {
		if spec.TypeName == name {
			return true
		}
	}
	switch name {
	case "SegmentType", "EventType", "MessageScene", "GroupNotificationType":
		return true
	}
	return false
}

// zeroValue 返回某类型的零值表达式（用于未知分支的占位实现）。
func (g *gen) zeroValue(typ string) string {
	if g.isEnumType(typ) {
		return `""`
	}
	switch {
	case typ == "string":
		return `""`
	case typ == "bool":
		return "false"
	case len(typ) > 2 && typ[:2] == "[]":
		return "nil"
	case typ == "IncomingSegments", typ == "OutgoingSegments":
		return "nil"
	case typ == "IncomingSegment", typ == "OutgoingSegment", typ == "IncomingMessage", typ == "GroupNotification":
		return "nil"
	case typ == "int32", typ == "int64", typ == "UserID", typ == "GroupID", typ == "Uin", typ == "MessageSeq":
		return "0"
	default:
		return typ + "{}"
	}
}

// ───────────────────────────── 消息包 ─────────────────────────────

func (g *gen) emitMessages(w *writer) {
	inSeg, ok1 := g.ir.FindStruct("IncomingSegment")
	outSeg, ok2 := g.ir.FindStruct("OutgoingSegment")
	if !ok1 || !ok2 {
		panic("IR 缺少 IncomingSegment / OutgoingSegment")
	}

	// SegmentType 由两个联合合并去重后只生成一次。
	values := dedupStrings(append(tagValues(inSeg.Variants()), tagValues(outSeg.Variants())...))
	g.emitEnumType(w, "SegmentType", "消息段类型", "Segment", values)

	if err := g.emitDataUnion(w, inSeg); err != nil {
		panic(err)
	}
	if err := g.emitDataUnion(w, outSeg); err != nil {
		panic(err)
	}
	for _, name := range []string{"GroupNotification", "IncomingMessage"} {
		if s, ok := g.ir.FindStruct(name); ok {
			if err := g.emitPlainUnion(w, s); err != nil {
				panic(err)
			}
		}
	}
}

// emitDataUnion 生成 withData 形式的联合（消息段）：JSON 形如 {"type":..,"data":{..}}。
func (g *gen) emitDataUnion(w *writer, s *Struct) error {
	variants := s.Variants()
	typeName, prefix := tagEnum(s)
	marker := camel(s.Name)

	w.p("// %s %s。", s.Name, cleanText(s.Description))
	w.p("type %s interface {", s.Name)
	w.p("\t// SegmentType 返回消息段类型。")
	w.p("\tSegmentType() %s", typeName)
	w.p("\t%s()", marker)
	w.p("}")
	w.p("")

	for _, v := range variants {
		tn := g.variantTypeName(s, v)
		specs, err := g.fieldSpecs(v.Fields)
		if err != nil {
			return err
		}
		g.emitStruct(w, tn, v.Description, specs)
		constName := prefix + pascal(v.TagValue)
		w.p("// SegmentType 返回 %s。", constName)
		w.p("func (s *%s) SegmentType() %s { return %s }", tn, typeName, constName)
		w.p("")
		w.p("func (s *%s) %s() {}", tn, marker)
		w.p("")
		w.p("// MarshalJSON 序列化为 {\"type\": %q, \"data\": {...}}。", v.TagValue)
		w.p("func (s *%s) MarshalJSON() ([]byte, error) {", tn)
		w.p("\t// 借助别名类型避免再次进入 MarshalJSON 造成递归。")
		w.p("\ttype plain %s", tn)
		w.p("\treturn jsonx.MarshalTagged(string(%s), (*plain)(s))", constName)
		w.p("}")
		w.p("")
	}

	unk := "Unknown" + s.Name
	w.p("// %s 是 SDK 尚未识别的 %s，原样保留 JSON。", unk, s.Name)
	w.p("type %s struct {", unk)
	w.p("\t// Type 是协议端给出的消息段类型。")
	w.p("\tType %s", typeName)
	w.p("\t// Raw 是原始消息段 JSON。")
	w.p("\tRaw json.RawMessage")
	w.p("}")
	w.p("")
	w.p("// SegmentType 返回原始类型。")
	w.p("func (s *%s) SegmentType() %s { return s.Type }", unk, typeName)
	w.p("")
	w.p("func (s *%s) %s() {}", unk, marker)
	w.p("")
	w.p("// MarshalJSON 原样回填未知消息段。")
	w.p("func (s *%s) MarshalJSON() ([]byte, error) { return s.Raw, nil }", unk)
	w.p("")

	slice := s.Name + "s"
	w.p("// %s 是 %s 的列表；解码时跳过非法元素而非整体失败。", slice, s.Name)
	w.p("type %s []%s", slice, s.Name)
	w.p("")
	w.p("// UnmarshalJSON 实现容错解码。")
	w.p("func (s *%s) UnmarshalJSON(b []byte) error {", slice)
	w.p("\tvar raws []json.RawMessage")
	w.p("\tif err := json.Unmarshal(b, &raws); err != nil {")
	w.p("\t\treturn fmt.Errorf(\"types: 解析 %s 失败: %%w\", err)", slice)
	w.p("\t}")
	w.p("\tout := make(%s, 0, len(raws))", slice)
	w.p("\tfor i, raw := range raws {")
	w.p("\t\tseg, err := Unmarshal%s(raw)", s.Name)
	w.p("\t\tif err != nil {")
	w.p("\t\t\tjsonx.Report(err, fmt.Sprintf(\"%s[%%d]\", i))", slice)
	w.p("\t\t\tcontinue")
	w.p("\t\t}")
	w.p("\t\tout = append(out, seg)")
	w.p("\t}")
	w.p("\t*s = out")
	w.p("\treturn nil")
	w.p("}")
	w.p("")

	w.p("// Unmarshal%s 按 %s 字段解码单个消息段；未知类型返回 *%s。", s.Name, s.TagFieldName, unk)
	w.p("func Unmarshal%s(b []byte) (%s, error) {", s.Name, s.Name)
	w.p("\ttag, err := jsonx.TaggedPeek(b, %q)", s.TagFieldName)
	w.p("\tif err != nil {")
	w.p("\t\treturn nil, err")
	w.p("\t}")
	w.p("\tdata, err := jsonx.TaggedData(b)")
	w.p("\tif err != nil {")
	w.p("\t\treturn nil, err")
	w.p("\t}")
	w.p("\tswitch %s(tag) {", typeName)
	for _, v := range variants {
		tn := g.variantTypeName(s, v)
		w.p("\tcase %s:", prefix+pascal(v.TagValue))
		w.p("\t\tvar seg %s", tn)
		w.p("\t\tif err := json.Unmarshal(data, &seg); err != nil {")
		w.p("\t\t\treturn nil, fmt.Errorf(\"types: 解码 %%s 消息段失败: %%w\", tag, err)")
		w.p("\t\t}")
		w.p("\t\treturn &seg, nil")
	}
	w.p("\tdefault:")
	w.p("\t\treturn &%s{Type: %s(tag), Raw: append(json.RawMessage(nil), b...)}, nil", unk, typeName)
	w.p("\t}")
	w.p("}")
	w.p("")
	return nil
}

// emitPlainUnion 生成 plain 形式的联合（判别字段与业务字段在同一层）。
func (g *gen) emitPlainUnion(w *writer, s *Struct) error {
	variants := s.Variants()
	typeName, prefix := tagEnum(s)
	marker := camel(s.Name)
	g.emitEnumType(w, typeName, s.Description, prefix, tagValues(variants))

	accessors := plainUnionAccessors[s.Name]
	if len(accessors) == 0 {
		return fmt.Errorf("%s: 未在 plainUnionAccessors 中定义访问器", s.Name)
	}

	w.p("// %s %s。", s.Name, cleanText(s.Description))
	w.p("type %s interface {", s.Name)
	for _, a := range accessors {
		typ, err := g.accessorType(s, variants, typeName, a.Field)
		if err != nil {
			return err
		}
		w.p("\t// %s %s", a.Method, a.Doc)
		w.p("\t%s() %s", a.Method, typ)
	}
	w.p("\t%s()", marker)
	w.p("}")
	w.p("")

	for _, v := range variants {
		tn := g.variantTypeName(s, v)
		specs, err := g.variantSpecs(s, v, typeName)
		if err != nil {
			return err
		}
		g.emitStruct(w, tn, v.Description, specs)
		for _, a := range accessors {
			typ, err := g.accessorType(s, variants, typeName, a.Field)
			if err != nil {
				return err
			}
			w.p("// %s %s", a.Method, a.Doc)
			w.p("func (m *%s) %s() %s { return m.%s }", tn, a.Method, typ, pascal(a.Field))
			w.p("")
		}
		w.p("func (m *%s) %s() {}", tn, marker)
		w.p("")
	}

	unk := "Unknown" + s.Name
	w.p("// %s 是 SDK 尚未识别的 %s，保留原始 JSON。", unk, s.Name)
	w.p("type %s struct {", unk)
	w.p("\t// Tag 是协议端给出的判别取值。")
	w.p("\tTag %s", typeName)
	w.p("\t// Raw 是原始 JSON。")
	w.p("\tRaw json.RawMessage")
	w.p("}")
	w.p("")
	for _, a := range accessors {
		typ, err := g.accessorType(s, variants, typeName, a.Field)
		if err != nil {
			return err
		}
		w.p("func (m *%s) %s() %s { return %s }", unk, a.Method, typ, g.zeroValue(typ))
	}
	w.p("func (m *%s) %s() {}", unk, marker)
	w.p("")
	w.p("// MarshalJSON 原样回填未知内容。")
	w.p("func (m *%s) MarshalJSON() ([]byte, error) { return m.Raw, nil }", unk)
	w.p("")

	w.p("// Unmarshal%s 按 %s 字段解码 %s。", s.Name, s.TagFieldName, s.Name)
	w.p("func Unmarshal%s(b []byte) (%s, error) {", s.Name, s.Name)
	w.p("\ttag, err := jsonx.TaggedPeek(b, %q)", s.TagFieldName)
	w.p("\tif err != nil {")
	w.p("\t\treturn nil, err")
	w.p("\t}")
	w.p("\tswitch %s(tag) {", typeName)
	for _, v := range variants {
		tn := g.variantTypeName(s, v)
		w.p("\tcase %s:", prefix+pascal(v.TagValue))
		w.p("\t\tvar v %s", tn)
		w.p("\t\tif err := json.Unmarshal(b, &v); err != nil {")
		w.p("\t\t\treturn nil, fmt.Errorf(\"types: 解码 %%s 失败: %%w\", tag, err)")
		w.p("\t\t}")
		w.p("\t\treturn &v, nil")
	}
	w.p("\tdefault:")
	w.p("\t\treturn &%s{Tag: %s(tag), Raw: append(json.RawMessage(nil), b...)}, nil", unk, typeName)
	w.p("\t}")
	w.p("}")
	w.p("")
	return nil
}

// ───────────────────────────── 事件 ─────────────────────────────

func (g *gen) emitEvents(w *writer) {
	s, ok := g.ir.FindStruct("Event")
	if !ok {
		panic("IR 缺少 Event")
	}
	variants := s.Variants()
	typeName, prefix := tagEnum(s)

	g.emitEnumType(w, typeName, "事件类型", prefix, tagValues(variants))

	metaSpecs, err := g.fieldSpecs(s.BaseFields)
	if err != nil {
		panic(err)
	}
	g.emitStruct(w, "EventMeta", "是所有事件共有的元信息", metaSpecs)

	w.p("// Event %s。", cleanText(s.Description))
	w.p("type Event interface {")
	w.p("\t// EventType 返回事件类型。")
	w.p("\tEventType() %s", typeName)
	w.p("\t// Meta 返回事件公共元信息。")
	w.p("\tMeta() EventMeta")
	w.p("\tevent()")
	w.p("}")
	w.p("")

	for _, v := range variants {
		tn := g.variantTypeName(s, v)
		constName := prefix + pascal(v.TagValue)
		dataType := ""
		if v.DerivingType == "ref" {
			if _, ok := g.ir.FindStruct(v.RefStructName); !ok {
				panic(fmt.Sprintf("事件 %s 引用未知结构体 %s", v.TagValue, v.RefStructName))
			}
			dataType = v.RefStructName
		} else {
			dataType = tn + "Data"
			specs, err := g.fieldSpecs(v.Fields)
			if err != nil {
				panic(err)
			}
			g.emitStruct(w, dataType, v.Description+"的数据部分", specs)
		}

		w.p("// %s %s。", tn, cleanText(v.Description))
		if v.Since != "" {
			w.p("// 自 Milky %s 起提供。", v.Since)
		}
		w.p("type %s struct {", tn)
		w.p("\tEventMeta")
		w.p("")
		w.p("\t// Data 是事件数据。")
		w.p("\tData %s `json:\"data\"`", dataType)
		w.p("}")
		w.p("")
		w.p("// EventType 返回 %s。", constName)
		w.p("func (e *%s) EventType() %s { return %s }", tn, typeName, constName)
		w.p("")
		w.p("// Meta 返回事件公共元信息。")
		w.p("func (e *%s) Meta() EventMeta { return e.EventMeta }", tn)
		w.p("")
		w.p("func (e *%s) event() {}", tn)
		w.p("")
		w.p("// MarshalJSON 序列化为协议事件信封。")
		w.p("func (e *%s) MarshalJSON() ([]byte, error) {", tn)
		w.p("\ttype payload struct {")
		w.p("\t\tEventMeta")
		w.p("\t\tEventType %s `json:\"event_type\"`", typeName)
		w.p("\t\tData %s `json:\"data\"`", dataType)
		w.p("\t}")
		w.p("\treturn json.Marshal(payload{e.EventMeta, %s, e.Data})", constName)
		w.p("}")
		w.p("")

		if v.DerivingType == "ref" {
			w.p("// UnmarshalJSON 按判别字段解码事件数据。")
			w.p("func (e *%s) UnmarshalJSON(b []byte) error {", tn)
			w.p("\tvar raw struct {")
			w.p("\t\tEventMeta")
			w.p("\t\tData json.RawMessage `json:\"data\"`")
			w.p("\t}")
			w.p("\tif err := json.Unmarshal(b, &raw); err != nil {")
			w.p("\t\treturn fmt.Errorf(\"types: 解码 %s 事件失败: %%w\", err)", v.TagValue)
			w.p("\t}")
			w.p("\tdata, err := Unmarshal%s(raw.Data)", v.RefStructName)
			w.p("\tif err != nil {")
			w.p("\t\treturn fmt.Errorf(\"types: 解码 %s 事件数据失败: %%w\", err)", v.TagValue)
			w.p("\t}")
			w.p("\te.EventMeta = raw.EventMeta")
			w.p("\te.Data = data")
			w.p("\treturn nil")
			w.p("}")
			w.p("")
		}
	}

	w.p("// UnknownEvent 是 SDK 尚未识别的事件，保留原始 JSON。")
	w.p("type UnknownEvent struct {")
	w.p("\tEventMeta")
	w.p("")
	w.p("\t// Type 是协议端给出的事件类型。")
	w.p("\tType %s", typeName)
	w.p("\t// Raw 是原始事件 JSON。")
	w.p("\tRaw json.RawMessage")
	w.p("}")
	w.p("")
	w.p("// EventType 返回原始事件类型。")
	w.p("func (e *UnknownEvent) EventType() %s { return e.Type }", typeName)
	w.p("")
	w.p("// Meta 返回事件公共元信息。")
	w.p("func (e *UnknownEvent) Meta() EventMeta { return e.EventMeta }")
	w.p("")
	w.p("func (e *UnknownEvent) event() {}")
	w.p("")
	w.p("// MarshalJSON 原样回填未知事件。")
	w.p("func (e *UnknownEvent) MarshalJSON() ([]byte, error) { return e.Raw, nil }")
	w.p("")

	w.p("// UnmarshalEvent 按 event_type 解码事件；未知类型返回 *UnknownEvent。")
	w.p("func UnmarshalEvent(b []byte) (Event, error) {")
	w.p("\ttag, err := jsonx.TaggedPeek(b, %q)", s.TagFieldName)
	w.p("\tif err != nil {")
	w.p("\t\treturn nil, err")
	w.p("\t}")
	w.p("\tvar ev Event")
	w.p("\tswitch %s(tag) {", typeName)
	for _, v := range variants {
		tn := g.variantTypeName(s, v)
		w.p("\tcase %s:", prefix+pascal(v.TagValue))
		w.p("\t\tev = new(%s)", tn)
	}
	w.p("\tdefault:")
	w.p("\t\tvar meta struct{ EventMeta }")
	w.p("\t\tif err := json.Unmarshal(b, &meta); err != nil {")
	w.p("\t\t\treturn nil, fmt.Errorf(\"types: 解码未知事件元信息失败: %%w\", err)")
	w.p("\t\t}")
	w.p("\t\treturn &UnknownEvent{EventMeta: meta.EventMeta, Type: %s(tag), Raw: append(json.RawMessage(nil), b...)}, nil", typeName)
	w.p("\t}")
	w.p("\tif err := json.Unmarshal(b, ev); err != nil {")
	w.p("\t\treturn nil, fmt.Errorf(\"types: 解码 %%q 事件失败: %%w\", tag, err)")
	w.p("\t}")
	w.p("\treturn ev, nil")
	w.p("}")
	w.p("")
}
