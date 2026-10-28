package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// IR 对应 Milky IR（@saltify/milky-protocol 的 dist/protocol.json）。
type IR struct {
	MilkyVersion        string        `json:"milkyVersion"`
	MilkyPackageVersion string        `json:"milkyPackageVersion"`
	CommonStructs       []Struct      `json:"commonStructs"`
	APICategories       []APICategory `json:"apiCategories"`
}

// Struct 是 IR 中的公共结构体，structType 为 simple 或 union。
type Struct struct {
	StructType     string    `json:"structType"`
	UnionType      string    `json:"unionType"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	TagFieldName   string    `json:"tagFieldName"`
	Since          string    `json:"since"`
	Fields         []Field   `json:"fields"`
	BaseFields     []Field   `json:"baseFields"`
	DerivedTypes   []Variant `json:"derivedTypes"`
	DerivedStructs []Variant `json:"derivedStructs"`
}

// Variants 返回该联合类型的所有分支，屏蔽 IR 中 derivedTypes / derivedStructs 的差异。
func (s *Struct) Variants() []Variant {
	if len(s.DerivedTypes) > 0 {
		return s.DerivedTypes
	}
	return s.DerivedStructs
}

// Variant 是联合类型的一个分支。
type Variant struct {
	TagValue      string  `json:"tagValue"`
	Description   string  `json:"description"`
	DerivingType  string  `json:"derivingType"` // struct | ref
	RefStructName string  `json:"refStructName"`
	Since         string  `json:"since"`
	Fields        []Field `json:"fields"`
}

// Field 是结构体字段或 API 参数。
type Field struct {
	FieldType     string          `json:"fieldType"` // scalar | ref | enum
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	IsArray       bool            `json:"isArray"`
	IsOptional    bool            `json:"isOptional"`
	ScalarType    string          `json:"scalarType"` // int32 | int64 | bool | string
	DataType      string          `json:"dataType"`   // uin
	RefStructName string          `json:"refStructName"`
	Values        []string        `json:"values"`
	DefaultValue  json.RawMessage `json:"defaultValue"`
	Since         string          `json:"since"`
}

// HasDefault 报告该字段在协议中是否带默认值。
func (f *Field) HasDefault() bool { return len(f.DefaultValue) > 0 }

// APICategory 是 API 分类（系统 / 消息 / 好友 / 群聊 / 文件）。
type APICategory struct {
	Name string `json:"name"`
	APIs []API  `json:"apis"`
}

// API 是一个协议端点。
type API struct {
	Endpoint       string  `json:"endpoint"`
	Description    string  `json:"description"`
	Since          string  `json:"since"`
	RequestFields  []Field `json:"requestFields"`
	ResponseFields []Field `json:"responseFields"`
}

// Load 读取并校验 IR 文件。
func Load(path string) (*IR, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 IR 失败: %w", err)
	}
	var ir IR
	if err := json.Unmarshal(raw, &ir); err != nil {
		return nil, fmt.Errorf("解析 IR 失败: %w", err)
	}
	if ir.MilkyVersion == "" || ir.MilkyPackageVersion == "" {
		return nil, fmt.Errorf("IR 缺少 milkyVersion / milkyPackageVersion")
	}
	if len(ir.CommonStructs) == 0 || len(ir.APICategories) == 0 {
		return nil, fmt.Errorf("IR 内容为空")
	}
	return &ir, nil
}

// APIList 按 IR 顺序展开所有 API。
func (ir *IR) APIList() []API {
	var out []API
	for _, c := range ir.APICategories {
		out = append(out, c.APIs...)
	}
	return out
}

// FindStruct 按名字查找公共结构体。
func (ir *IR) FindStruct(name string) (*Struct, bool) {
	for i := range ir.CommonStructs {
		if ir.CommonStructs[i].Name == name {
			return &ir.CommonStructs[i], true
		}
	}
	return nil, false
}
