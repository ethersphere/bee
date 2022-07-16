package api

import (
	"errors"
	"fmt"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/mux"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/wapc/wapc-go"
	"github.com/wapc/wapc-go/engines/wazero"
	"golang.org/x/net/context"
	"io"
	"net/http"
)

func handleStoreChunk(server *server, request *http.Request, ctx context.Context, payload []byte) ([]byte, error) {
	var args StoreChunkArgs
	err := msgpack.Unmarshal(payload, &args)
	data := args.Data

	if len(data) < swarm.SpanSize {
		server.logger.Debug("chunk upload: not enough data")
		server.logger.Error("chunk upload: data length")
		return nil, errors.New("not enough data")
	}

	putter, wait, err := server.newStamperPutter(request)
	if err != nil {
		server.logger.Debugf("chunk upload: putter: %v", err)
		server.logger.Error("chunk upload: putter")
		switch {
		case errors.Is(err, postage.ErrNotFound):
			return nil, errors.New("batch not found")
		case errors.Is(err, postage.ErrNotUsable):
			return nil, errors.New("batch not usable")
		}
		return nil, err
	}

	chunk, err := cac.NewWithDataSpan(data)
	if err != nil {
		return nil, err
	}

	_, err = putter.Put(ctx, storage.ModePutUpload, chunk)
	if err != nil {
		return nil, err
	}

	if err = wait(); err != nil {
		server.logger.Debugf("chunk upload: sync chunk: %v", err)
		server.logger.Error("chunk upload: sync chunk")
		return nil, err
	}

	chunkRef := chunk.Address().String()

	return msgpack.Marshal(chunkRef)
}

func handleGetChunk(server *server, ctx context.Context, payload []byte) ([]byte, error) {
	var inputArgs GetChunkArgs
	err := msgpack.Unmarshal(payload, &inputArgs)
	if err != nil {
		return nil, err
	}
	nameOrHex := inputArgs.Reference

	address, err := server.resolveNameOrAddress(nameOrHex)
	if err != nil {
		server.logger.Debugf("chunk: parse chunk address %s: %v", nameOrHex, err)
		server.logger.Error("chunk: parse chunk address error")
		return nil, err
	}

	chunk, err := server.storer.Get(ctx, storage.ModeGetRequest, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			server.logger.Tracef("chunk: chunk not found. addr %s", address)
			return nil, err

		}
		server.logger.Debugf("chunk: chunk read error: %v ,addr %s", err, address)
		server.logger.Error("chunk: chunk read error")
		return nil, err
	}

	chunkData := chunk.Data()

	return msgpack.Marshal(&chunkData)
}

func bootstrapEngine(server *server, request *http.Request, ctx context.Context, protocolBinary []byte) (wapc.Module, wapc.Instance) {
	hostCall := func(ctx context.Context, binding, namespace, operation string, payload []byte) ([]byte, error) {
		switch namespace {
		case "swarm":
			switch operation {
			case "storeChunk":
				return handleStoreChunk(server, request, ctx, payload)
			case "getChunk":
				return handleGetChunk(server, ctx, payload)
			}
		}
		return []byte(""), nil
	}

	engine := wazero.Engine()
	module, err := engine.New(ctx, protocolBinary, hostCall)
	if err != nil {
		panic(err)
	}

	module.SetLogger(wapc.Println)
	module.SetWriter(wapc.Print)

	instance, err := module.Instantiate(ctx)
	if err != nil {
		panic(err)
	}

	return module, instance
}

func (s *server) handleProtocol(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := tracing.NewLoggerWithTraceID(ctx, s.logger)

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("handleProtocol: invalid request body ")
		jsonhttp.BadRequest(w, nil)
		return
	}

	protocol := mux.Vars(r)["protocol"]
	rest := mux.Vars(r)["rest"]

	// TODO: Add headers
	request := HttpRequest{
		Path: rest,
		Body: string(requestBody),
	}

	protocolAddress, err := s.resolveProtocol(protocol)
	if err != nil {
		logger.Debugf("handleProtocol: invalid protocol %s: %v", protocol, err)
		logger.Error("handleProtocol: invalid protocol ")
		jsonhttp.BadRequest(w, nil)
		return
	}

	protocolBinary, err := s.retrieveProtocol(ctx, protocolAddress)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			logger.Debugf("handleProtocol: protocol not found %s: %v", protocolAddress, err)
			logger.Error("handleProtocol: protocol not found")
			jsonhttp.BadRequest(w, "protocol not found")
			return
		}

		logger.Debugf("handleProtocol: invalid protocol %s: %v", protocolAddress, err)
		logger.Error("handleProtocol: invalid protocol ")
		jsonhttp.BadRequest(w, nil)
		return
	}

	module, instance := bootstrapEngine(s, r, ctx, protocolBinary)
	defer module.Close(ctx)
	defer instance.Close(ctx)

	host := NewHost(instance)
	result, err := host.HandleRequest(ctx, request)
	if err != nil {
		panic(err)
	}

	if result.Status != nil {
		w.WriteHeader(int(*result.Status))
	}

	for _, header := range result.Headers {
		w.Header().Set(header.Name, header.Value)
	}

	if result.Body != nil {
		fmt.Fprint(w, *result.Body)
	}
}

type Host struct {
	instance wapc.Instance
}

func NewHost(instance wapc.Instance) *Host {
	return &Host{
		instance: instance,
	}
}

func (m *Host) HandleRequest(ctx context.Context, request HttpRequest) (HttpResponse, error) {
	var ret HttpResponse
	inputArgs := HandleRequestArgs{
		Request: request,
	}
	inputPayload, err := msgpack.Marshal(&inputArgs)
	if err != nil {
		return ret, err
	}
	payload, err := m.instance.Invoke(
		ctx,
		"handleRequest",
		inputPayload,
	)
	if err != nil {
		return ret, err
	}
	err = msgpack.Unmarshal(payload, &ret)
	return ret, err
}

type GetChunkArgs struct {
	Reference string `json:"reference" msgpack:"reference"`
}

type StoreChunkArgs struct {
	Data []byte `json:"data" msgpack:"data"`
}

type HandleRequestArgs struct {
	Request HttpRequest `json:"request" msgpack:"request"`
}

type HttpResponse struct {
	Status  *uint16  `json:"status" msgpack:"status"`
	Headers []Header `json:"headers" msgpack:"headers"`
	Body    *string  `json:"body" msgpack:"body"`
}

type HttpRequest struct {
	Headers []Header `json:"headers" msgpack:"headers"`
	Body    string   `json:"body" msgpack:"body"`
	Path    string
}

type Header struct {
	Name  string `json:"name" msgpack:"name"`
	Value string `json:"value" msgpack:"value"`
}
