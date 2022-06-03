package api

import (
	"errors"
	"fmt"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/mux"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/assemblyscript"
	"golang.org/x/net/context"
	"log"
	"net/http"
	"os"
)

func bootstrapRuntime(ctx context.Context, w http.ResponseWriter, r *http.Request) wazero.Runtime {
	// Create a new WebAssembly Runtime.
	runtime := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfig().
		WithWasmCore2())
	defer runtime.Close(ctx) // This closes everything this Runtime created.

	_, err := assemblyscript.Instantiate(ctx, runtime)
	if err != nil {
		log.Panicln(err)
	}


	return runtime
}

func (s *server) engine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := tracing.NewLoggerWithTraceID(ctx, s.logger)

	protocol := mux.Vars(r)["protocol"]
	//rest := mux.Vars(r)["rest"]

	protocolAddress, err := s.resolveProtocol(protocol)
	if err != nil {
		logger.Debugf("engine: invalid protocol %s: %v", protocol, err)
		logger.Error("engine: invalid protocol ")
		jsonhttp.BadRequest(w, nil)
		return
	}

	protocolBinary, err := s.retrieveProtocol(ctx, protocolAddress)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			logger.Debugf("engine: protocol not found %s: %v", protocolAddress, err)
			logger.Error("engine: protocol not found")
			jsonhttp.BadRequest(w, "protocol not found")
			return
		}

		logger.Debugf("engine: invalid protocol %s: %v", protocolAddress, err)
		logger.Error("engine: invalid protocol ")
		jsonhttp.BadRequest(w, nil)
		return
	}

	// Create a new WebAssembly Runtime.
	runtime := bootstrapRuntime(ctx, w, r)

	// Compile the WebAssembly module using the default configuration.
	protocolModule, err := runtime.CompileModule(ctx, protocolBinary, wazero.NewCompileConfig())
	if err != nil {
		logger.Debugf("engine: wasm compilation of protocol %s failed: %v", protocolAddress, err)
		logger.Error("engine: wasm compilation of protocol failed")
		jsonhttp.BadRequest(w, "bad protocol")
		return
	}

	module, err := runtime.InstantiateModule(ctx, protocolModule, wazero.NewModuleConfig().WithStdout(os.Stdout).WithStderr(os.Stderr))
	if err != nil {
		logger.Debugf("engine: wasm instantiation of protocol %s failed: %v", protocolAddress, err)
		logger.Error("engine: wasm instantiation of protocol failed")
		jsonhttp.BadRequest(w, "bad protocol")
		return
	}

	result, err := module.ExportedFunction("handleProtocol").Call(ctx, 2)
	if err != nil {
		logger.Debugf("engine: execution of protocol %s failed: %v", protocolAddress, err)
		logger.Error("engine: execution of of protocol failed")
		jsonhttp.BadRequest(w, "bad protocol run")
		return
	}

	fmt.Fprint(w, result)
}