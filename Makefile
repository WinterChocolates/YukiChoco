# YukiChoco OneBot v11 SDK — 构建入口
#
#   make check      格式 / vet / 测试 / race 全量校验
#   make cover      覆盖率（排除 examples）
#   make bench      基准测试

GO ?= go
PKGS := $(shell $(GO) list ./...)

.PHONY: all fmt vet test race check cover bench clean

all: check

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet $(PKGS)

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

check: fmt vet test race

# 覆盖率：排除 examples/，给出总行覆盖率
cover:
	$(GO) test ./... -coverprofile=coverage.out -covermode=atomic -coverpkg=./...
	@grep -v -e '/examples/' coverage.out > coverage.filtered.out
	@$(GO) tool cover -func=coverage.filtered.out | tail -1
	@rm -f coverage.out coverage.filtered.out

# 基准测试（事件分发 / CQ 编解码 / 消息解析）
bench:
	$(GO) test -run '^$$' -bench . -benchmem -benchtime=500ms ./ ./event ./types ./segment

clean:
	$(GO) clean ./...
	@rm -f coverage.out coverage.filtered.out
