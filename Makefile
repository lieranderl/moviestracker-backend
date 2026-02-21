SHELL := /bin/sh

MODULES := \
	SERVICES/GetImdbRating \
	SERVICES/GetTorrentsForMovie \
	JOBS/GetMovies \
	JOBS/DownloadImdbData

COVER_DIR := .coverage
CACHE_DIR := .cache
GOCACHE ?= $(CURDIR)/$(CACHE_DIR)/go-build
GOMODCACHE ?= $(CURDIR)/$(CACHE_DIR)/go-mod
GOENV := GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)
GOLANGCI_CONFIG := $(CURDIR)/.golangci.yml
PROJECT_ID ?= moviestracker-f07e2
REGION ?= europe-west1
REPO ?= gcf-artifacts
IMAGE_TAG ?= $(shell sh -c 'git rev-parse --short HEAD 2>/dev/null || echo local')-$(shell date +%Y%m%d%H%M)
AR_BASE := $(REGION)-docker.pkg.dev/$(PROJECT_ID)/$(REPO)

.PHONY: fmt test race cover build vet lint staticcheck gosec tidy push-images print-images precommit-install precommit-run clean

fmt:
	@echo "==> gofmt"
	@find . -name '*.go' -not -path './.git/*' -print0 | xargs -0 gofmt -w

tidy:
	@set -e; \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	for mod in $(MODULES); do \
		echo "==> go mod tidy $$mod"; \
		(cd $$mod && $(GOENV) go mod tidy); \
	done

test:
	@set -e; \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	for mod in $(MODULES); do \
		echo "==> go test $$mod"; \
		(cd $$mod && $(GOENV) go test ./...); \
	done

race:
	@set -e; \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	for mod in $(MODULES); do \
		echo "==> go test -race $$mod"; \
		(cd $$mod && $(GOENV) go test -race ./...); \
	done

cover:
	@set -e; \
	mkdir -p $(COVER_DIR); \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	for mod in $(MODULES); do \
		name=$$(echo $$mod | tr '/' '-'); \
		outfile="$(CURDIR)/$(COVER_DIR)/$$name.out"; \
		echo "==> go test -coverprofile $$mod"; \
		(cd $$mod && $(GOENV) go test -covermode=atomic -coverprofile="$$outfile" ./...); \
		(cd $$mod && go tool cover -func="$$outfile" | tail -n 1); \
	done

build:
	@set -e; \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	for mod in $(MODULES); do \
		echo "==> go build $$mod"; \
		name=$$(echo $$mod | tr '/' '-'); \
		(cd $$mod && $(GOENV) go build -o "/tmp/$$name" ./cmd/main.go); \
	done

vet:
	@set -e; \
	mkdir -p "$(GOCACHE)" "$(GOMODCACHE)"; \
	for mod in $(MODULES); do \
		echo "==> go vet $$mod"; \
		(cd $$mod && $(GOENV) go vet ./...); \
	done

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint is not installed"; exit 1; }
	@set -e; \
	for mod in $(MODULES); do \
		echo "==> golangci-lint $$mod"; \
		(cd $$mod && golangci-lint run --config "$(GOLANGCI_CONFIG)" ./...); \
	done

staticcheck:
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck is not installed"; exit 1; }
	@set -e; \
	for mod in $(MODULES); do \
		echo "==> staticcheck $$mod"; \
		(cd $$mod && staticcheck ./...); \
	done

gosec:
	@command -v gosec >/dev/null 2>&1 || { echo "gosec is not installed"; exit 1; }
	@set -e; \
	for mod in $(MODULES); do \
		echo "==> gosec $$mod"; \
		(cd $$mod && gosec ./...); \
	done

push-images:
	@command -v gcloud >/dev/null 2>&1 || { echo "gcloud is not installed"; exit 1; }
	@set -e; \
	echo "==> pushing images to $(AR_BASE) with tag $(IMAGE_TAG)"; \
	gcloud builds submit SERVICES/GetTorrentsForMovie \
		--tag "$(AR_BASE)/get-torrents-for-movie-service:$(IMAGE_TAG)" \
		--region "$(REGION)"; \
	gcloud builds submit SERVICES/GetImdbRating \
		--tag "$(AR_BASE)/get-imdb-rating-service:$(IMAGE_TAG)" \
		--region "$(REGION)"; \
	gcloud builds submit JOBS/GetMovies \
		--tag "$(AR_BASE)/getmovies-image:$(IMAGE_TAG)" \
		--region "$(REGION)"; \
	gcloud builds submit JOBS/DownloadImdbData \
		--tag "$(AR_BASE)/downloadimdbdata-image:$(IMAGE_TAG)" \
		--region "$(REGION)"; \
	echo "==> done"; \
	echo "Tags pushed:"; \
	echo "  $(AR_BASE)/get-torrents-for-movie-service:$(IMAGE_TAG)"; \
	echo "  $(AR_BASE)/get-imdb-rating-service:$(IMAGE_TAG)"; \
	echo "  $(AR_BASE)/getmovies-image:$(IMAGE_TAG)"; \
	echo "  $(AR_BASE)/downloadimdbdata-image:$(IMAGE_TAG)"

print-images:
	@echo "$(AR_BASE)/get-torrents-for-movie-service:$(IMAGE_TAG)"
	@echo "$(AR_BASE)/get-imdb-rating-service:$(IMAGE_TAG)"
	@echo "$(AR_BASE)/getmovies-image:$(IMAGE_TAG)"
	@echo "$(AR_BASE)/downloadimdbdata-image:$(IMAGE_TAG)"

precommit-install:
	@command -v pre-commit >/dev/null 2>&1 || { echo "pre-commit is not installed"; exit 1; }
	pre-commit install

precommit-run:
	@command -v pre-commit >/dev/null 2>&1 || { echo "pre-commit is not installed"; exit 1; }
	pre-commit run --all-files

clean:
	@rm -rf $(COVER_DIR) $(CACHE_DIR)
