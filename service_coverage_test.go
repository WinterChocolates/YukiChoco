package YukiChoco

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"github.com/WinterChocolates/YukiChoco/types"
)

// TestServiceCoverageMatchesIR 用 AST 解析生成的 service 文件，
// 断言方法集合与 types.EndpointService（源自 Milky IR）完全一致。
//
// 这样即使生成器逻辑被改动，也不会出现「端点少生成 / 方法名漂移」而测试仍通过。
func TestServiceCoverageMatchesIR(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return strings.HasPrefix(fi.Name(), "service_") && strings.HasSuffix(fi.Name(), "_gen.go")
	}, 0)
	if err != nil {
		t.Fatalf("解析生成文件失败: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("生成文件应属于同一个包，实际 %d 个", len(pkgs))
	}

	got := map[string]map[string]bool{}
	impls := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if it, ok := ts.Type.(*ast.InterfaceType); ok {
						name := ts.Name.Name
						if !strings.HasSuffix(name, "Service") {
							t.Errorf("生成文件里出现非 Service 接口: %s", name)
							continue
						}
						set := got[name]
						if set == nil {
							set = map[string]bool{}
							got[name] = set
						}
						for _, m := range it.Methods.List {
							for _, n := range m.Names {
								set[n.Name] = true
							}
						}
						continue
					}
					impls[ts.Name.Name] = true
				}
			}
		}
	}

	want := map[string]map[string]bool{}
	for _, spec := range types.EndpointService {
		svc := spec.Service + "Service"
		if want[svc] == nil {
			want[svc] = map[string]bool{}
		}
		if want[svc][spec.Method] {
			t.Errorf("%s 出现重复方法 %s", svc, spec.Method)
		}
		want[svc][spec.Method] = true
	}

	if len(got) != len(want) {
		t.Errorf("service 数量 = %d, want %d", len(got), len(want))
	}
	total := 0
	for svc, wantSet := range want {
		gotSet := got[svc]
		if gotSet == nil {
			t.Errorf("缺少接口 %s", svc)
			continue
		}
		total += len(gotSet)
		if len(gotSet) != len(wantSet) {
			t.Errorf("%s 方法数量 = %d, want %d", svc, len(gotSet), len(wantSet))
		}
		for m := range wantSet {
			if !gotSet[m] {
				t.Errorf("%s 缺少方法 %s", svc, m)
			}
		}
		for m := range gotSet {
			if !wantSet[m] {
				t.Errorf("%s 多出方法 %s", svc, m)
			}
		}
		// 每个接口都应有对应的实现结构体（首字母小写 + Impl 后缀的约定在此不强制，
		// 但必须存在一个实现了它的类型；用命名约定做弱校验）。
		implName := strings.ToLower(svc[:1]) + svc[1:]
		if !impls[implName] {
			t.Errorf("缺少 %s 的实现结构体 %s", svc, implName)
		}
	}
	if total != 65 {
		t.Errorf("service 方法总数 = %d, want 65", total)
	}
}

// TestServiceAccessors 确保 5 个 service 都能从 Client 取到。
func TestServiceAccessors(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
	if c.System() == nil || c.Message() == nil || c.Friend() == nil || c.Group() == nil || c.File() == nil {
		t.Fatal("service 访问器返回了 nil")
	}
}
