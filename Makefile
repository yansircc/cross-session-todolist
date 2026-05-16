PREFIX ?= $(HOME)/.local
BIN ?= $(PREFIX)/bin/cst
CODEX_SKILLS_DIR ?= $(HOME)/.codex/skills
SKILL_NAME ?= cst

.PHONY: test build install skill

test:
	go test ./...

build:
	go build ./...

install:
	mkdir -p "$(dir $(BIN))"
	go build -o "$(BIN)" ./cmd/cst

skill:
	mkdir -p "$(CODEX_SKILLS_DIR)"
	ln -sfn "$(CURDIR)/skills/cst" "$(CODEX_SKILLS_DIR)/$(SKILL_NAME)"
