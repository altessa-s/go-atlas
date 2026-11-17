GOPATH         			:= ${HOME}/go
PATH           			:= ${GOPATH}/bin:$(PATH)
SHELL          			:= /bin/bash -o pipefail
.DELETE_ON_ERROR:
MAKEFLAGS      			+= --warn-undefined-variables
MAKEFLAGS      			+= --no-builtin-rules
CHANGELOG_NEXT_VERSION 	?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")

# MODULES lists the Go modules the targets iterate over: every directory with
# a go.mod, except devtools (tool pins) and tests/integration (run by
# test-integration against live backends).
MODULES ?= $(sort $(patsubst ./%,%,$(patsubst %/go.mod,%,$(shell find . -name go.mod -not -path './devtools/*' -not -path './tests/integration/*' -not -path '*/.*'))))
# FOR_MODULES runs the command that follows it in every module, stopping at the
# first failure; $$m is the module directory.
FOR_MODULES = set -e; for m in $(MODULES); do

# dupl configuration
# - DUPL_THRESHOLD: token threshold (higher -> fewer matches)
# - DUPL_PATH: scan root
# - DUPL_IGNORE_REGEX: paths to exclude from dupl output; we treat these as generated/noise.
# dlock/idempotency metric registrations are declarative noise: their instruments,
# help text and operation semantics differ; a shared bundle would couple unrelated APIs.
DUPL_THRESHOLD ?= 130
DUPL_PATH ?= .
DUPL_IGNORE_REGEX ?= (_gen\.go|_string\.go|\.pb(\.[^/]*)?\.go|_grpc\.pb\.go|\.pb\.gw\.go|_test\.go|tests/|factory/[^:]*/options\.go|config/.*acl\.go|coord/dlock/metrics\.go|reliability/idempotency/metrics\.go)

# gosec rule classes excluded globally because they are by-design or redundant
# for a transport/auth/secrets toolkit. Verified false positives in KEPT rules
# (G101, G103, G204, G404, ...) are suppressed individually via `// #nosec` so
# those rules still gate new code. Revisit the excludes below in a dedicated
# audit before relaxing further.
#   G104 - redundant with golangci-lint's errcheck (the project's error gate)
#   G115 - integer overflow on reviewed-safe numeric conversions (hash/proto/sizing)
#   G117 - exported config fields named like secrets (PrivateKey/SessionToken/JWT)
#   G304 - file reads from operator-provided paths (config/secrets/TLS)
#   G702/G703/G704/G705 - experimental taint analysis (cmd-inject/path/SSRF/XSS):
#          flags the toolkit's core function (outbound HTTP to configured endpoints,
#          HTTP response writing, config path reads); provenance is the caller's job
GOSEC_EXCLUDED_RULES ?= G104,G115,G117,G304,G702,G703,G704,G705

# Fuzzing configuration
# - FUZZTIME: per-target budget. `go test -fuzz` accepts exactly one target per
#   package per invocation, so `make fuzz` walks them and this is the cost of a
#   single target, not of the whole sweep.
# - FUZZPKG: restricts the sweep, e.g. FUZZPKG=./store/mongo
FUZZTIME ?= 30s
FUZZPKG ?= ./...

# Stress configuration (tests behind the `stress` build tag, see AGENTS.md)
# - STRESS_DURATION: run length of each stress test (ATLAS_STRESS_DURATION).
# - STRESS_TIMEOUT: go test -timeout for the whole run.
# - STRESS_PKG: packages to run; defaults to every package with a *_stress_test.go.
STRESS_DURATION ?= 30s
STRESS_TIMEOUT ?= 30m
STRESS_PKG ?= $(sort $(dir $(shell find . -name '*_stress_test.go' -not -path './tests/*' -not -path '*/.*')))

# Volume configuration (integration tests behind the `volume` build tag)
# - VOLUME_ROWS: rows seeded per backend (ATLAS_VOLUME_ROWS).
# - VOLUME_TIMEOUT: go test -timeout for the whole run.
# - VOLUME_RUN: -run filter, e.g. VOLUME_RUN=TestVolume_Outbox
VOLUME_ROWS ?= 1000000
VOLUME_TIMEOUT ?= 120m
VOLUME_RUN ?= ^TestVolume

.PHONY: all
all: help

.PHONY: fmt
fmt: tidy  ## Format and sort imports with the formatters of .golangci.yml
	@cd devtools && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint
	@$(FOR_MODULES) (cd $$m && golangci-lint fmt ./...); done

.PHONY: lint
lint: tidy fmt ## Run linter
	@cd devtools && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint
	@$(FOR_MODULES) (cd $$m && golangci-lint run ./...); done

.PHONY: tidy
tidy: ## Run go mod tidy in every module
	@$(FOR_MODULES) (cd $$m && go mod tidy); done

.PHONY: tidy-check
tidy-check: ## Fail if go mod tidy would change any module
	@$(FOR_MODULES) (cd $$m && go mod tidy -diff); done

.PHONY: modules
modules: ## List the modules the targets iterate over
	@printf '%s\n' $(MODULES)

.PHONY: proto
proto: ## Generate Go code from proto files using buf
	@cd proto && buf generate

.PHONY: generate
generate: ## Run go generate on all packages
	@$(FOR_MODULES) go generate -C $$m ./...; done

.PHONY: test
test: ## Run all tests
	@$(FOR_MODULES) go test -C $$m ./...; done 2>&1 | awk -F'\t' '\
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
	@$(FOR_MODULES) go test -C $$m -v ./...; done

.PHONY: test-race
test-race: ## Run all tests with the race detector (slower)
	@$(FOR_MODULES) go test -C $$m -race ./...; done

.PHONY: test-shuffle
test-shuffle: ## Run all tests with randomized order (helps catch order-dependence)
	@$(FOR_MODULES) go test -C $$m -shuffle=on ./...; done

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@$(FOR_MODULES) go test -C $$m -cover ./...; done

.PHONY: bench
bench: ## Run all benchmarks
	@$(FOR_MODULES) go test -C $$m -bench=. -benchmem -run=^$$ ./...; done 2>&1 | awk -F'\t' '\
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

.PHONY: bench-gate
bench-gate: ## Fail if hot-path benchmarks regressed against benchmarks/baseline.txt (same machine only)
	@./benchmarks/bench-gate.sh gate

.PHONY: bench-baseline
bench-baseline: ## Regenerate benchmarks/baseline.txt from the hot-path list on this machine
	@./benchmarks/bench-gate.sh baseline

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

.PHONY: fuzz-list
fuzz-list: ## List every fuzz target (package and name)
	@for pkg in $$(go list $(FUZZPKG)); do \
	  for t in $$(go test -list '^Fuzz' $$pkg 2>/dev/null | grep '^Fuzz' || true); do \
	    printf '%s\t%s\n' "$${pkg#github.com/altessa-s/go-atlas/}" "$$t"; \
	  done; \
	done

.PHONY: fuzz-seed
fuzz-seed: ## Run every fuzz target over its seed corpus only (no mutation, fast)
	@go test -run '^Fuzz' $(FUZZPKG)

.PHONY: fuzz
fuzz: ## Fuzz every target for FUZZTIME each (FUZZTIME=30s FUZZPKG=./...)
	@failed=""; \
	for pkg in $$(go list $(FUZZPKG)); do \
	  for t in $$(go test -list '^Fuzz' $$pkg 2>/dev/null | grep '^Fuzz' || true); do \
	    printf '\033[36m==> %s %s (%s)\033[0m\n' "$${pkg#github.com/altessa-s/go-atlas/}" "$$t" "$(FUZZTIME)"; \
	    go test -run '^$$' -fuzz "^$$t$$" -fuzztime $(FUZZTIME) $$pkg || failed="$$failed $${pkg#github.com/altessa-s/go-atlas/}:$$t"; \
	  done; \
	done; \
	if [ -n "$$failed" ]; then \
	  printf '\033[31mfailing fuzz targets:%s\033[0m\n' "$$failed"; \
	  printf 'minimized reproducers were written to the package testdata/fuzz directories; commit them as regression seeds\n'; \
	  exit 1; \
	fi

.PHONY: test-integration
test-integration: ## Run the integration suite against live backends (see tests/integration/README.md)
	@go test -C tests/integration -race -count=1 -p 1 ./...

.PHONY: test-volume
test-volume: ## Run the volume tests (VOLUME_ROWS rows per backend) against the integration backends
	@ATLAS_VOLUME_ROWS=$(VOLUME_ROWS) go test -C tests/integration -tags volume -count=1 -p 1 -v \
	    -timeout $(VOLUME_TIMEOUT) -run '$(VOLUME_RUN)' ./...

.PHONY: test-stress
test-stress: ## Run the stress tests under -race, STRESS_DURATION each (needs NATS from integration-up for dlock)
	@ATLAS_STRESS_DURATION=$(STRESS_DURATION) go test -tags stress -race -count=1 -p 1 -timeout $(STRESS_TIMEOUT) \
	    -run '^(TestStress|TestWALCrashRecovery$$)' $(STRESS_PKG)

.PHONY: integration-up
integration-up: ## Start the backends the integration suite runs against
	@docker compose -f tests/integration/docker-compose.yml up -d --wait

.PHONY: integration-down
integration-down: ## Stop the integration backends and drop their volumes
	@docker compose -f tests/integration/docker-compose.yml down -v

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
	@cd devtools && go install github.com/altessa-s/go-copyright-checker
	go-copyright-checker check --fix .

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

.PHONY: gosec
gosec: ## Run gosec static security scanner (see GOSEC_EXCLUDED_RULES)
	@cd devtools && go install github.com/securego/gosec/v2/cmd/gosec
	@gosec -quiet -exclude-generated -exclude=$(GOSEC_EXCLUDED_RULES) ./...

.PHONY: security-scan
security-scan: ## Security checks - run vulnerability and security scanners
	@cd devtools && go install golang.org/x/vuln/cmd/govulncheck
	@cd devtools && go install github.com/securego/gosec/v2/cmd/gosec
	@# Run every scanner even when an earlier one reports findings, then fail
	@# if any did, so one report never hides the other.
	@status=0; \
	echo "Running Go vulnerability check ($$(go env GOVERSION))..."; \
	govulncheck ./... || status=1; \
	echo ""; \
	echo "Running gosec security scanner..."; \
	gosec -quiet -exclude-generated -exclude=$(GOSEC_EXCLUDED_RULES) ./... || status=1; \
	echo ""; \
	if [ $$status -ne 0 ]; then echo "Security scan found issues"; exit 1; fi; \
	echo "Security scan completed"

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: check-architecture
check-architecture: ## Check production package boundaries on every platform
	go test ./tests/architecture
