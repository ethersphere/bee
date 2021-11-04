package franky_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/franky"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
)

func TestFranky(t *testing.T) {
	dir := t.TempDir()
	f := franky.New(dir)
	defer f.Close()
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				ch := testingc.GenerateTestRandomChunk()
				f.Write(ch.Data())
			}
		}()
	}
	wg.Wait()
	fmt.Println("took", time.Since(start))
}
