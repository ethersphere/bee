GO ?= go
GOLANGCI_LINT ?= $$($(GO) env GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.37.0
GOGOPROTOBUF ?= protoc-gen-gogofaster
GOGOPROTOBUF_VERSION ?= v1.3.1
BEEKEEPER_INSTALL_DIR ?= $$($(GO) env GOPATH)/bin
BEEKEEPER_USE_SUDO ?= false
BEEKEEPER_CLUSTER ?= local
BEELOCAL_BRANCH ?= main
BEEKEEPER_BRANCH ?= master

COMMIT_HASH ?= "$(shell git describe --long --dirty --always --match "" || true)"
CLEAN_COMMIT ?= "$(shell git describe --long --always --match "" || true)"
COMMIT_TIME ?= "$(shell git show -s --format=%ct $(CLEAN_COMMIT) || true)"
LDFLAGS ?= -s -w -X github.com/ethersphere/bee.commitHash="$(COMMIT_HASH)" -X github.com/ethersphere/bee.commitTime="$(COMMIT_TIME)"

.PHONY: all
all: build lint vet test-race binary

.PHONY: binary
binary: export CGO_ENABLED=0
binary: dist FORCE
	$(GO) version
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/bee ./cmd/bee

dist:
	mkdir $@

.PHONY: beekeeper
beekeeper:
	git clone -b $(BEEKEEPER_BRANCH) https://github.com/ethersphere/beekeeper.git && cd beekeeper && mkdir -p $(BEEKEEPER_INSTALL_DIR) && make binary && sudo mv dist/beekeeper $(BEEKEEPER_INSTALL_DIR)
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
	export PATH=${PATH}:$$($(GO) env GOPATH)/bin
	beekeeper check --cluster-name local --checks=ci-full-connectivity,ci-gc,ci-manifest,ci-pingpong,ci-pss,ci-pushsync-chunks,ci-retrieval,ci-content-availability,ci-settlements,ci-soc

.PHONY: testlocal-all
all: beekeeper beelocal deploylocal testlocal

.PHONY: lint
lint: linter
	$(GOLANGCI_LINT) run

.PHONY: linter
linter:
	test -f $(GOLANGCI_LINT) || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$($(GO) env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test-race
test-race:
	$(GO) test -race -failfast -v ./...

.PHONY: test-integration
test-integration:
	$(GO) test -tags=integration -v ./...

.PHONY: test
test:
	$(GO) test -v -failfast ./...

.PHONY: build
build: export CGO_ENABLED=0
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" ./...

.PHONY: githooks
githooks:
	ln -f -s ../../.githooks/pre-push.bash .git/hooks/pre-push

.PHONY: protobuftools
protobuftools:
	which protoc || ( echo "install protoc for your system from https://github.com/protocolbuffers/protobuf/releases" && exit 1)
	which $(GOGOPROTOBUF) || ( cd /tmp && GO111MODULE=on $(GO) get -u github.com/gogo/protobuf/$(GOGOPROTOBUF)@$(GOGOPROTOBUF_VERSION) )

.PHONY: protobuf
protobuf: GOFLAGS=-mod=mod # use modules for protobuf file include option
protobuf: protobuftools
	$(GO) generate -run protoc ./...

.PHONY: clean
clean:
	$(GO) clean
	rm -rf dist/

FORCE:
