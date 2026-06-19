# Based on ~/Code/github.com/grafana/loki/Makefile
# TODO: Switch docker to ??? for CI

SHELL = /usr/bin/env bash -o pipefail

.DEFAULT_GOAL := all

CI ?= false

# Ensure you run `make release-workflows` after changing this
GO_VERSION         := 1.26.3

IMAGE_TAG          ?= $(shell ./tools/image-tag)
GIT_REVISION       := $(shell git rev-parse --short HEAD)
GIT_BRANCH         := $(shell git rev-parse --abbrev-ref HEAD)

# Golang environment
GOOS               ?= $(shell go env GOOS)
GOHOSTOS           ?= $(shell go env GOHOSTOS)
GOARCH             ?= $(shell go env GOARCH)
GOARM              ?= $(shell go env GOARM)
GOEXPERIMENT       ?= $(shell go env GOEXPERIMENT)

GOTEST             ?= go test

# Build flags
VPREFIX            := github.com/zsrv/goscape/pkg/util/build
GO_LDFLAGS         := -X $(VPREFIX).Branch=$(GIT_BRANCH) \
                      -X $(VPREFIX).Version=$(IMAGE_TAG) \
                      -X $(VPREFIX).Revision=$(GIT_REVISION) \
                      -X $(VPREFIX).BuildUser=$(shell whoami)@$(shell hostname) \
                      -X $(VPREFIX).BuildDate=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

GO_FLAGS := -trimpath -ldflags "-s -w $(GO_LDFLAGS)"

# Per some websites I've seen to add `-gcflags "all=-N -l"`, the gcflags seem poorly if at all documented
# the best I could dig up is -N disables optimizations and -l disables inlining which should make debugging match source better.
# Also remove the -s and -w flags present in the normal build which strip the symbol table and the DWARF symbol table.
DEBUG_GO_FLAGS := -gcflags "all=-N -l" -trimpath -ldflags "$(GO_LDFLAGS)"

# Cache packing (baked into the goscape image)
CACHE_SRC_DIR ?= data/src
CACHE_RAW_DIR ?= data/raw
CACHE_OUT_DIR ?= data/pack

# Image names
IMAGE_PREFIX      ?= goscape
GOSCAPE_IMAGE     := $(IMAGE_PREFIX)/goscape:$(IMAGE_TAG)
GOSCAPE_CLI_IMAGE := $(IMAGE_PREFIX)/goscape-cli:$(IMAGE_TAG)

# OCI (Docker) setup
# TODO: Adjust for whichever CI builder we select
OCI_PLATFORMS  := --platform=linux/amd64,linux/arm64
OCI_BUILD_ARGS := --build-arg GO_VERSION=$(GO_VERSION) --build-arg IMAGE_TAG=$(IMAGE_TAG)
#OCI_PUSH       := docker push
OCI_PUSH       := podman push
#OCI_TAG        := docker tag

ifeq ($(CI),true)
	# TODO: change this to whichever CI builder we end up using
	# ensure buildx is set up
	_               := $(shell ./tools/ensure-buildx-builder.sh)
	OCI_BUILD       := DOCKER_BUILDKIT=1 docker buildx build $(OCI_PLATFORMS) $(OCI_BUILD_ARGS)
else
#	OCI_BUILD       := DOCKER_BUILDKIT=1 docker build $(OCI_BUILD_ARGS)
	OCI_BUILD       := podman build $(OCI_BUILD_ARGS)
endif

# Adapted from https://www.thapaliya.com/en/writings/well-documented-makefiles/
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-45s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: all images check-generated-files goscape goscape-debug goscape-cli pack lint test clean protos proto
.PHONY: format check-format
.PHONY: goscape-image goscape-cli-image build-image build-image-push
.PHONY: benchmark-store check-mod
.PHONY: migrate migrate-image lint-markdown
.PHONY: doc check-doc
.PHONY: validate-example-configs generate-example-config-doc check-example-config-doc
.PHONY: clean clean-protos
.PHONY: dev-k3d-goscape dev-k3d-down
.PHONY: helm-test helm-lint

#############
# Variables #
#############

# We don't want find to scan inside a bunch of directories, to accelerate the
# 'make: Entering directory '/src/loki' phase.
DONT_FIND := -name tools -prune -o -name vendor -prune -o -name operator -prune -o -name .git -prune -o -name .cache -prune -o -name .pkg -prune -o

# Protobuf files
PROTO_DEFS := $(shell find . $(DONT_FIND) -type f -name '*.proto' -print)

# Documentation source path
DOC_SOURCES_PATH := docs/sources
DOC_TEMPLATE_PATH := docs/templates

# Configuration flags documentation
DOC_FLAGS_TEMPLATE := $(DOC_TEMPLATE_PATH)/configuration.template
DOC_FLAGS := $(DOC_SOURCES_PATH)/shared/configuration.md

################
# Main Targets #
################

all: goscape goscape-cli ## build all executables (goscape, goscape-cli)

# This is really a check for the CI to make sure generated files are built and checked in manually
check-generated-files: fmt-proto protos
	@if ! (git diff --exit-code -- $(PROTO_DEFS) $$(find . $(DONT_FIND) -type f -name '*.pb.go' -print)); then \
		echo "\nChanges found in generated files"; \
		echo "Run 'make check-generated-files' and commit the changes to fix this error."; \
		echo "If you are actively developing these files you can ignore this error"; \
		echo "(Don't forget to check in the generated files when finished)\n"; \
		exit 1; \
	fi

###########
# goscape #
###########

.PHONY: cmd/goscape/goscape cmd/goscape/goscape-debug
goscape: cmd/goscape/goscape ## build goscape executable
goscape-debug: cmd/goscape/goscape-debug ## build goscape debug executable

cmd/goscape/goscape:
	CGO_ENABLED=0 go build $(GO_FLAGS) -o $@ ./$(@D)

cmd/goscape/goscape-debug:
	CGO_ENABLED=0 go build $(DEBUG_GO_FLAGS) -o $@ ./$(@D)

###############
# goscape-cli #
###############

.PHONY: cmd/goscape-cli/goscape-cli
goscape-cli: cmd/goscape-cli/goscape-cli ## build goscape-cli executable
goscape-cli-debug: cmd/goscape-cli/goscape-cli-debug ## build debug goscape-cli executable

cmd/goscape-cli/goscape-cli:
	CGO_ENABLED=0 go build $(GO_FLAGS) -o $@ ./cmd/goscape-cli

cmd/goscape-cli/goscape-cli-debug:
	CGO_ENABLED=0 go build $(DEBUG_GO_FLAGS) -o ./cmd/goscape-cli/goscape-cli-debug ./cmd/goscape-cli

.PHONY: pack
pack: goscape-cli ## pack the game cache from $(CACHE_SRC_DIR) into $(CACHE_OUT_DIR)
	CGO_ENABLED=0 ./cmd/goscape-cli/goscape-cli pack \
		--src-dir $(CACHE_SRC_DIR) \
		--raw-dir $(CACHE_RAW_DIR) \
		--out-dir $(CACHE_OUT_DIR)

########
# Helm #
########

.PHONY: production/helm/goscape/src/helm-test/helm-test
helm-test: production/helm/goscape/src/helm-test/helm-test ## run helm tests

# Package Helm tests but do not run them.
production/helm/goscape/src/helm-test/helm-test:
	CGO_ENABLED=0 go test $(GO_FLAGS) --tags=helm_test -c -o $@ ./$(@D)

helm-lint: ## run helm linter
	$(MAKE) -BC production/helm/goscape lint

helm-docs: ## generate reference documentation
	$(MAKE) -BC docs sources/setup/install/helm/reference.md

#########
# Mixin #
#########

MIXIN_PATH := production/goscape-mixin
MIXIN_OUT_PATH := production/goscape-mixin-compiled
MIXIN_OUT_PATH_SSD := production/goscape-mixin-compiled-ssd

goscape-mixin: ## compile the goscape mixin
	@rm -rf $(MIXIN_OUT_PATH) && mkdir $(MIXIN_OUT_PATH)
	@cd $(MIXIN_PATH) && jb install
	@mixtool generate all --output-alerts $(MIXIN_OUT_PATH)/alerts.yaml --output-rules $(MIXIN_OUT_PATH)/rules.yaml --directory $(MIXIN_OUT_PATH)/dashboards ${MIXIN_PATH}/mixin.libsonnet

	@rm -rf $(MIXIN_OUT_PATH_SSD) && mkdir $(MIXIN_OUT_PATH_SSD)
	@cd $(MIXIN_PATH) && jb install
	@mixtool generate all --output-alerts $(MIXIN_OUT_PATH_SSD)/alerts.yaml --output-rules $(MIXIN_OUT_PATH_SSD)/rules.yaml --directory $(MIXIN_OUT_PATH_SSD)/dashboards ${MIXIN_PATH}/mixin-ssd.libsonnet

goscape-mixin-check: goscape-mixin ## check the goscape mixin is up to date
	@echo "Checking diff"
	@git diff --exit-code -- $(MIXIN_OUT_PATH) || (echo "Please build mixin by running 'make goscape-mixin'" && false)
	@git diff --exit-code -- $(MIXIN_OUT_PATH_SSD) || (echo "Please build mixin by running 'make goscape-mixin'" && false)

#############
# Releasing #
#############

GOX = gox $(GO_FLAGS) -output="dist/{{.Dir}}-{{.OS}}-{{.Arch}}"

SKIP_ARM ?= false
dist: clean
ifeq ($(SKIP_ARM),true)
	CGO_ENABLED=0 $(GOX) -osarch="linux/amd64 darwin/amd64 windows/amd64 freebsd/amd64" ./cmd/goscape
	CGO_ENABLED=0 $(GOX) -osarch="linux/amd64 darwin/amd64 windows/amd64 freebsd/amd64" ./cmd/goscape-cli
else
	CGO_ENABLED=0 $(GOX) -osarch="linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64 freebsd/amd64" ./cmd/goscape
	CGO_ENABLED=0 $(GOX) -osarch="linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64 freebsd/amd64" ./cmd/goscape-cli
endif
	for i in dist/*; do zip -j -m $$i.zip $$i; done
	pushd dist && sha256sum * > SHA256SUMS && popd

packages: dist
	@tools/packaging/nfpm.sh

publish: packages
	./tools/release

# Included file contains dynamically created make targets for cross-compiling
include crosscompile.mk

########
# Lint #
########

ifeq ($(UNAME_S),Linux)
LINT_FLAGS=--timeout=15m --build-tags=linux
GOFLAGS=-tags=linux
else
LINT_FLAGS=--timeout=15m
GOFLAGS=""
endif
lint: ## run linters
	go version
	golangci-lint version
	golangci-lint run -v $(LINT_FLAGS)
	GOFLAGS=$(GOFLAGS) faillint -paths \
		"sync/atomic=go.uber.org/atomic" \
		./...

	# Use our spanlogger implementation instead of the one in dskit to make sure we use the correct tracing lib.
	faillint -paths \
		"github.com/grafana/dskit/spanlogger=github.com/grafana/loki/pkg/util/spanlogger" \
		./...

########
# Test #
########

test: all ## run the unit tests
	go test $(GO_FLAGS) -covermode=atomic -coverprofile=coverage.txt -p=4 ./... | tee test_results.txt

test-integration:
	$(GOTEST) -count=1 -v -tags=integration -timeout 15m ./integration

compare-coverage:
	./tools/diff_coverage.sh $(old) $(new) $(packages)

#########
# Clean #
#########

clean-protos:
	find . $(DONT_FIND) -type f -name '*.pb.go' -print0 | xargs -0 -r rm -f

clean: ## clean the generated files
	#rm -rf .cache
	rm -rf cmd/goscape/goscape
	rm -rf cmd/goscape-cli/goscape-cli
	rm -rf dist/
	go clean ./...

#############
# Protobufs #
#############

protos: clean-protos
	buf generate

##########
# Images #
##########

images: goscape-image goscape-cli-image #helm-test-image

# goscape image
goscape-image: ## build the goscape docker image
	$(OCI_BUILD) -t $(GOSCAPE_IMAGE) -f cmd/goscape/Dockerfile .
goscape-debug-image: ## build the goscape debug docker image
	$(OCI_BUILD) -t $(GOSCAPE_IMAGE)-debug -f cmd/goscape/Dockerfile.debug .

# goscape local image
# Default architecture for local builds
LOCAL_ARCH ?= linux/amd64
goscape-local-image: ## build the goscape docker image locally (set LOCAL_ARCH=linux/arm64 for arm64)
	docker buildx build --load --platform=$(LOCAL_ARCH) -t $(GOSCAPE_IMAGE) -f cmd/goscape/Dockerfile .

# goscape-cli image
goscape-cli-image: ## build the goscape-cli docker image
	$(OCI_BUILD) -t $(GOSCAPE_CLI_IMAGE) -f cmd/goscape-cli/Dockerfile .

# Helm test image
helm-test-image: ## build the helm test docker image
	$(OCI_BUILD) -t $(IMAGE_PREFIX)/goscape-helm-test:$(IMAGE_TAG) -f production/helm/goscape/src/helm-test/Dockerfile .
helm-test-push: helm-test-image
	$(OCI_PUSH) $(IMAGE_PREFIX)/goscape-helm-test:$(IMAGE_TAG)

#################
# Documentation #
#################

documentation-helm-reference-check:
	@echo "Checking diff"
	$(MAKE) -BC docs sources/setup/install/helm/reference.md
	@git diff --exit-code -- docs/sources/setup/install/helm/reference.md || (echo "Please generate Helm Chart reference by running 'make -C docs sources/setup/install/helm/reference.md'" && false)

########
# Misc #
########

# Targets can depend on ALWAYS_BUILD to run regardless of whether the target is
# up-to-date or not because PHONY targets are always rebuilt.
.PHONY: ALWAYS_BUILD
ALWAYS_BUILD:

benchmark-store:
	go run ./pkg/storage/hack/main.go
	$(GOTEST) ./pkg/storage/ -bench=.  -benchmem -memprofile memprofile.out -cpuprofile cpuprofile.out -trace trace.out

# support go modules
check-mod:
	GO111MODULE=on GOPROXY=https://proxy.golang.org go mod download
	GO111MODULE=on GOPROXY=https://proxy.golang.org go mod verify
	GO111MODULE=on GOPROXY=https://proxy.golang.org go mod tidy
	GO111MODULE=on GOPROXY=https://proxy.golang.org go mod vendor
	@git diff --exit-code -- go.sum go.mod vendor/ || \
	    (echo "Run 'go mod download && go mod verify && go mod tidy && go mod vendor' and check in changes to vendor/ to fix failed check-mod."; exit 1)

lint-jsonnet:
	@RESULT=0; \
	for f in $$(find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print); do \
		jsonnetfmt -- "$$f" | diff -u "$$f" -; \
		RESULT=$$(($$RESULT + $$?)); \
	done; \
	for d in $$(find . -name '*-mixin' -a -type d -print); do \
		if [ -e "$$d/jsonnetfile.json" ]; then \
			echo "Installing dependencies for $$d"; \
			pushd "$$d" >/dev/null && jb install && popd >/dev/null; \
		fi; \
	done; \
	for m in $$(find . -name 'mixin.libsonnet' -not -path '*/vendor/*' -print); do \
			echo "Linting $$m"; \
			mixtool lint -J $$(dirname "$$m")/vendor "$$m"; \
			if [ $$? -ne 0 ]; then \
				RESULT=1; \
			fi; \
	done; \
	exit $$RESULT

fmt-jsonnet:
	@find . -name 'vendor' -prune -o -name '*.libsonnet' -print -o -name '*.jsonnet' -print | \
		xargs -n 1 -- jsonnetfmt -i


fmt-proto:
	echo '$(PROTO_DEFS)' | \
		xargs -n 1 -- buf format -w


lint-scripts:
    # Ignore https://github.com/koalaman/shellcheck/wiki/SC2312
	@find . -name '*.sh' -not -path '*/vendor/*' -print0 | \
		xargs -0 -n1 shellcheck -e SC2312 -x -o all


# search for dead link in our documentation.
# To avoid being rate limited by Github you can use an env variable GITHUB_TOKEN to pass a github token API.
# see https://github.com/settings/tokens
lint-markdown:
	lychee --verbose --config .lychee.toml ./*.md  ./docs/**/*.md  ./production/**/*.md ./cmd/**/*.md ./clients/**/*.md ./tools/**/*.md


format:
	find . $(DONT_FIND) -name '*.pb.go' -prune -o \
		-type f -name '*.go' -exec gofmt -w -s {} \;
	find . $(DONT_FIND) -name '*.pb.go' -prune -o \
		-type f -name '*.go' -exec goimports -w -local github.com/zsrv/goscape {} \;


# main is the codeless docs hub; diff against the revision branch.
GIT_TARGET_BRANCH ?= rev-225
check-format: format
	git diff --name-only HEAD origin/$(GIT_TARGET_BRANCH) -- "*.go" | xargs --no-run-if-empty git diff --exit-code -- \
	|| (echo "Please format code by running 'make format' and committing the changes" && false)

# Documentation related commands

doc: ## Generates the config file documentation
	go run $(GO_FLAGS) ./tools/doc-generator $(DOC_FLAGS_TEMPLATE) > $(DOC_FLAGS)

docs: doc

check-doc: ## Check the documentation files are up to date
check-doc: doc
	@find . -name "*.md" | xargs git diff --exit-code -- \
	|| (echo "Please update generated documentation by running 'make doc' and committing the changes" && false)

###################
# Example Configs #
###################
EXAMPLES_DOC_PATH := $(DOC_SOURCES_PATH)/configure/examples
EXAMPLES_DOC_OUTPUT_PATH := $(EXAMPLES_DOC_PATH)/configuration-examples.md
EXAMPLES_YAML_PATH := $(EXAMPLES_DOC_PATH)/yaml
EXAMPLES_SKIP_VALIDATION_FLAG := "doc-example:skip-validation=true"

# Validate the example configurations that we provide in ./docs/sources/configure/examples
# We run the validation only for complete examples, not snippets.
# Complete examples should contain "Example" in their file name.
validate-example-configs: loki
	for f in $$(grep -rL $(EXAMPLES_SKIP_VALIDATION_FLAG) $(EXAMPLES_YAML_PATH)/*.yaml); do echo "Validating provided example config: $$f" && ./cmd/goscape/goscape --config.file=$$f --verify-config || exit 1; done

validate-dev-cluster-config: loki
	./cmd/goscape/goscape --config.file=./tools/dev/loki-tsdb-storage-s3/config/loki.yaml --verify-config

# Dynamically generate ./docs/sources/configure/examples.md using the example configs that we provide.
# This target should be run if any of our example configs change.
generate-example-config-doc:
	echo "Removing existing doc at $(EXAMPLES_DOC_OUTPUT_PATH) and re-generating. . ."
	# Title and Heading
	echo -e "---\ntitle: Configuration\ndescription: Goscape Configuration Examples and Snippets\nweight:  100\n---\n# Configuration" > $(EXAMPLES_DOC_OUTPUT_PATH)
	# Append each configuration and its file name to examples.md
	for f in $$(find $(EXAMPLES_YAML_PATH)/*.yaml -printf "%f\n" | sort -k1n); do \
		echo -e "\n## $$f\n\n\`\`\`yaml\n" >> $(EXAMPLES_DOC_OUTPUT_PATH); \
		grep -v $(EXAMPLES_SKIP_VALIDATION_FLAG) $(EXAMPLES_YAML_PATH)/$$f >> $(EXAMPLES_DOC_OUTPUT_PATH); \
		echo -e "\n\`\`\`\n" >> $(EXAMPLES_DOC_OUTPUT_PATH); \
	done


# Fail our CI build if changes are made to example configurations but our doc is not updated
check-example-config-doc: generate-example-config-doc
	@if ! (git diff --exit-code $(EXAMPLES_DOC_OUTPUT_PATH)); then \
		echo -e "\nChanges found in generated example configuration doc"; \
		echo "Run 'make generate-example-config-doc' and commit the changes to fix this error."; \
		echo "If you are actively developing these files you can ignore this error"; \
		echo -e "(Don't forget to check in the generated files when finished)\n"; \
		exit 1; \
	fi

dev-k3d-goscape:
	$(MAKE) -C $(CURDIR)/tools/dev/k3d goscape

dev-k3d-down:
	$(MAKE) -C $(CURDIR)/tools/dev/k3d down

# Snyk is used to scan for vulnerabilities
.PHONY: snyk
snyk: goscape-image goscape-cli-image
	snyk container test $(IMAGE_PREFIX)/goscape:$(IMAGE_TAG) --file=cmd/goscape/Dockerfile
	snyk container test $(IMAGE_PREFIX)/goscape-cli:$(IMAGE_TAG) --file=cmd/goscape-cli/Dockerfile
	snyk code test

.PHONY: scan-vulnerabilities
scan-vulnerabilities: snyk

.PHONY: release-workflows
release-workflows:
	pushd $(CURDIR)/.github && jb update && popd
	jsonnet -SJ .github/vendor -m .github/workflows -V GO_VERSION=$(GO_VERSION) .github/release-workflows.jsonnet

.PHONY: release-workflows-check
release-workflows-check:
	@$(MAKE) release-workflows
	@echo "Checking diff"
	@git diff --exit-code --ignore-space-at-eol -- ".github/workflows/*release*" || (echo "Please build release workflows by running 'make release-workflows'" && false)

.PHONY: update-goscape-release-sha
update-goscape-release-sha:
	@echo "Updating loki-release SHA in .github/jsonnetfile.json"
	@NEW_SHA=$$(curl -s https://api.github.com/repos/zsrv/goscape-release/commits/main | jq -r .sha); \
	jq --arg new_sha "$$NEW_SHA" '.dependencies[] |= if .source.git.remote == "https://github.com/zsrv/goscape-release.git" then .version = $$new_sha else . end' .github/jsonnetfile.json > .github/jsonnetfile.json.tmp && \
	mv .github/jsonnetfile.json.tmp .github/jsonnetfile.json
	@echo "Updated successfully"
	@$(MAKE) release-workflows

.PHONY: flake-update
flake-update:
	@docker run -v $(CURDIR):/goscape \
		--workdir /goscape \
		nixos/nix \
		nix \
		--extra-experimental-features nix-command \
		--extra-experimental-features flakes \
		flake update
