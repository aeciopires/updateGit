#---------------------------
#---------------------------
# VARIABLES
#---------------------------
#---------------------------

# Supported platforms (GOOS/GOARCH). The asset names (updateGit-<os>-<arch>) are a
# contract with "updateGit update": do not change them.
GOLANG_SUPPORTED_PLATFORMS=linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
APP_NAME=updateGit
MAIN_PACKAGE=.
BUILD_DIR=bin
CONFIG_PKG=github.com/aeciopires/updateGit/internal/config

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean

# Version information (VERSION comes from config.CLIVersion)
VERSION ?= $(shell $(GOCMD) run . -v 2>/dev/null)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags: commit and build date are shown by "updateGit -V"
LDFLAGS=-ldflags "-X $(CONFIG_PKG).CLICommit=$(COMMIT) -X $(CONFIG_PKG).CLIBuildDate=$(DATE)"

# sha256sum on Linux, shasum on macOS (same output format)
SHA256SUM := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")
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


# Running "make" without a target shows the help
.DEFAULT_GOAL := help

# Colors used in messages
RED    := \033[31m
GREEN  := \033[32m
YELLOW := \033[33m
RESET  := \033[0m

##@ Help
# To document a new target, add a comment starting with "## " after the target
# name (e.g. "my-target: ## Description"). To create a new section in the help
# output, add a line starting with "##@ " (e.g. "##@ Tests").

.PHONY: help
help: ## Show this help with the available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z0-9_.-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo ""

##@ Dependencies

# Commands required to build the project (check-deps fails if one is missing).
REQUIRED_PACKAGES := git go trivy
# Commands used by the development checks (check-deps only warns if one is missing).
DEV_PACKAGES := golangci-lint govulncheck actionlint shellcheck pre-commit
# Versions of the tools are pinned in $(TOOLS_FILE) and installed by mise.
TOOLS_FILE := mise.toml

.PHONY: check-deps
check-deps: ## Check if the software dependencies are installed (required and development tools)
	@echo "Checking software dependencies..."
	@missing=""; missing_dev=""; \
	for cmd in $(REQUIRED_PACKAGES) $(DEV_PACKAGES); do \
		required=no; \
		case " $(REQUIRED_PACKAGES) " in *" $$cmd "*) required=yes ;; esac; \
		case $$cmd in \
			go)          version_cmd="go version" ;; \
			govulncheck) version_cmd="govulncheck -version" ;; \
			*)           version_cmd="$$cmd --version" ;; \
		esac; \
		if ! command -v $$cmd >/dev/null 2>&1 || ! output=$$($$version_cmd 2>/dev/null); then \
			if [ $$required = yes ]; then \
				printf "  $(RED)[MISSING]$(RESET) %-14s not installed or not available in the PATH\n" "$$cmd"; \
				missing="$$missing $$cmd"; \
			else \
				printf "  $(YELLOW)[MISSING]$(RESET) %-14s not installed (development tool). Run 'make install-deps'\n" "$$cmd"; \
				missing_dev="$$missing_dev $$cmd"; \
			fi; \
			continue; \
		fi; \
		if [ $$cmd = govulncheck ]; then output=$$(echo "$$output" | grep Scanner); fi; \
		version=$$(echo "$$output" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n 1); \
		expected=$$(sed -n "s/^\"*[a-z:./-]*$$cmd\"* *= *\"\(.*\)\"/\1/p" $(TOOLS_FILE) 2>/dev/null | head -n 1); \
		if [ -n "$$expected" ] && [ "$$version" != "$$expected" ]; then \
			printf "  $(YELLOW)[WARNING]$(RESET) %-14s %s (expected version $$expected from $(TOOLS_FILE))\n" "$$cmd" "$$version"; \
		else \
			printf "  $(GREEN)[OK]$(RESET)      %-14s %s\n" "$$cmd" "$$version"; \
		fi; \
	done; \
	if command -v mise >/dev/null 2>&1; then \
		printf "  $(GREEN)[OK]$(RESET)      %-14s %s\n" "mise" "$$(mise --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n 1)"; \
	else \
		printf "  $(YELLOW)[WARNING]$(RESET) %-14s not installed (required by 'make install-deps'). See https://mise.jdx.dev/getting-started.html\n" "mise"; \
	fi; \
	if [ -n "$$missing" ]; then \
		printf "\n$(RED)[ERROR]$(RESET) Missing software dependencies:$$missing\n"; \
		printf "Run 'make install-deps' to install them. If they were installed with mise, make sure mise is activated\n"; \
		printf "in your shell: https://mise.jdx.dev/getting-started.html#activate-mise\n"; \
		exit 1; \
	fi; \
	if [ -n "$$missing_dev" ]; then \
		printf "\n$(YELLOW)[WARNING]$(RESET) Missing development tools:$$missing_dev. Run 'make install-deps'.\n"; \
	else \
		printf "\n$(GREEN)All software dependencies are installed.$(RESET)\n"; \
	fi

.PHONY: install-deps
install-deps: ## Install the software dependencies with mise (versions in mise.toml) and download the Go modules
	@if ! command -v mise >/dev/null 2>&1; then \
		printf "$(RED)[ERROR]$(RESET) mise is not installed. Install it following https://mise.jdx.dev/getting-started.html\n"; \
		exit 1; \
	fi
	@if ! command -v git >/dev/null 2>&1; then \
		printf "$(RED)[ERROR]$(RESET) git is not installed. Install it with your package manager, e.g.:\n"; \
		printf "  Ubuntu/Debian: sudo apt install git\n  macOS:         brew install git\n"; \
		exit 1; \
	fi
	mise trust --quiet $(TOOLS_FILE)
	mise install
	mise exec -- $(GOCMD) mod download
	@$(MAKE) --no-print-directory check-deps

.PHONY: install-hooks
install-hooks: ## Install the git hooks (pre-commit and pre-push) with pre-commit
	pre-commit install --hook-type pre-commit --hook-type pre-push

.PHONY: requirements
requirements: check-deps ## Alias to check-deps (kept for compatibility)

.PHONY: prepare
prepare: check-deps ## Check the software dependencies and download the Go modules
	$(GOCMD) mod download

##@ Tests and code quality

COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
# Minimum total test coverage (percent) required by coverage-check
COVERAGE_MIN ?= 90
# The race detector requires cgo and a C compiler. Disable it with: make test RACE_FLAG=
RACE_FLAG ?= $(if $(shell command -v cc 2>/dev/null),-race,)

.PHONY: test
test: ## Run the unit tests with race detector and coverage
	$(GOCMD) test $(RACE_FLAG) -coverprofile=$(COVERAGE_FILE) ./...

.PHONY: test-short
test-short: ## Run only the fast unit tests (skips the load tests)
	$(GOCMD) test $(RACE_FLAG) -short ./...

.PHONY: test-verbose
test-verbose: ## Run the unit tests in verbose mode
	$(GOCMD) test $(RACE_FLAG) -v -count=1 ./...

.PHONY: coverage
coverage: test ## Run the unit tests and show the coverage per function
	$(GOCMD) tool cover -func=$(COVERAGE_FILE)

.PHONY: coverage-html
coverage-html: test ## Run the unit tests and generate the HTML coverage report (coverage.html)
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report: $(COVERAGE_HTML)"

.PHONY: coverage-check
coverage-check: test ## Fail if the total test coverage is below COVERAGE_MIN (default 90)
	@total=$$($(GOCMD) tool cover -func=$(COVERAGE_FILE) | awk '/^total:/ {sub("%", "", $$3); print $$3}'); \
	if awk "BEGIN {exit !($$total < $(COVERAGE_MIN))}"; then \
		printf "$(RED)[ERROR]$(RESET) Total coverage $$total%% is below the minimum $(COVERAGE_MIN)%%. Add tests (see 'make coverage').\n"; \
		exit 1; \
	fi; \
	printf "$(GREEN)[OK]$(RESET) Total coverage $$total%% (minimum $(COVERAGE_MIN)%%)\n"

.PHONY: vet
vet: ## Run go vet to find suspicious constructs
	$(GOCMD) vet ./...

.PHONY: lint
lint: ## Run golangci-lint (configuration in .golangci.yml)
	@golangci-lint run ./... || { printf "$(RED)[ERROR]$(RESET) Lint failed. Try 'make lint-fix' for the automatic fixes.\n"; exit 1; }
	@printf "$(GREEN)[OK]$(RESET) Lint passed.\n"

.PHONY: lint-fix
lint-fix: ## Run golangci-lint applying the automatic fixes
	golangci-lint run --fix ./...

.PHONY: lint-workflows
lint-workflows: ## Lint the GitHub Actions workflows with actionlint (and shellcheck)
	@if [ ! -d .github/workflows ]; then echo "No workflows found, skipping."; exit 0; fi; \
	if ! command -v actionlint >/dev/null 2>&1; then \
		printf "$(YELLOW)[WARNING]$(RESET) actionlint is not installed, skipping. Run 'make install-deps'.\n"; exit 0; \
	fi; \
	actionlint && printf "$(GREEN)[OK]$(RESET) Workflows are valid.\n"

# Docker image used to validate the Mermaid diagrams (pinned version)
MERMAID_IMAGE ?= minlag/mermaid-cli:12.0.0

.PHONY: lint-diagrams
lint-diagrams: ## Validate the Mermaid diagrams of the Markdown files with the mermaid-cli Docker image
	@if ! command -v docker >/dev/null 2>&1; then \
		printf "$(YELLOW)[WARNING]$(RESET) docker is not installed, skipping the validation of the Mermaid diagrams.\n"; exit 0; \
	fi; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	for file in $$(find . -name '*.md' -not -path './.git/*' -not -path './bin/*'); do \
		name=$$(echo "$$file" | sed 's#^\./##; s#[/.]#_#g'); \
		awk -v base="$$tmp/$$name" '/^```mermaid/ {n++; out = base "-" n ".mmd"; p = 1; next} p && /^```/ {p = 0; close(out); next} p {print > out}' "$$file"; \
	done; \
	total=0; failed=0; \
	for diagram in "$$tmp"/*.mmd; do \
		[ -e "$$diagram" ] || continue; \
		total=$$((total + 1)); name=$$(basename "$$diagram"); \
		if ! docker run --rm -u "$$(id -u):$$(id -g)" -v "$$tmp:/data" $(MERMAID_IMAGE) -q -i "/data/$$name" -o "/data/$$name.svg" > "$$diagram.log" 2>&1; then \
			failed=$$((failed + 1)); printf "$(RED)[ERROR]$(RESET) Invalid Mermaid diagram: %s\n" "$$name"; tail -n 5 "$$diagram.log"; \
		fi; \
	done; \
	if [ $$failed -gt 0 ]; then printf "$(RED)[ERROR]$(RESET) %d of %d Mermaid diagrams are invalid.\n" $$failed $$total; exit 1; fi; \
	printf "$(GREEN)[OK]$(RESET) %d Mermaid diagrams are valid ($(MERMAID_IMAGE)).\n" $$total

.PHONY: mod-check
mod-check: ## Check if go.mod/go.sum are tidy and verify the downloaded modules
	@$(GOCMD) mod tidy -diff || { printf "$(RED)[ERROR]$(RESET) go.mod/go.sum are not tidy. Run 'go mod tidy'.\n"; exit 1; }
	@$(GOCMD) mod verify
	@printf "$(GREEN)[OK]$(RESET) Go modules are tidy and verified.\n"

.PHONY: vuln
vuln: ## Search for known vulnerabilities in the code and dependencies (govulncheck)
	govulncheck ./...

.PHONY: security
security: ## Scan dependencies, secrets and misconfigurations with trivy (HIGH and CRITICAL)
	trivy fs --no-progress --scanners vuln,secret,misconfig --exit-code 1 --severity HIGH,CRITICAL --skip-dirs $(BUILD_DIR) .

.PHONY: fmt
fmt: ## Format the Go code with gofmt
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Check if the Go code is formatted (gofmt)
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		printf "$(RED)[ERROR]$(RESET) The following files are not formatted. Run 'make fmt':\n$$files\n"; \
		exit 1; \
	fi; \
	printf "$(GREEN)[OK]$(RESET) All Go files are formatted.\n"

.PHONY: check
check: fmt-check mod-check vet lint test ## Run the fast checks used during development (format, modules, vet, lint and tests)

.PHONY: check-all
check-all: check coverage-check vuln security lint-workflows lint-diagrams build verify-artifacts ## Run all checks (check + coverage, vulnerabilities, workflows, diagrams, build and artifacts)

##@ Build and release

.PHONY: build
build: prepare build-linux-and-darwin ## Build for all platforms

.PHONY: build-linux-and-darwin
build-linux-and-darwin: ## Build binaries for Linux/macOS (amd64/arm64), SBOM and checksums
	rm -rf $(BUILD_DIR)
	mkdir -p $(BUILD_DIR)
	@for platform in $(GOLANG_SUPPORTED_PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "Building $(BUILD_DIR)/$(APP_NAME)-$$os-$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GOBUILD) -trimpath $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-$$os-$$arch $(MAIN_PACKAGE) || exit 1; \
	done

# Generate SBOM (Software Bill of Materials) using Trivy
## References:
##https://www.bytesizego.com/lessons/sbom-golang
##https://medium.com/@krishnaduttpanchagnula/vulnerability-identification-of-images-and-files-using-sbom-with-trivy-23e1a4a5eea4
	@echo "Generating SBOM (Software Bill of Materials)..."
	trivy fs --no-progress --format cyclonedx --skip-dirs $(BUILD_DIR) --output $(BUILD_DIR)/$(APP_NAME).sbom.json .

# The "bin/" prefix in checksums.txt is required by "updateGit update" (config.CLICheckSumBinDir)
	@echo "Generating SHA256 checksums for all the built artifacts..."
	$(SHA256SUM) $(BUILD_DIR)/$(APP_NAME)-* $(BUILD_DIR)/$(APP_NAME).sbom.json > $(BUILD_DIR)/checksums.txt
	@echo "Build completed. Binaries are located in the $(BUILD_DIR) directory."
	@ls -l $(BUILD_DIR)

.PHONY: verify-artifacts
verify-artifacts: ## Verify the build artifacts (binaries, SBOM, checksums and version)
	@echo "Verifying artifacts in $(BUILD_DIR)..."
	@for platform in $(GOLANG_SUPPORTED_PLATFORMS); do \
		file="$(BUILD_DIR)/$(APP_NAME)-$${platform%/*}-$${platform#*/}"; \
		if [ ! -s "$$file" ]; then printf "$(RED)[ERROR]$(RESET) Missing $$file. Run 'make build'.\n"; exit 1; fi; \
		printf "  $(GREEN)[OK]$(RESET) $$file\n"; \
	done
	@if [ ! -s $(BUILD_DIR)/$(APP_NAME).sbom.json ]; then printf "$(RED)[ERROR]$(RESET) Missing $(BUILD_DIR)/$(APP_NAME).sbom.json\n"; exit 1; fi
	@printf "  $(GREEN)[OK]$(RESET) $(BUILD_DIR)/$(APP_NAME).sbom.json\n"
	@$(SHA256SUM) -c $(BUILD_DIR)/checksums.txt >/dev/null || { printf "$(RED)[ERROR]$(RESET) Checksums do not match $(BUILD_DIR)/checksums.txt\n"; exit 1; }
	@printf "  $(GREEN)[OK]$(RESET) $(BUILD_DIR)/checksums.txt\n"
	@binary="$(BUILD_DIR)/$(APP_NAME)-$$($(GOCMD) env GOOS)-$$($(GOCMD) env GOARCH)"; \
	version=$$($$binary -v); \
	if [ "$$version" != "$(VERSION)" ]; then \
		printf "$(RED)[ERROR]$(RESET) $$binary reports version '$$version', expected '$(VERSION)'\n"; exit 1; \
	fi; \
	printf "  $(GREEN)[OK]$(RESET) $$binary -v = $$version\n"

##@ Clean

# Clean targets
.PHONY: clean
clean: ## Clean build artifacts and coverage reports
	$(GOCLEAN)
	rm -rf $(BUILD_DIR) $(COVERAGE_FILE) $(COVERAGE_HTML)

# Default target
.PHONY: all
all: clean build ## Clean and build for all platforms
