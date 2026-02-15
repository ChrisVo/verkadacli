.PHONY: build test tidy clean

BIN := bin/verkcli
PKG := ./cmd/verkcli

# In some sandboxed environments, Go can't write to ~/Library/Caches/go-build.
# Use repo-local caches so `make build` / `make test` works everywhere.
GOCACHE_DIR := $(CURDIR)/.cache/go-build
GOMODCACHE_DIR := $(CURDIR)/.cache/go-mod

build:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@mkdir -p "$(dir $(BIN))"
	@env GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go build -o "$(BIN)" "$(PKG)"
	@echo "built ./$(BIN)"

test:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@env GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go test ./...

tidy:
	@mkdir -p "$(GOCACHE_DIR)" "$(GOMODCACHE_DIR)"
	@env GOCACHE="$(GOCACHE_DIR)" GOMODCACHE="$(GOMODCACHE_DIR)" go mod tidy

clean:
	@# Go module cache files are often read-only; make them writable so clean works reliably.
	@chmod -R u+w "./.cache" 2>/dev/null || true
	@rm -rf "./bin" "./.cache"
