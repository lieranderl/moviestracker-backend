SHELL := /bin/sh

APPS := getmovies getimdbrating downloadimdbdata
CACHE_DIR := $(CURDIR)/.cache
COVER_DIR := $(CURDIR)/.coverage
GOCACHE ?= $(CACHE_DIR)/go-build
GOMODCACHE ?= $(CACHE_DIR)/go-mod
GOENV := GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)
GOLANGCI_LINT_CACHE ?= $(CACHE_DIR)/golangci-lint
COVERAGE_THRESHOLD ?= 65

.PHONY: fmt tidy test race cover build vet lint vuln workflows verify clean

fmt:
	@gofmt -w cmd internal

tidy:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	@$(GOENV) go mod tidy

test:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	@$(GOENV) go test ./...

race:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	@$(GOENV) go test -race ./...

cover:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)" "$(COVER_DIR)"
	@$(GOENV) go test ./... -covermode=atomic -coverprofile="$(COVER_DIR)/coverage.out"
	@total=`$(GOENV) go tool cover -func="$(COVER_DIR)/coverage.out" | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}'`; \
	awk -v total="$$total" -v minimum="$(COVERAGE_THRESHOLD)" 'BEGIN { printf "coverage: %.1f%% (minimum %.1f%%)\n", total, minimum; exit(total < minimum) }'

build:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)" "$(CURDIR)/bin"
	@set -e; for app in $(APPS); do $(GOENV) CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags='-s -w -buildid=' -o "$(CURDIR)/bin/$$app" "./cmd/$$app"; done

vet:
	@mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"
	@$(GOENV) go vet ./...

lint:
	@mkdir -p "$(GOLANGCI_LINT_CACHE)"
	@GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) $(GOENV) golangci-lint run --config .golangci.yml ./...

vuln:
	@$(GOENV) govulncheck ./...

workflows:
	@$(GOENV) go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
	@pipx run zizmor==1.31.1 --offline --min-severity medium .github/workflows
	@python3 scripts/test_release_workflow.py

verify: test race cover vet lint vuln workflows build

clean:
	@rm -rf "$(COVER_DIR)" "$(CACHE_DIR)" "$(CURDIR)/bin"
