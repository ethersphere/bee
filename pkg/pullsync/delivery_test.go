package pullsync_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	pullsyncold "github.com/ethersphere/bee-old/pkg/pullsync"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p"
	"github.com/ethersphere/bee/v2/pkg/pullsync"

	"github.com/ethersphere/bee/v2/pkg/soc"
	statestoremock "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mock "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
)

func BenchmarkDeliveryLibP2P(b *testing.B) {
	for _, n := range []int{25, 120} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			testChunks := make([]swarm.Chunk, n)
			testResults := make([]*storer.BinC, n)
			for i := range n {
				testChunks[i] = testingc.GenerateTestRandomChunk()
				stampHash, _ := testChunks[i].Stamp().Hash()
				testResults[i] = &storer.BinC{
					Address:   testChunks[i].Address(),
					BatchID:   testChunks[i].Stamp().BatchID(),
					BinID:     uint64(testChunks[i].Address().Bytes()[0]),
					StampHash: stampHash,
				}
			}

			b.Run("Batched", func(b *testing.B) {
				logger := log.Noop
				serverService, serverAddr := newLibp2pService(b, 1, logger)
				clientService, _ := newLibp2pService(b, 1, logger)

				baseReserve := mock.NewReserve()
				res1 := &customReserve{
					Reserve: baseReserve,
					chunks:  testChunks,
					results: testResults,
				}

				psServer := pullsync.New(serverService, res1, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, logger, uint64(n))

				if err := serverService.AddProtocol(psServer.Protocol()); err != nil {
					b.Fatal(err)
				}

				store2 := &customClientReserve{Reserve: mock.NewReserve()}
				psClient := pullsync.New(clientService, store2, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, logger, uint64(n))

				if err := clientService.AddProtocol(psClient.Protocol()); err != nil {
					b.Fatal(err)
				}

				serverAddrs, err := serverService.Addresses()
				if err != nil {
					b.Fatal(err)
				}

				_, err = clientService.Connect(context.Background(), serverAddrs)
				if err != nil {
					b.Fatal(err)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, count, err := psClient.Sync(ctx, serverAddr, 0, 0)
					cancel()

					if err != nil {
						b.Fatalf("Sync failed: %v", err)
					}
					if count != n {
						b.Fatalf("Expected %d chunks, got %d", n, count)
					}
				}
			})

			b.Run("v2.8.1", func(b *testing.B) {
				logger := log.Noop
				serverService, serverAddr := newLibp2pService(b, 1, logger)
				clientService, _ := newLibp2pService(b, 1, logger)

				baseReserve := mock.NewReserve()
				res1 := &customReserve{
					Reserve: baseReserve,
					chunks:  testChunks,
					results: testResults,
				}

				psServer := pullsyncold.New(serverService, res1, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, logger, uint64(n))

				if err := serverService.AddProtocol(psServer.Protocol()); err != nil {
					b.Fatal(err)
				}

				store2 := &customClientReserve{Reserve: mock.NewReserve()}
				psClient := pullsyncold.New(clientService, store2, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, logger, uint64(n))

				if err := clientService.AddProtocol(psClient.Protocol()); err != nil {
					b.Fatal(err)
				}

				serverAddrs, err := serverService.Addresses()
				if err != nil {
					b.Fatal(err)
				}

				_, err = clientService.Connect(context.Background(), serverAddrs)
				if err != nil {
					b.Fatal(err)
				}

				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, count, err := psClient.Sync(ctx, serverAddr, 0, 0)
					cancel()

					if err != nil {
						b.Fatalf("Sync failed: %v", err)
					}
					if count != n {
						b.Fatalf("Expected %d chunks, got %d", n, count)
					}
				}
			})
		})
	}
}

func newLibp2pService(t testing.TB, networkID uint64, logger log.Logger) (*libp2p.Service, swarm.Address) {
	t.Helper()

	swarmKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}

	nonce := common.HexToHash("0x1").Bytes()

	overlay, err := crypto.NewOverlayAddress(swarmKey.PublicKey, networkID, nonce)
	if err != nil {
		t.Fatal(err)
	}

	addr := "127.0.0.1:0"

	statestore := statestoremock.NewStateStore()
	ab := addressbook.New(statestore)

	libp2pKey, err := crypto.GenerateSecp256r1Key()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	lightNodes := lightnode.NewContainer(overlay)

	opts := libp2p.Options{
		PrivateKey:        libp2pKey,
		Nonce:             nonce,
		FullNode:          true,
		NATAddr:           "127.0.0.1:0", // Disable default NAT manager
		AllowPrivateCIDRs: true,
	}

	s, err := libp2p.New(ctx, crypto.NewDefaultSigner(swarmKey), networkID, overlay, addr, ab, statestore, lightNodes, logger, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_ = s.Ready()

	return s, overlay
}

type customReserve struct {
	storer.Reserve
	chunks  []swarm.Chunk
	results []*storer.BinC
}

func (c *customReserve) ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte, stampHash []byte) (swarm.Chunk, error) {
	for _, chunk := range c.chunks {
		if chunk.Address().Equal(addr) {
			return chunk, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (c *customReserve) SubscribeBin(ctx context.Context, bin uint8, start uint64) (<-chan *storer.BinC, func(), <-chan error) {
	out := make(chan *storer.BinC)
	errC := make(chan error, 1)

	go func() {
		defer close(out)
		for _, r := range c.results {
			select {
			case out <- r:
			case <-ctx.Done():
				errC <- ctx.Err()
				return
			}
		}
	}()

	return out, func() {}, errC
}

type customClientReserve struct {
	storer.Reserve
}

func (c *customClientReserve) ReserveHas(addr swarm.Address, batchID []byte, stampHash []byte) (bool, error) {
	return false, nil
}

func (c *customClientReserve) Put(ctx context.Context, chunk swarm.Chunk) error {
	return nil
}

func BenchmarkDeliveryPipe(b *testing.B) {
	for _, n := range []int{25, 120} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.StopTimer()

			testChunks := make([]swarm.Chunk, n)
			testResults := make([]*storer.BinC, n)
			for i := range n {
				testChunks[i] = testingc.GenerateTestRandomChunk()
				stampHash, _ := testChunks[i].Stamp().Hash()
				testResults[i] = &storer.BinC{
					Address:   testChunks[i].Address(),
					BatchID:   testChunks[i].Stamp().BatchID(),
					BinID:     uint64(testChunks[i].Address().Bytes()[0]),
					StampHash: stampHash,
				}
			}

			b.Run("Batched", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					store1 := mock.NewReserve(mock.WithSubscribeResp(testResults, nil), mock.WithChunks(testChunks...))
					psServer := pullsync.New(nil, store1, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, log.Noop, uint64(n))

					streamer := &pipeStreamer{serverHandler: psServer.Protocol().StreamSpecs[0].Handler}

					store2 := &customClientReserve{Reserve: mock.NewReserve()}
					psClient := pullsync.New(streamer, store2, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, log.Noop, uint64(n))

					b.StartTimer()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, count, err := psClient.Sync(ctx, swarm.ZeroAddress, 0, 0)
					cancel()
					b.StopTimer()

					if err != nil {
						b.Fatalf("Sync failed: %v", err)
					}
					if count != n {
						b.Fatalf("Expected %d chunks, got %d", n, count)
					}
					psClient.Close()
					psServer.Close()
				}
			})

			b.Run("v2.8.1", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					store1 := mock.NewReserve(mock.WithSubscribeResp(testResults, nil), mock.WithChunks(testChunks...))
					psServer := pullsyncold.New(nil, store1, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, log.Noop, uint64(n))

					streamer := &pipeStreamer{serverHandler: psServer.Protocol().StreamSpecs[0].Handler}

					store2 := &customClientReserve{Reserve: mock.NewReserve()}
					psClient := pullsyncold.New(streamer, store2, func(swarm.Chunk) {}, func(*soc.SOC) {}, func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }, log.Noop, uint64(n))

					b.StartTimer()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, count, err := psClient.Sync(ctx, swarm.ZeroAddress, 0, 0)
					cancel()
					b.StopTimer()

					if err != nil {
						b.Fatalf("Sync failed: %v", err)
					}
					if count != n {
						b.Fatalf("Expected %d chunks, got %d", n, count)
					}
					psClient.Close()
					psServer.Close()
				}
			})
		})
	}
}

type pipeStream struct {
	net.Conn
	headers p2p.Headers
}

func (p *pipeStream) Read(b []byte) (n int, err error)  { return p.Conn.Read(b) }
func (p *pipeStream) Write(b []byte) (n int, err error) { return p.Conn.Write(b) }
func (p *pipeStream) Headers() p2p.Headers              { return p.headers }
func (p *pipeStream) ResponseHeaders() p2p.Headers      { return p.headers }
func (p *pipeStream) FullClose() error                  { return p.Conn.Close() }
func (p *pipeStream) Reset() error                      { return p.Conn.Close() }

type pipeStreamer struct {
	serverHandler p2p.HandlerFunc
}

func (s *pipeStreamer) NewStream(ctx context.Context, address swarm.Address, h p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	c1, c2 := net.Pipe()

	clientStream := &pipeStream{Conn: c1, headers: h}
	serverStream := &pipeStream{Conn: c2, headers: h}

	go func() {
		_ = s.serverHandler(context.Background(), p2p.Peer{Address: address}, serverStream)
		_ = serverStream.FullClose()
	}()

	return clientStream, nil
}
