SHELL := /bin/sh

VERSION ?=
DIR_OUT ?= _output
DIR_ROOT = $(realpath $(CURDIR))
DIR_OUT_ABS = $(abspath $(DIR_OUT))
DIR_RELEASE ?= $(DIR_OUT)/release
DIR_STG = $(DIR_OUT)/staging/$(VERSION)/$(OS)-$(ARCH)
OS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH ?= $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
BINARY = tfwrapper$(if $(filter windows,$(OS)),.exe)
ARCHIVE = tfwrapper-$(VERSION)-$(OS)-$(ARCH).zip
CTR_IMAGE_GO = ghcr.io/cloudboss/docker.io/library/golang:1.27.3-trixie
CTR_IMAGE_GOLANGCI = ghcr.io/cloudboss/golangci/golangci-lint:v2.15.0-alpine
USER_ID = $(shell id -u)
GROUP_ID = $(shell id -g)
SHA256 ?= $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo 'shasum -a 256')

CTR_RUN = docker run --rm \
	-u $(USER_ID):$(GROUP_ID) \
	-v "$(DIR_ROOT):$(DIR_ROOT):z" \
	-w "$(DIR_ROOT)" \
	-e GOPATH="$(DIR_OUT_ABS)/go" \
	-e GOCACHE="$(DIR_OUT_ABS)/gocache" \
	-e XDG_CACHE_HOME="$(DIR_OUT_ABS)/cache" \
	-e XDG_CONFIG_HOME="$(DIR_OUT_ABS)/config"

export VERSION OS ARCH

.DEFAULT_GOAL := test

.PHONY: build clean test lint check-docker check-version check-platform check-release \
	release release-one check-archive check-release-assets \
	release-linux-amd64 release-linux-arm64 release-darwin-arm64 release-windows-amd64

$(DIR_OUT):
	@mkdir -p "$(DIR_OUT)"

clean:
	@case "$(DIR_OUT_ABS)" in \
		"$(DIR_ROOT)"/*) ;; \
		*) echo 'DIR_OUT must be under the project root' >&2; exit 1 ;; \
	esac
	@if [ -e "$(DIR_OUT_ABS)" ] || [ -L "$(DIR_OUT_ABS)" ]; then \
		find "$(DIR_OUT_ABS)" -type d -exec chmod u+w {} + && \
		rm -rf -- "$(DIR_OUT_ABS)"; \
	fi

check-docker:
	@command -v docker >/dev/null 2>&1 || { echo 'docker is required' >&2; exit 1; }

build: check-docker check-platform | $(DIR_OUT)
	@$(CTR_RUN) -e CGO_ENABLED=0 -e GOOS="$${OS}" -e GOARCH="$${ARCH}" \
		$(CTR_IMAGE_GO) go build -trimpath -o "$(DIR_OUT)/$(BINARY)" .

test: check-docker | $(DIR_OUT)
	@$(CTR_RUN) -e CGO_ENABLED=1 -e TFWRAPPER_LIVE_TEST $(CTR_IMAGE_GO) sh -c \
		'go vet ./... && go test -race ./...'

lint: check-docker | $(DIR_OUT)
	@$(CTR_RUN) -e CGO_ENABLED=0 \
		-e GOLANGCI_LINT_CACHE="$(DIR_OUT_ABS)/cache/golangci-lint" \
		$(CTR_IMAGE_GOLANGCI) golangci-lint run --timeout 5m ./...

check-version:
	@number='(0|[1-9][0-9]*)'; \
	printf '%s\n' "$${VERSION}" | LC_ALL=C grep -Eq \
		"^v$${number}\.$${number}\.$${number}$$" || \
		{ echo 'VERSION must be vMAJOR.MINOR.PATCH without a suffix' >&2; exit 1; }

check-platform:
	@case $${OS}/$${ARCH} in \
		linux/amd64|linux/arm64|darwin/arm64|windows/amd64) ;; \
		*) echo "Unsupported release platform: $${OS}/$${ARCH}" >&2; exit 1 ;; \
	esac

check-release: check-version
	@grep -F "## [$${VERSION#v}] - " CHANGELOG.md | \
		LC_ALL=C grep -Eq '^## \[[^]]+\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$$' || \
		{ echo "CHANGELOG.md needs a dated entry for $${VERSION}" >&2; exit 1; }

release-one: check-version check-platform check-docker | $(DIR_OUT)
	@mkdir -p "$(DIR_STG)" "$(DIR_RELEASE)"
	@$(CTR_RUN) -e CGO_ENABLED=0 -e GOOS="$${OS}" -e GOARCH="$${ARCH}" \
		$(CTR_IMAGE_GO) go build \
		-trimpath -ldflags '-s -w' -o "$(DIR_STG)/$(BINARY)" .
	@cp README.md CHANGELOG.md "$(DIR_STG)/"
	@cd "$(DIR_STG)" && zip -q - "$(BINARY)" README.md CHANGELOG.md \
		> "$(abspath $(DIR_RELEASE))/$(ARCHIVE)"
	@cd "$(DIR_RELEASE)" && $(SHA256) "$(ARCHIVE)" > "$(ARCHIVE).sha256"

check-archive: check-version check-platform
	@cd "$(DIR_RELEASE)" && $(SHA256) -c "$(ARCHIVE).sha256"
	@entries=$$(unzip -Z1 "$(DIR_RELEASE)/$(ARCHIVE)") && \
	expected=$$(printf '%s\n' "$(BINARY)" README.md CHANGELOG.md | LC_ALL=C sort) && \
	[ "$$(printf '%s\n' "$${entries}" | LC_ALL=C sort)" = "$${expected}" ] || \
	{ echo "Unexpected contents in $(ARCHIVE)" >&2; exit 1; }

check-release-assets: check-version
	@for platform in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do \
		$(MAKE) --no-print-directory check-archive \
			OS="$${platform%/*}" ARCH="$${platform#*/}" \
			DIR_RELEASE="$(DIR_RELEASE)" || exit $${?}; \
	done

release-linux-amd64:
	@$(MAKE) release-one OS=linux ARCH=amd64

release-linux-arm64:
	@$(MAKE) release-one OS=linux ARCH=arm64

release-darwin-arm64:
	@$(MAKE) release-one OS=darwin ARCH=arm64

release-windows-amd64:
	@$(MAKE) release-one OS=windows ARCH=amd64

release: release-linux-amd64 release-linux-arm64 release-darwin-arm64 release-windows-amd64
