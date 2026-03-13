GOPATH         			:= ${HOME}/go
PATH           			:= ${GOPATH}/bin:$(PATH)
SHELL          			:= /bin/bash
.DELETE_ON_ERROR:
MAKEFLAGS      			+= --warn-undefined-variables
MAKEFLAGS      			+= --no-builtin-rules
CHANGELOG_NEXT_VERSION 	?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")

# dupl configuration
# - DUPL_THRESHOLD: token threshold (higher -> fewer matches)
# - DUPL_PATH: scan root
# - DUPL_IGNORE_REGEX: paths to exclude from dupl output; we treat these as generated/noise.
DUPL_THRESHOLD ?= 60
DUPL_PATH ?= .
DUPL_IGNORE_REGEX ?= (_gen\.go|\.pb(\.[^/]*)?\.go|_grpc\.pb\.go|\.pb\.gw\.go)

.PHONY: all
all: help

.PHONY: fmt
fmt: tidy  ## Run go fmt on all go files
	@cd devtools && go install github.com/daixiang0/gci
	@gci write \
	    -s standard \
	    -s default \
	    -s "prefix(google.golang.org)" \
	    -s "prefix(golang.org)" \
	    -s "prefix(github.com/altessa-s/go-atlas)" \
		-s "prefix(github.com/altessa-s)" \
	    -s blank -s alias \
	 $$(go list -f {{.Dir}} ./...)

.PHONY: lint
lint: tidy fmt ## Run linter
	@cd devtools && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint
	golangci-lint run ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	@go mod tidy

.PHONY: generate
generate: ## Run go generate on all packages
	@go generate ./...

.PHONY: test
test: ## Run all tests
	@go test ./... 2>&1 | awk -F'\t' '\
	  function pad(s,w, _l,_p) { gsub(/^[ \t]+|[ \t]+$$/, "", s); _l=length(s); _p=""; while(_l++<w) _p=_p" "; return s _p } \
	  function color(s,c) { return "\033[" c "m" s "\033[0m" } \
	  { \
	    st=$$1; gsub(/^[ \t]+|[ \t]+$$/, "", st); \
	    pkg=$$2; gsub(/^[ \t]+|[ \t]+$$/, "", pkg); sub(/github.com\/altessa-s\/go-atlas\//, "", pkg); \
	    if (st=="ok") cs="32"; else if (st=="FAIL") cs="31"; else cs="33"; \
	    printf "| %s | %s | %s |\n", color(pad(st,4),cs), pad(pkg,56), pad($$3,15) \
	  }'

.PHONY: test-verbose
test-verbose: ## Run all tests with verbose output
	@go test -v ./...

.PHONY: test-race
test-race: ## Run all tests with the race detector (slower)
	@go test -race ./...

.PHONY: test-shuffle
test-shuffle: ## Run all tests with randomized order (helps catch order-dependence)
	@go test -shuffle=on ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@go test -cover ./...


.PHONY: bench
bench: ## Run all benchmarks
	@go test -bench=. -benchmem -run=^$$ ./... 2>&1 | awk -F'\t' '\
	  function pad(s,w, _l,_p) { gsub(/^[ \t]+|[ \t]+$$/, "", s); _l=length(s); _p=""; while(_l++<w) _p=_p" "; return s _p } \
	  function color(s,c) { return "\033[" c "m" s "\033[0m" } \
	  /^(ok|FAIL|\?)/ { \
	    st=$$1; gsub(/^[ \t]+|[ \t]+$$/, "", st); \
	    pkg=$$2; gsub(/^[ \t]+|[ \t]+$$/, "", pkg); sub(/github.com\/altessa-s\/go-atlas\//, "", pkg); \
	    if (st=="ok") cs="32"; else if (st=="FAIL") cs="31"; else cs="33"; \
	    printf "| %s | %s | %s |\n", color(pad(st,4),cs), pad(pkg,56), pad($$3,15); next \
	  } \
	  { print }'

.PHONY: bench-count
bench-count: ## Run benchmarks multiple times for benchstat (COUNT=5)
	@go test -bench=. -benchmem -run=^$$ -count=$${COUNT:-5} ./... | tee bench.txt
	@echo "Results saved to bench.txt (use benchstat bench.txt)"

.PHONY: test-coverage-func
test-coverage-func: ## Run tests and show formatted per-package coverage
	@go test -cover ./... 2>&1 | awk -F'\t' '\
	  function pad(s,w, _l,_p) { gsub(/^[ \t]+|[ \t]+$$/, "", s); _l=length(s); _p=""; while(_l++<w) _p=_p" "; return s _p } \
	  function color(s,c) { return "\033[" c "m" s "\033[0m" } \
	  { \
	    st=$$1; gsub(/^[ \t]+|[ \t]+$$/, "", st); \
	    pkg=$$2; gsub(/^[ \t]+|[ \t]+$$/, "", pkg); sub(/github.com\/altessa-s\/go-atlas\//, "", pkg); \
	    if (st=="ok") cs="32"; else if (st=="FAIL") cs="31"; else cs="33"; \
	    cov=$$4; gsub(/^[ \t]+|[ \t]+$$/, "", cov); \
	    match(cov, /[0-9]+\.[0-9]+/); pct=substr(cov,RSTART,RLENGTH)+0; \
	    if (cov=="") cc="0"; else if (pct>=80) cc="32"; else if (pct>=50) cc="33"; else cc="31"; \
	    printf "| %s | %s | %s | %s |\n", color(pad(st,4),cs), pad(pkg,56), pad($$3,15), color(pad(cov,30),cc) \
	  }'

.PHONY: test-all
test-all: ## Run tests with race detector, shuffle, and coverage
	@go test -race -shuffle=on -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1
	@rm -f coverage.out

.PHONY: dupl
dupl: ## Run dupl (ignoring generated files)
	@cd devtools && go install github.com/mibk/dupl
	@dupl -t $(DUPL_THRESHOLD) -plumbing $(DUPL_PATH) | grep -Ev "$(DUPL_IGNORE_REGEX)" || true

.PHONY: dupl-check
dupl-check: ## Fail if dupl finds duplicates (ignoring generated files)
	@cd devtools && go install github.com/mibk/dupl
	@out="$$(dupl -t $(DUPL_THRESHOLD) -plumbing $(DUPL_PATH) | grep -Ev "$(DUPL_IGNORE_REGEX)" || true)"; \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		exit 1; \
	fi

.PHONY: ci
ci: lint test-all security-scan dupl-check ## Run full CI checks locally (lint, test, security, dupl)

.PHONY: clean
clean: ## Remove generated artifacts (coverage.out, bench.txt)
	@rm -f coverage.out bench.txt

.PHONY: copyright
copyright: ## Add copyright header to all files
	@echo "Updating copyright header to all go files"
	go-copyright-checker check --fix .

.PHONY: changelog
changelog: ## Update changelog
	@echo "Updating CHANGELOG file"
	@cd devtools && go install github.com/git-chglog/git-chglog/cmd/git-chglog
	git-chglog --next-tag=$(CHANGELOG_NEXT_VERSION) -o CHANGELOG.md

.PHONY: precommit-install
precommit-install: ## Install pre-commit hooks
	@pip3 install pre-commit
	@pre-commit install

.PHONY: precommit-run
precommit-run: ## Run pre-commit hooks
	@pre-commit run --all-files

.PHONY: release
release: ## Semantic versioning - create and push a new release tag
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make release VERSION=v1.2.3"; \
		echo "Current version: $$(git describe --tags --abbrev=0 2>/dev/null || echo 'no tags')"; \
		echo "Next version suggestion: $(CHANGELOG_NEXT_VERSION)"; \
		exit 1; \
	fi
	@echo "Creating release $(VERSION)..."
	@git tag -a $(VERSION) -m "Release $(VERSION)"
	@git push origin $(VERSION)
	@echo "Release $(VERSION) created and pushed"

.PHONY: security-scan
security-scan: ## Security checks - run vulnerability and security scanners
	@echo "Running Go vulnerability check..."
	@cd devtools && go install golang.org/x/vuln/cmd/govulncheck
	@govulncheck ./...
	@echo ""
	@echo "Running gosec security scanner..."
	@cd devtools && go install github.com/securego/gosec/v2/cmd/gosec
	@gosec -quiet -exclude-generated ./...
	@echo ""
	@echo "Security scan completed"

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
