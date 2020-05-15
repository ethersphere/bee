package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

var (
	host        string // flag variable, http api host
	port        int    // flag variable, http api port
	ssl         bool   // flag variable, uses https for api if set
)

// apiStore provies a storage.Getter that retrieves chunks from swarm through the HTTP chunk API.
type apiStore struct {
	baseUrl string
}

// newApiStore creates a new apiStore
func newApiStore(host string, port int, ssl bool) (storage.Getter, error) {
	scheme := "http"
	if ssl {
		scheme += "s"
	}
	u := &url.URL{
		Host:   fmt.Sprintf("%s:%d", host, port),
		Scheme: scheme,
		Path:   "bzz-chunk",
	}
	return &apiStore{
		baseUrl: u.String(),
	}, nil
}

// Get implements storage.Getter
func (a *apiStore) Get(ctx context.Context, mode storage.ModeGet, address swarm.Address) (ch swarm.Chunk, err error) {
	addressHex := address.String()
	url := strings.Join([]string{a.baseUrl, addressHex}, "/")
	c := http.DefaultClient
	res, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chunk %s not found", addressHex)
	}
	chunkData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	ch = swarm.NewChunk(address, chunkData)
	return ch, nil
}

// Join is the underlying procedure for the CLI command
func Join(cmd *cobra.Command, args []string) (err error) {

	outFile := os.Stdout

	addr, err := swarm.ParseHexAddress(args[0])
	if err != nil {
		return err
	}

	store, err := newApiStore (host, port, ssl)
	if err != nil {
		return err
	}
	j := joiner.NewSimpleJoiner(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, l, err := j.Join(ctx, addr)
	if err != nil {
		return err
	}

	teeReader := io.TeeReader(r, outFile)
	data := make([]byte, swarm.ChunkSize)
	var total int64
	for i := int64(0); i < l; i += swarm.ChunkSize {
		c, err := teeReader.Read(data)
		if err != nil {
			return err
		}
		total += int64(c)
	}
	if total != l {
		return fmt.Errorf("received only %d of %d total bytes", total, l)
	}
	return nil
}

func main() {
	c := &cobra.Command{
		Use: "join [hash]",
		Args: cobra.ExactArgs(1),
		Short: "Retrieve data from Swarm",
		Long: `Assembles chunked data from referenced by a root Swarm Hash.

Will output retrieved data to stdout.`,
		RunE: Join,
	}
	c.Flags().StringVar(&host, "host", "127.0.0.1", "api host")
	c.Flags().IntVar(&port, "port", 8080, "api port")
	c.Flags().BoolVar(&ssl, "ssl", false, "use ssl")

	err := c.Execute()
	if err != nil {
		os.Exit(1)
	}
}
