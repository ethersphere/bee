GO ?= go
GOLANGCI_LINT ?= $$($(GO) env GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.30.0
GOGOPROTOBUF ?= protoc-gen-gogofaster
GOGOPROTOBUF_VERSION ?= v1.3.1
BEEKEEPER ?= $$($(GO) env GOPATH)/bin/beekeeper
BEELOCAL_BRANCH ?= main
BEEKEEPER_BRANCH ?= master

COMMIT ?= "$(shell git describe --long --dirty --always --match "" || true)"
LDFLAGS ?= -s -w -X github.com/ethersphere/bee.commit="$(COMMIT)"

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
	curl -sSfL https://raw.githubusercontent.com/ethersphere/beekeeper/master/install.sh | BEEKEEPER_INSTALL_DIR=$$($(GO) env GOPATH)/bin USE_SUDO=false bash
	test -f ~/.beekeeper.yaml || curl -sSfL https://raw.githubusercontent.com/ethersphere/beekeeper/${BEEKEEPER_BRANCH}/config/beekeeper-local.yaml -o ~/.beekeeper.yaml
	mkdir -p ~/.beekeeper && curl -sSfL https://raw.githubusercontent.com/ethersphere/beekeeper/${BEEKEEPER_BRANCH}/config/ci.yaml -o ~/.beekeeper/ci.yaml

.PHONY: beelocal
beelocal:
	curl -sSfL https://raw.githubusercontent.com/ethersphere/beelocal/$(BEELOCAL_BRANCH)/beelocal.sh | bash

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
