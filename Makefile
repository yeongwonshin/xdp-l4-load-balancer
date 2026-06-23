APP := xdp-l4lb
PKG := ./cmd/xdp-l4lb
IFACE ?= eth0
CONFIG ?= configs/example.yaml
XDP_MODE ?= generic

.PHONY: all generate build run test clean fmt

all: build

generate:
	go generate $(PKG)

build: generate
	go build -o bin/$(APP) $(PKG)

run: build
	sudo ./bin/$(APP) -iface $(IFACE) -config $(CONFIG) -xdp-mode $(XDP_MODE)

test:
	go test ./...

fmt:
	gofmt -w cmd scripts || true

clean:
	rm -rf bin cmd/xdp-l4lb/bpf_bpf*.go cmd/xdp-l4lb/bpf_bpf*.o
