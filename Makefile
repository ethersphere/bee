COMMIT ?= ""

GO ?= go
GOLANGCI_LINT ?= golangci-lint

<<<<<<< HEAD
LDFLAGS ?= -s -w -X github.com/janos/bee.commit="$(COMMIT)"
=======
LDFLAGS ?= -s -w -X github.com/ethersphere/bee.commit="$(COMMIT)"
>>>>>>> 1971bae2015d482f60af4e87b73d1999abc90157

.PHONY: all
all: build lint vet test binary

.PHONY: binary
binary: export CGO_ENABLED=0
binary: dist FORCE
	$(GO) version
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/bee ./cmd/bee

dist:
	mkdir $@

.PHONY: lint
lint:
	$(GOLANGCI_LINT) run

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test
test:
	$(GO) test -v -race ./...

.PHONY: build
build: export CGO_ENABLED=0
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" ./...

.PHONY: clean
clean:
	$(GO) clean
	rm -rf dist/

FORCE:
