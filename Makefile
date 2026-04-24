SHELL := /bin/bash
GO    ?= go

PKG        := github.com/pagombin/fragmention-poc
VERSION    ?= 0.1.0-dev
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILDTIME  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
              -X $(PKG)/internal/version.Version=$(VERSION) \
              -X $(PKG)/internal/version.Commit=$(COMMIT) \
              -X $(PKG)/internal/version.BuildTime=$(BUILDTIME)

BIN_DIR    := bin
BIN        := $(BIN_DIR)/mfpoc

.PHONY: all
all: fmt lint test build

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	$(GO) test -race -count=1 ./...

.PHONY: test-cover
test-cover:
	$(GO) test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -func=coverage.out | tail -n 20

.PHONY: test-integration
test-integration:
	$(GO) test -tags=integration -race -count=1 -timeout=10m ./test/integration/...

.PHONY: build
build: $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/mfpoc

.PHONY: build-linux-amd64
build-linux-amd64: $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/mfpoc-linux-amd64 ./cmd/mfpoc

.PHONY: run
run: build
	./$(BIN) server --config configs/config.dev.yaml

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html

.PHONY: frontend-dev
frontend-dev:
	cd web && npm run dev

.PHONY: frontend-build
frontend-build:
	cd web && npm install && npm run build

.PHONY: docker
docker:
	docker build -f deploy/docker/Dockerfile -t mfpoc:$(VERSION) .

.PHONY: deploy-droplet
deploy-droplet: build-linux-amd64
	@test -n "$$DROPLET_IP" || (echo "DROPLET_IP is required" && exit 1)
	@test -n "$$DROPLET_USER" || (echo "DROPLET_USER is required" && exit 1)
	scp $(BIN_DIR)/mfpoc-linux-amd64 deploy/droplet/install.sh deploy/droplet/mfpoc.service deploy/droplet/config.droplet.yaml $$DROPLET_USER@$$DROPLET_IP:/tmp/
	ssh $$DROPLET_USER@$$DROPLET_IP "sudo bash /tmp/install.sh"

$(BIN_DIR):
	mkdir -p $@
