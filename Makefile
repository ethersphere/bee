GO ?= go
GOLANGCI_LINT ?= golangci-lint

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
	$(GO) build -trimpath -ldflags "-s -w" ./...
