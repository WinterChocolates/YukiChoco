package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)
	spec := flag.String("spec", "protocol/protocol-1.3.0.json", "Milky IR 文件路径")
	out := flag.String("out", "types", "生成文件输出目录")
	flag.Parse()

	ir, err := Load(*spec)
	if err != nil {
		log.Fatalf("codegen: %v", err)
	}
	g, err := newGen(ir)
	if err != nil {
		log.Fatalf("codegen: IR 校验失败: %v", err)
	}
	if err := g.emitAll(*out); err != nil {
		log.Fatalf("codegen: %v", err)
	}

	log.Printf("codegen: Milky %s (%s) → %s/：%d 个公共结构体、%d 个端点",
		ir.MilkyVersion, ir.MilkyPackageVersion, *out, len(ir.CommonStructs), len(ir.APIList()))
	if _, err := os.Stat(*out); err != nil {
		log.Fatalf("codegen: 输出目录不可用: %v", err)
	}
}
