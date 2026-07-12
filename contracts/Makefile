.PHONY: protobuf protobuftools

GO ?= go
GOPATH_BIN := $(shell $(GO) env GOPATH)/bin

protobuftools:
	which protoc || (echo "install protoc from https://github.com/protocolbuffers/protobuf/releases" && exit 1)
	which $(GOPATH_BIN)/protoc-gen-go || $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
	which $(GOPATH_BIN)/protoc-gen-go-grpc || $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

protobuf: protobuftools
	mkdir -p pb
	PATH="$(GOPATH_BIN):$$PATH" protoc \
		--proto_path=proto \
		--go_out=pb --go_opt=paths=source_relative \
		--go-grpc_out=pb --go-grpc_opt=paths=source_relative \
		proto/storage.proto
