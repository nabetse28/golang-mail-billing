# -------------------------------
# Project settings
# -------------------------------
APP_NAME := gmail-billing
MAIN_PATH := ./main.go

# Auto-versioning
VERSION := $(shell git describe --tags --always --dirty)
BUILD_DATE := $(shell date +%Y-%m-%dT%H:%M:%S)
LD_FLAGS := "-X main.Version=$(VERSION) -X main.BuildDate=$(BUILD_DATE)"

# Output directory
DIST := dist

# -------------------------------
# Default target
# -------------------------------
.PHONY: all
all: build

# -------------------------------
# Standard local build (current OS/ARCH)
# -------------------------------
.PHONY: build
build:
	@echo "Building for local machine..."
	GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME) $(MAIN_PATH)
	@echo "Binary created: $(DIST)/$(APP_NAME)"

# -------------------------------
# Clean build artifacts
# -------------------------------
.PHONY: clean
clean:
	@echo "Cleaning dist directory..."
	rm -rf $(DIST)

# -------------------------------
# Multi-arch Linux builds
# -------------------------------
.PHONY: build-linux
build-linux: clean
	@echo "Building Linux binaries (amd64, arm64)..."
	mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-linux-arm64 $(MAIN_PATH)
	@echo "Linux builds completed."

# -------------------------------
# Multi-OS builds (macOS & Windows)
# -------------------------------
.PHONY: build-desktop
build-desktop: clean
	mkdir -p $(DIST)
	@echo "Building macOS binaries..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)

	@echo "Building Windows binaries..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)

	@echo "Desktop builds completed."

# -------------------------------
# Full Release (ALL architectures)
# -------------------------------
.PHONY: release
release: clean
	mkdir -p $(DIST)

	@echo "Building full multi-platform release..."

	# Linux
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-linux-arm64 $(MAIN_PATH)

	# macOS
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)

	# Windows
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
		go build -ldflags $(LD_FLAGS) -o $(DIST)/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)

	@echo ""
	@echo "----------------------------------------"
	@echo " Release build complete:"
	@ls -lh $(DIST)
	@echo "----------------------------------------"

# -------------------------------
# Install binary into /usr/local/bin
# -------------------------------
.PHONY: install
install: build
	@echo "Installing $(APP_NAME) to /usr/local/bin..."

	# Ensure /usr/local/bin exists
	@if [ ! -d /usr/local/bin ]; then \
		echo "Creating /usr/local/bin ..."; \
		sudo mkdir -p /usr/local/bin; \
	fi

	# Copy appropriate binary
	@if [ -f "$(DIST)/$(APP_NAME)" ]; then \
		echo "Installing compiled binary..."; \
		sudo cp "$(DIST)/$(APP_NAME)" /usr/local/bin/$(APP_NAME); \
		sudo chmod +x /usr/local/bin/$(APP_NAME); \
		echo "Installed → /usr/local/bin/$(APP_NAME)"; \
	else \
		echo "ERROR: $(DIST)/$(APP_NAME) does not exist. Did you run 'make build'?"; \
		exit 1; \
	fi

# -------------------------------
# Uninstall binary from /usr/local/bin
# -------------------------------
.PHONY: uninstall
uninstall:
	@echo "Uninstalling $(APP_NAME) from /usr/local/bin..."

	@if [ -f "/usr/local/bin/$(APP_NAME)" ]; then \
		echo "Removing /usr/local/bin/$(APP_NAME)..."; \
		sudo rm -f /usr/local/bin/$(APP_NAME); \
		echo "Uninstall complete."; \
	else \
		echo "Nothing to uninstall: /usr/local/bin/$(APP_NAME) does not exist."; \
	fi
