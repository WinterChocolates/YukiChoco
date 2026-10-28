package main

import (
	"flag"
	"log"
)

const (
	// defaultModule 是生成代码里引用的模块路径。
	defaultModule = "github.com/WinterChocolates/YukiChoco"
	// defaultPkg 是根包名（与模块路径最后一段不同，属于有意为之）。
	defaultPkg = "YukiChoco"
)

func main() {
	log.SetFlags(0)
	spec := flag.String("spec", "protocol/protocol-1.3.0.json", "Milky IR 文件路径")
	out := flag.String("out", "types", "types 包生成输出目录")
	services := flag.String("services", ".", "service 实现生成输出目录（根包）")
	module := flag.String("module", defaultModule, "模块导入路径，用于生成 types 的 import")
	pkg := flag.String("pkg", defaultPkg, "根包名")
	flag.Parse()

	ir, err := Load(*spec)
	if err != nil {
		log.Fatalf("codegen: %v", err)
	}
	g, err := newGen(ir)
	if err != nil {
		log.Fatalf("codegen: IR 校验失败: %v", err)
	}

	// 1. types 包
	if err := g.emitAll(*out); err != nil {
		log.Fatalf("codegen: %v", err)
	}
	// 2. 根包 service 实现
	if err := g.emitServices(*services, *pkg, *module); err != nil {
		log.Fatalf("codegen: %v", err)
	}

	log.Printf("codegen: Milky %s (%s) → %s/ + %s/：%d 个公共结构体、%d 个端点、%d 个 service",
		ir.MilkyVersion, ir.MilkyPackageVersion, *out, *services,
		len(ir.CommonStructs), len(ir.APIList()), len(serviceOrder))
}
