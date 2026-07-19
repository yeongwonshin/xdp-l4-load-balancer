APP := xdp-l4lb
PKG := ./cmd/xdp-l4lb
IFACE ?= eth0
CONFIG ?= configs/example.yaml
XDP_MODE ?= generic
BPF_CLANG ?= clang
GO ?= go

GO_FILES := $(shell find cmd -type f -name '*.go' ! -name 'bpf_bpf*.go' | sort)
GENERATED_BPF := cmd/xdp-l4lb/bpf_bpfel.go cmd/xdp-l4lb/bpf_bpfel.o cmd/xdp-l4lb/bpf_bpfeb.go cmd/xdp-l4lb/bpf_bpfeb.o

.PHONY: all generate build run config-check test vet fmt format-check check clean

all: build

generate:
	BPF_CLANG=$(BPF_CLANG) $(GO) generate $(PKG)

build: generate
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/$(APP) $(PKG)

run: build
	sudo ./bin/$(APP) -iface $(IFACE) -config $(CONFIG) -xdp-mode $(XDP_MODE)

config-check: build
	./bin/$(APP) -config $(CONFIG) -check-config

test: generate
	$(GO) test ./...

vet: generate
	$(GO) vet ./...

fmt:
	gofmt -w $(GO_FILES)

format-check:
	@test -z "$$(gofmt -l $(GO_FILES))" || { gofmt -l $(GO_FILES); exit 1; }

check: format-check test vet

clean:
	rm -rf bin $(GENERATED_BPF)
