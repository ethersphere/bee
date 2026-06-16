// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethersphere/bee/v2/pkg/chainsim"
)

func main() {
	configPath := flag.String("config", "chainsim.yaml", "path to YAML config file")
	flag.Parse()

	yamlCfg, err := loadYAMLConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	simCfg, err := yamlCfg.toSimConfig()
	if err != nil {
		log.Fatalf("sim config: %v", err)
	}

	store, err := newStateStore(yamlCfg.StateDir)
	if err != nil {
		log.Fatalf("state store: %v", err)
	}

	var sim *chainsim.SimChain
	if store.exists() {
		snap, err := store.load()
		if err != nil {
			log.Fatalf("load state: %v", err)
		}
		sim, err = chainsim.Restore(simCfg, snap)
		if err != nil {
			log.Fatalf("restore state: %v", err)
		}
		log.Printf("restored state at block %d from %s", sim.BlockCount(), yamlCfg.StateDir)
	} else {
		sim = chainsim.New(simCfg)
		if err := applyGenesisAccounts(sim, yamlCfg.Accounts); err != nil {
			log.Fatalf("genesis accounts: %v", err)
		}
		if err := store.save(sim); err != nil {
			log.Fatalf("save initial state: %v", err)
		}
		log.Printf("initialized new chain (chain_id=%d)", yamlCfg.ChainID)
	}
	defer sim.Close()

	attachStateSaver(sim, store)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go sim.Run(ctx)

	httpServer := newRPCServer(sim, yamlCfg.ChainID)
	httpServer.Addr = yamlCfg.RPC.Endpoint

	go func() {
		log.Printf("chainsim listening on http://%s", yamlCfg.RPC.Endpoint)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("rpc server: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down at block %d", sim.BlockCount())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("rpc shutdown: %v", err)
	}

	if err := store.save(sim); err != nil {
		log.Printf("final state save: %v", err)
	}
}
