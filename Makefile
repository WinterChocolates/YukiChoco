# YukiChoco Milky SDK — 构建入口
#
#   make generate   由 protocol/protocol-1.3.0.json 重新生成 types 包
#   make check      生成 / vet / test / race 全量校验（含生成物一致性）
#   make spec-check 拉取上游最新 Milky IR 并与本地锁定版本对比

GO ?= go
SPEC_VERSION ?= 1.3.0
SPEC_URL ?= https://unpkg.com/@saltify/milky-protocol@$(SPEC_VERSION)/dist/protocol.json

.PHONY: all generate fmt vet test race check spec-fetch spec-check clean

all: check

generate:
	$(GO) run ./internal/codegen -spec protocol/protocol-$(SPEC_VERSION).json -out types -services .

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

# 生成物必须与 IR 一致且不允许手改：由 internal/codegen 的 TestGolden 兜底
# （它把 IR 重新生成到临时目录后与仓库文件逐字节比对，不依赖 git 状态）。
check: generate fmt vet test race

spec-fetch:
	curl -fsSL -o /tmp/milky-protocol-latest.json $(SPEC_URL)
	@echo "上游 IR 已下载到 /tmp/milky-protocol-latest.json"

spec-check: spec-fetch
	@diff -u protocol/protocol-$(SPEC_VERSION).json /tmp/milky-protocol-latest.json && echo "IR 与上游一致"

clean:
	$(GO) clean ./...
