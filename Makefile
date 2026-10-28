# YukiChoco Milky SDK — 构建入口
#
#   make generate   由 protocol/protocol-1.3.0.json 重新生成 types 包
#   make check      生成物 diff / vet / test / race 全量校验
#   make spec-check 拉取上游最新 Milky IR 并与本地锁定版本对比

GO ?= go
SPEC_VERSION ?= 1.3.0
SPEC_URL ?= https://unpkg.com/@saltify/milky-protocol@$(SPEC_VERSION)/dist/protocol.json

.PHONY: all generate fmt vet test race check spec-fetch spec-check clean

all: check

generate:
	$(GO) run ./internal/codegen -spec protocol/protocol-$(SPEC_VERSION).json -out types

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

# 生成物必须与 IR 一致，且不允许手改
check: generate fmt vet test race
	@if git rev-parse --git-dir >/dev/null 2>&1; then \
		git diff --exit-code -- types || (echo "types/ 与 IR 不一致：生成物被手改或未重新生成" && exit 1); \
	else \
		echo "跳过生成物 diff（当前目录不是 git 仓库）"; \
	fi

spec-fetch:
	curl -fsSL -o /tmp/milky-protocol-latest.json $(SPEC_URL)
	@echo "上游 IR 已下载到 /tmp/milky-protocol-latest.json"

spec-check: spec-fetch
	@diff -u protocol/protocol-$(SPEC_VERSION).json /tmp/milky-protocol-latest.json && echo "IR 与上游一致"

clean:
	$(GO) clean ./...
