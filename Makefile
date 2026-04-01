.DEFAULT_GOAL := help

GO ?= go
APP_NAME ?= cliproxyapi
PORT ?= 8318
CONFIG ?= ./config.yaml
SERVER_PKG ?= ./cmd/server

ifeq ($(OS),Windows_NT)
BIN := $(APP_NAME).exe
else
BIN := $(APP_NAME)
endif

.PHONY: help build test test-short fmt vet run run-config start stop restart verify clean

help:
	@echo "Available targets:"
	@echo "  make build       - build server binary"
	@echo "  make test        - run all tests (timeout=60s)"
	@echo "  make test-short  - run short test suite"
	@echo "  make fmt         - format Go code"
	@echo "  make vet         - run go vet"
	@echo "  make run         - run server with default config.yaml"
	@echo "  make run-config  - run server with CONFIG=<path>"
	@echo "  make start       - start built binary in background"
	@echo "  make stop        - stop process listening on PORT"
	@echo "  make restart     - build + stop + start"
	@echo "  make verify      - test + build"
	@echo "  make clean       - remove built binary"

build:
	$(GO) build -o $(BIN) $(SERVER_PKG)

test:
	$(GO) test ./... -timeout 60s

test-short:
	$(GO) test ./... -short -timeout 60s

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

run:
	$(GO) run $(SERVER_PKG)

run-config:
	$(GO) run $(SERVER_PKG) --config $(CONFIG)

verify: test build

ifeq ($(OS),Windows_NT)
start:
	powershell -NoProfile -Command '$$p = Start-Process -FilePath "$(CURDIR)\$(BIN)" -WorkingDirectory "$(CURDIR)" -PassThru; Write-Output "STARTED_PID=$$($$p.Id)"'

stop:
	powershell -NoProfile -Command '$$listener = Get-NetTCPConnection -LocalPort $(PORT) -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1; if($$listener){ Stop-Process -Id $$listener.OwningProcess -Force; Write-Output "STOPPED_PID=$$($$listener.OwningProcess)" } else { Write-Output "NO_PROCESS_ON_PORT_$(PORT)" }'

clean:
	powershell -NoProfile -Command 'Remove-Item -Force -ErrorAction SilentlyContinue "$(CURDIR)\$(BIN)"'
else
start:
	./$(BIN) > /tmp/$(APP_NAME).log 2>&1 & echo STARTED_PID=$$!

stop:
	@if lsof -ti tcp:$(PORT) >/dev/null 2>&1; then \
		lsof -ti tcp:$(PORT) | xargs kill -9; \
		echo STOPPED_PORT=$(PORT); \
	else \
		echo NO_PROCESS_ON_PORT_$(PORT); \
	fi

clean:
	rm -f ./$(BIN)
endif

restart: build stop start
