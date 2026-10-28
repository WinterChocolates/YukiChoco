package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const specPath = "../../protocol/protocol-1.3.0.json"

// generatedFiles 是 types 包的产物清单，新增生成文件时必须同步此列表。
var generatedFiles = []string{
	"enum_gen.go",
	"entity_gen.go",
	"message_gen.go",
	"event_gen.go",
	"api_model_gen.go",
	"endpoint_gen.go",
}

// generatedServiceFiles 是根包 service 的产物清单。
var generatedServiceFiles = []string{
	"service_system_gen.go",
	"service_message_gen.go",
	"service_friend_gen.go",
	"service_group_gen.go",
	"service_file_gen.go",
}

// TestGolden 保证 types/ 下的生成物与 IR 完全一致：
// 手改生成物、或改了生成器但忘记重新生成，都会在这里失败。
func TestGolden(t *testing.T) {
	ir, err := Load(specPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	g, err := newGen(ir)
	if err != nil {
		t.Fatalf("newGen: %v", err)
	}
	dir := t.TempDir()
	if err := g.emitAll(dir); err != nil {
		t.Fatalf("emitAll: %v", err)
	}
	if err := g.emitServices(dir, defaultPkg, defaultModule); err != nil {
		t.Fatalf("emitServices: %v", err)
	}

	compare := func(name, repoPath string) {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("读取生成物 %s: %v", name, err)
		}
		want, err := os.ReadFile(repoPath)
		if err != nil {
			t.Fatalf("读取仓库中的 %s: %v", repoPath, err)
		}
		// 忽略行尾差异：Windows 上 git 可能以 CRLF 检出（见 .gitattributes），
		// 生成器始终输出 LF，比较前统一归一化。
		if !bytes.Equal(normalizeEOL(got), normalizeEOL(want)) {
			t.Errorf("%s 与 IR 生成结果不一致；请执行 `make generate`", repoPath)
		}
	}
	for _, name := range generatedFiles {
		compare(name, filepath.Join("../../types", name))
	}
	for _, name := range generatedServiceFiles {
		compare(name, filepath.Join("../../", name))
	}
}

// normalizeEOL 把 CRLF 归一化为 LF。
func normalizeEOL(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// TestIRBaseline 锁定协议基线，防止无意间替换了 IR 文件。
func TestIRBaseline(t *testing.T) {
	ir, err := Load(specPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ir.MilkyVersion != "1.3" || ir.MilkyPackageVersion != "1.3.0" {
		t.Fatalf("协议版本 = %s (%s)，期望 1.3 (1.3.0)", ir.MilkyVersion, ir.MilkyPackageVersion)
	}
	apis := ir.APIList()
	if len(apis) != 65 {
		t.Fatalf("端点数量 = %d，期望 65", len(apis))
	}
	if len(ir.CommonStructs) != 16 {
		t.Fatalf("公共结构体数量 = %d，期望 16", len(ir.CommonStructs))
	}
	events, ok := ir.FindStruct("Event")
	if !ok {
		t.Fatal("IR 缺少 Event")
	}
	if got := len(events.Variants()); got != 21 {
		t.Fatalf("事件数量 = %d，期望 21", got)
	}
}

// TestValidateRejectsUnknownEndpoint 确保新增端点必须显式登记命名，而不是被静默忽略。
func TestValidateRejectsUnknownEndpoint(t *testing.T) {
	ir, err := Load(specPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ir.APICategories[0].APIs = append(ir.APICategories[0].APIs, API{
		Endpoint:    "future_endpoint",
		Description: "未来端点",
	})
	_, err = newGen(ir)
	if err == nil {
		t.Fatal("新增未登记端点时应报错")
	}
	if !strings.Contains(err.Error(), "future_endpoint") {
		t.Fatalf("错误信息应指出端点名，实际: %v", err)
	}
}

// TestValidateRejectsEnumDrift 确保枚举值域变化会中断生成。
func TestValidateRejectsEnumDrift(t *testing.T) {
	ir, err := Load(specPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// 篡改 GroupEntity 的一个枚举字段值域是不现实的（无枚举），改用 Event 的 tag 值域：
	// 这里直接篡改 enumByField 副本无法做到（全局表），故构造一个带未知枚举字段的结构体。
	ir.CommonStructs = append(ir.CommonStructs, Struct{
		StructType: "simple",
		Name:       "FakeEntity",
		Fields: []Field{{
			FieldType: "enum",
			Name:      "brand_new_enum",
			Values:    []string{"a", "b"},
		}},
	})
	_, err = newGen(ir)
	if err == nil {
		t.Fatal("未命名的枚举字段应报错")
	}
	if !strings.Contains(err.Error(), "brand_new_enum") {
		t.Fatalf("错误信息应指出字段名，实际: %v", err)
	}
}
