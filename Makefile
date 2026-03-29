# Extract version from package.json
VERSION := $(shell node -p "require('./package.json').version")
LDFLAGS := -X 'application-use/internal/appuse.Version=$(VERSION)' -extldflags '-Wl,-no_warn_duplicate_libraries'

# Build the Go application for specific architectures
build-arm64:
	@echo "Building Swift bridge for arm64..."
	$(MAKE) -C internal/cgo/macos/appuse_bridge ARCH=arm64
	@echo "Building application-use (arm64)..."
	mkdir -p bin
	CGO_ENABLED=1 GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/application-use-darwin-arm64 cmd/application-use/main.go

build-amd64:
	@echo "Building Swift bridge for x86_64..."
	$(MAKE) -C internal/cgo/macos/appuse_bridge ARCH=x86_64
	@echo "Building application-use (amd64)..."
	mkdir -p bin
	CGO_ENABLED=1 GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/application-use-darwin-x64 cmd/application-use/main.go

# Alias for x86_64
build-x86_64: build-amd64

# Default target: build both
all: build-arm64 build-amd64

# Package for NPM (aliases all)
package-npm: all
	@echo "NPM package ready in current directory."

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f bin/application-use-darwin-arm64 bin/application-use-darwin-x64
	$(MAKE) -C internal/cgo/macos/appuse_bridge clean || true
	rm -f internal/cgo/macos/appuse_bridge/*.a internal/cgo/macos/appuse_bridge/*.dylib

help:
	@echo "Usage:"
	@echo "  make          Build the entire project"
	@echo "  make clean    Remove build artifacts"
	@echo "  make help     Show this help message"
