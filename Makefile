# harbormaster — build / test tasks

GO      ?= go
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

GOBIN := $(shell $(GO) env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell $(GO) env GOPATH)/bin
endif

.PHONY: all build install test vet fmt tidy clean

all: build

## build: compile both binaries into ./bin and symlink hm -> harbormaster
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/harbormaster ./cmd/harbormaster
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/harbormasterd ./cmd/harbormasterd
	@ln -sf harbormaster $(BIN_DIR)/hm

## install: go install both binaries and symlink hm -> harbormaster in GOBIN
install:
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/harbormaster
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/harbormasterd
	@ln -sf harbormaster $(GOBIN)/hm

## test: run the full test suite
test:
	$(GO) test ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## fmt: gofmt the tree
fmt:
	$(GO) fmt ./...

## tidy: sync go.mod / go.sum
tidy:
	$(GO) mod tidy

## clean: remove build output
clean:
	rm -rf $(BIN_DIR)
