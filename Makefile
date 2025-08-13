#---------------------------
#---------------------------
# VARIABLES
#---------------------------
#---------------------------

GOLANG_SUPPORTED_PLATFORMS=linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
APP_NAME=updateGit
MAIN_PACKAGE=.
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean

# Version information
VERSION ?= $(shell go run . -v)
COMMIT ?= $(shell git rev-parse --short HEAD)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)"
#----------------------------------------------------------------------------------------------------------




#---------------------------
#---------------------------
# MAIN
#---------------------------
#---------------------------

# References
# https://ryanstutorials.net/bash-scripting-tutorial/bash-input.php
# https://stackoverflow.com/questions/3743793/makefile-why-is-the-read-command-not-reading-the-user-input
# https://stackoverflow.com/questions/60147129/interactive-input-of-a-makefile-variable
# https://makefiletutorial.com/
# https://stackoverflow.com/questions/589276/how-can-i-use-bash-syntax-in-makefile-targets
# https://til.hashrocket.com/posts/k3kjqxtppx-escape-dollar-sign-on-makefiles
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile
# https://www.docker.com/blog/multi-arch-build-and-images-the-simple-way/


requirements:
REQUIRED_PACKAGES := go git wget trivy

$(foreach package,$(REQUIRED_PACKAGES),\
	$(if $(shell command -v $(package) 2> /dev/null),$(info Found `$(package)`),$(error Please install `$(package)`)))

prepare:
	make requirements
# Install go packages
	go mod download

.PHONY: build
build: prepare build-linux-and-darwin ## Build for all platforms

.PHONY: build-linux-and-darwin
build-linux-and-darwin: 
	mkdir -p $(BUILD_DIR)

# Build for Linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 $(MAIN_PACKAGE)

# Build for macOS
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PACKAGE)

# Generate SBOM (Software Bill of Materials) using Trivy
## References:
##https://www.bytesizego.com/lessons/sbom-golang
##https://medium.com/@krishnaduttpanchagnula/vulnerability-identification-of-images-and-files-using-sbom-with-trivy-23e1a4a5eea4
	echo "Generating SBOM (Software Bill of Materials)..."
	trivy fs --format cyclonedx --output bin/${APP_NAME}.sbom.json .

	echo "Generate SHA256 checksums for all the built artifacts..."
	sha256sum $(BUILD_DIR)/* > $(BUILD_DIR)/checksums.txt
	echo "Build completed. Binaries are located in the $(BUILD_DIR) directory."
	echo "Binaries:"
	ls -l $(BUILD_DIR)

# Clean targets
.PHONY: clean
clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

# Default target
.PHONY: all
all: clean build
