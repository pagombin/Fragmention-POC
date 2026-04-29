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
build: frontend-embed $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/mfpoc

.PHONY: build-linux-amd64
build-linux-amd64: frontend-embed $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/mfpoc-linux-amd64 ./cmd/mfpoc

.PHONY: run
run: build
	./$(BIN) server --config configs/config.dev.yaml

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html internal/api/webui/dist
	mkdir -p internal/api/webui/dist
	printf '<!doctype html><meta charset=utf-8><title>mfpoc</title><p>SPA not built.' > internal/api/webui/dist/index.html

.PHONY: frontend-dev
frontend-dev:
	cd web && npm run dev

.PHONY: frontend-build
frontend-build:
	cd web && npm install && npm run build

.PHONY: frontend-embed
frontend-embed:
	@cd web && npm install --silent && npm run build
	@rm -rf internal/api/webui/dist
	@mkdir -p internal/api/webui/dist
	@cp -r web/dist/. internal/api/webui/dist/

.PHONY: docker
docker:
	docker build -f deploy/docker/Dockerfile -t mfpoc:$(VERSION) \
	  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILDTIME=$(BUILDTIME) .

.PHONY: docker-up
docker-up:
	docker compose -f deploy/docker/docker-compose.yaml up -d --build

.PHONY: docker-down
docker-down:
	docker compose -f deploy/docker/docker-compose.yaml down -v

.PHONY: test-e2e
test-e2e:
	$(GO) test -tags=integration -race -count=1 -timeout=15m ./test/e2e/...

.PHONY: deploy-droplet
deploy-droplet: build-linux-amd64
	@test -n "$$DROPLET_IP" || (echo "DROPLET_IP is required (export DROPLET_IP=...)" && exit 1)
	@test -n "$$DROPLET_USER" || (echo "DROPLET_USER is required (export DROPLET_USER=root)" && exit 1)
	@echo "→ uploading binary + install scripts to $$DROPLET_USER@$$DROPLET_IP"
	scp -q $(BIN_DIR)/mfpoc-linux-amd64 \
	    deploy/droplet/install.sh \
	    deploy/droplet/mfpoc.service \
	    deploy/droplet/config.droplet.yaml \
	    $$DROPLET_USER@$$DROPLET_IP:/tmp/
	@echo "→ running install.sh on $$DROPLET_IP"
	ssh -t $$DROPLET_USER@$$DROPLET_IP "sudo bash /tmp/install.sh"

# Alias - functionally identical to deploy-droplet, but documents the upgrade
# semantic for operators who want to be explicit.
.PHONY: redeploy-droplet
redeploy-droplet: deploy-droplet

# Print the current bearer token from a running droplet without redeploying.
.PHONY: show-droplet-token
show-droplet-token:
	@test -n "$$DROPLET_IP" || (echo "DROPLET_IP is required" && exit 1)
	@test -n "$$DROPLET_USER" || (echo "DROPLET_USER is required" && exit 1)
	@ssh $$DROPLET_USER@$$DROPLET_IP "sudo grep -E '^MFPOC_AUTH_BEARER_TOKEN=' /etc/mfpoc/mfpoc.env | cut -d= -f2-"

# Tail logs from a running droplet.
.PHONY: tail-droplet-logs
tail-droplet-logs:
	@test -n "$$DROPLET_IP" || (echo "DROPLET_IP is required" && exit 1)
	@test -n "$$DROPLET_USER" || (echo "DROPLET_USER is required" && exit 1)
	ssh -t $$DROPLET_USER@$$DROPLET_IP "sudo journalctl -u mfpoc -f"

$(BIN_DIR):
	mkdir -p $@
