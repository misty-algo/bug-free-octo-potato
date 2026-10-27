GO ?= go
PROTOC ?= protoc
PROTO_PATH ?= api/proto
PROTO_OUT ?= api/gen

export PROTOC
export PROTO_PATH
export PROTO_OUT

.PHONY: proto test tidy run-gateway run-user run-cli install-cli install-bw-cli tools run-order run-driver mint-token run-shop

tools:
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

proto:
	$(GO) run ./tools/protogen

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

run-user:
	$(GO) run ./cmd/user

run-gateway:
	$(GO) run ./cmd/gateway

run-cli:
	$(GO) run ./cmd/bw-cli

install-cli:
	$(GO) install ./cmd/bw-cli

install-bw-cli: install-cli

run-order:
	$(GO) run ./cmd/order

run-driver:
	$(GO) run ./cmd/driver

# 联调工具：直接签发令牌，跳过尚未实现的登录链路。
# 例：make mint-token ARGS="6ad9adcc77e161c8570967b3 driver"
mint-token:
	$(GO) run ./cmd/mint-token $(ARGS)

run-shop:
	$(GO) run ./cmd/shop