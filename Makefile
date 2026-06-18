# harbormaster — build / test tasks

GO      ?= go
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

GOBIN := $(shell $(GO) env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell $(GO) env GOPATH)/bin
endif

# Where `make install-skill` drops the Claude Code skill (override for a project:
#   make install-skill SKILL_DIR=/path/to/repo/.claude/skills/harbormaster)
SKILL_DIR ?= $(HOME)/.claude/skills/harbormaster

.PHONY: all build install install-skill test vet fmt tidy clean

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

## install-skill: install the Claude Code skill so agents can use it anywhere
## (defaults to ~/.claude/skills; override SKILL_DIR to vendor into a project)
install-skill:
	@rm -rf "$(SKILL_DIR)"
	@mkdir -p "$(SKILL_DIR)"
	@cp -R .claude/skills/harbormaster/. "$(SKILL_DIR)/"
	@echo "installed harbormaster skill -> $(SKILL_DIR)"

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
