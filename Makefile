GO ?= go
GOBIN ?= $$($(GO) env GOPATH)/bin
GOLANGCI_LINT ?= $(GOBIN)/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.61.0
GOGOPROTOBUF ?= protoc-gen-gogofaster
GOGOPROTOBUF_VERSION ?= v1.3.1
BEEKEEPER_INSTALL_DIR ?= $(GOBIN)
BEEKEEPER_USE_SUDO ?= false
BEEKEEPER_CLUSTER ?= local
BEELOCAL_BRANCH ?= main
BEEKEEPER_BRANCH ?= master
REACHABILITY_OVERRIDE_PUBLIC ?= false
BATCHFACTOR_OVERRIDE_PUBLIC ?= 5
BEE_IMAGE ?= ethersphere/bee:latest

BEE_API_VERSION ?= "$(shell grep '^  version:' openapi/Swarm.yaml | awk '{print $$2}')"

VERSION ?= "$(shell git describe --tags --abbrev=0 | cut -c2-)"
COMMIT_HASH ?= "$(shell git describe --long --dirty --always --match "" || true)"
CLEAN_COMMIT ?= "$(shell git describe --long --always --match "" || true)"
COMMIT_TIME ?= "$(shell git show -s --format=%ct $(CLEAN_COMMIT) || true)"
LDFLAGS ?= -s -w \
-X github.com/ethersphere/bee/v2.version="$(VERSION)" \
-X github.com/ethersphere/bee/v2.commitHash="$(COMMIT_HASH)" \
-X github.com/ethersphere/bee/v2.commitTime="$(COMMIT_TIME)" \
-X github.com/ethersphere/bee/v2/pkg/api.Version="$(BEE_API_VERSION)" \
-X github.com/ethersphere/bee/v2/pkg/p2p/libp2p.reachabilityOverridePublic="$(REACHABILITY_OVERRIDE_PUBLIC)" \
-X github.com/ethersphere/bee/v2/pkg/postage/listener.batchFactorOverridePublic="$(BATCHFACTOR_OVERRIDE_PUBLIC)"

.PHONY: all
all: build lint test-race binary

.PHONY: binary
binary: export CGO_ENABLED=0
binary: dist FORCE
	$(GO) version
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/bee ./cmd/bee

dist:
	mkdir $@

.PHONY: beekeeper
beekeeper:
ifeq ($(BEEKEEPER_BRANCH), master)
	curl -sSfL https://raw.githubusercontent.com/ethersphere/beekeeper/master/scripts/install.sh | BEEKEEPER_INSTALL_DIR=$(BEEKEEPER_INSTALL_DIR) USE_SUDO=$(BEEKEEPER_USE_SUDO) bash
else
	git clone -b $(BEEKEEPER_BRANCH) https://github.com/ethersphere/beekeeper.git && mv beekeeper beekeeper_src && cd beekeeper_src && mkdir -p $(BEEKEEPER_INSTALL_DIR) && make binary
ifeq ($(BEEKEEPER_USE_SUDO), true)
	sudo mv beekeeper_src/dist/beekeeper $(BEEKEEPER_INSTALL_DIR)
else
	mv beekeeper_src/dist/beekeeper $(BEEKEEPER_INSTALL_DIR)
endif
	rm -rf beekeeper_src
endif
	test -f ~/.beekeeper.yaml || curl -sSfL https://raw.githubusercontent.com/ethersphere/beekeeper/$(BEEKEEPER_BRANCH)/config/beekeeper-local.yaml -o ~/.beekeeper.yaml
	mkdir -p ~/.beekeeper && curl -sSfL https://raw.githubusercontent.com/ethersphere/beekeeper/$(BEEKEEPER_BRANCH)/config/local.yaml -o ~/.beekeeper/local.yaml

.PHONY: beelocal
beelocal:
	curl -sSfL https://raw.githubusercontent.com/ethersphere/beelocal/$(BEELOCAL_BRANCH)/beelocal.sh | bash

.PHONY: deploylocal
deploylocal:
	beekeeper create bee-cluster --cluster-name $(BEEKEEPER_CLUSTER)

.PHONY: testlocal
testlocal:
	export PATH=${PATH}:$(GOBIN)
	beekeeper check --cluster-name local --checks=ci-full-connectivity,ci-manifest,ci-pingpong,ci-pss,ci-pushsync-chunks,ci-retrieval,ci-settlements,ci-soc

.PHONY: testlocal-all
testlocal-all: beekeeper beelocal deploylocal testlocal

.PHONY: install-formatters
install-formatters:
	$(GO) get github.com/daixiang0/gci
	$(GO) install mvdan.cc/gofumpt@latest

FOLDER=$(shell pwd)

.PHONY: format
format:
	$(GOBIN)/gofumpt -l -w $(FOLDER)
	$(GOBIN)/gci -w -local $(go list -m) `find $(FOLDER) -type f \! -name "*.pb.go" -name "*.go" \! -path \*/\.git/\* -exec echo {} \;`

.PHONY: lint
lint: linter
	$(GOLANGCI_LINT) run ./...

.PHONY: linter
linter:
	test -f $(GOLANGCI_LINT) || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$($(GO) env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: check-whitespace
check-whitespace:
	TREE=$$(git hash-object -t tree /dev/null); \
	TW=$$(git diff-index --cached --check --diff-filter=d "$${TREE}"); \
	[ "$${TW}" != "" ] && echo "Trailing whitespaces found:\n $${TW}" && exit 1; exit 0

.PHONY: test-race
test-race:
ifdef cover
	$(GO) test -race -failfast -coverprofile=cover.out -v ./...
else
	$(GO) test -race -failfast -v ./...
endif

.PHONY: test-integration
test-integration:
	$(GO) test -tags=integration -v ./...

.PHONY: test
test:
ifdef cover
	$(GO) test -failfast -coverprofile=cover.out -v ./...
else
	$(GO) test -failfast -v ./...
endif

.PHONY: test-ci
test-ci:
ifdef cover
	$(GO) test -run "[^FLAKY]$$" -coverprofile=cover.out ./...
else
	$(GO) test -run "[^FLAKY]$$" ./...
endif

.PHONY: test-ci-race
test-ci-race:
ifdef cover
	$(GO) test -race -run "[^FLAKY]$$" -coverprofile=cover.out ./...
else
	$(GO) test -race -run "[^FLAKY]$$" ./...
endif

.PHONY: test-ci-flaky
test-ci-flaky:
	$(GO) test -race -run "FLAKY$$" ./...

.PHONY: build
build: export CGO_ENABLED=0
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" ./...

.PHONY: docker-build
docker-build: binary
	@echo "Build flags: $(LDFLAGS)"
	mkdir -p ./tmp
	cp ./dist/bee ./tmp/bee
	docker build -f Dockerfile.dev -t $(BEE_IMAGE) . --no-cache
	rm -rf ./tmp
	@echo "Docker image: $(BEE_IMAGE)"

.PHONY: githooks
githooks:
	ln -f -s ../../.githooks/pre-push.bash .git/hooks/pre-push

.PHONY: protobuftools
protobuftools:
	which protoc || ( echo "install protoc for your system from https://github.com/protocolbuffers/protobuf/releases" && exit 1)
	which $(GOGOPROTOBUF) || ( cd /tmp && GO111MODULE=on $(GO) install github.com/gogo/protobuf/$(GOGOPROTOBUF)@$(GOGOPROTOBUF_VERSION) )

.PHONY: protobuf
protobuf: GOFLAGS=-mod=mod # use modules for protobuf file include option
protobuf: protobuftools
	$(GO) generate -run protoc ./...

.PHONY: clean
clean:
	$(GO) clean
	rm -rf dist/

FORCE: