package api

import (
	"errors"
	"fmt"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/mux"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/wapc/wapc-go"
	"github.com/wapc/wapc-go/engines/wazero"
	"golang.org/x/net/context"
	"io"
	"net/http"
)

func bootstrapEngine(ctx context.Context, protocolBinary []byte) (wapc.Module, wapc.Instance) {
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

	module, instance := bootstrapEngine(ctx, protocolBinary)
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

	fmt.Fprint(w, result.Body)
}


func hostCall(ctx context.Context, binding, namespace, operation string, payload []byte) ([]byte, error) {
	// Route the payload to any custom functionality accordingly.
	// You can even route to other waPC modules!!!
	switch namespace {
	// TODO: Add handling for getChunk & storeChunk
	case "swarm":
		switch operation {
		case "storeChunk":
			name := string(payload)
			fmt.Println(name)
			return nil, nil
		}
	}
	return []byte(""), nil
}


type Host struct {
	instance wapc.Instance
}

func NewHost(instance wapc.Instance) *Host {
	return &Host{
		instance: instance,
	}
}

func (m *Host) GetChunk(ctx context.Context, reference string) ([]byte, error) {
	var ret []byte
	inputArgs := GetChunkArgs{
		Reference: reference,
	}
	inputPayload, err := msgpack.Marshal(&inputArgs)
	if err != nil {
		return ret, err
	}
	payload, err := m.instance.Invoke(
		ctx,
		"getChunk",
		inputPayload,
	)
	if err != nil {
		return ret, err
	}
	err = msgpack.Unmarshal(payload, &ret)
	return ret, err
}

func (m *Host) StoreChunk(ctx context.Context, data []byte) (string, error) {
	var ret string
	inputArgs := StoreChunkArgs{
		Data: data,
	}
	inputPayload, err := msgpack.Marshal(&inputArgs)
	if err != nil {
		return ret, err
	}
	payload, err := m.instance.Invoke(
		ctx,
		"storeChunk",
		inputPayload,
	)
	if err != nil {
		return ret, err
	}
	err = msgpack.Unmarshal(payload, &ret)
	return ret, err
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
