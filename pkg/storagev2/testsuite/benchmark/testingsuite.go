package test

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"runtime"
	"sync"
	"time"
)

type DB interface {
	Set(key, value []byte) error
	Get(key []byte) ([]byte, error)
	Del(key []byte) error
	Close() error
}

var (
	duration = flag.Duration("d", time.Minute, "test duration for each case")
	c        = flag.Int("c", runtime.NumCPU(), "concurrent goroutines")
	size     = flag.Int("size", 256, "data size")
)

var data = make([]byte, *size)

// test get
func testGet(name string, s DB) {
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	counts := make([]int, *c)
	start := time.Now()

	for j := 0; j < *c; j++ {
		wg.Add(1)

		index := uint64(j)
		go func() {
			defer wg.Done()

			i := index

			for {
				select {
				case <-ctx.Done():
					return
				default:
					v, _ := s.Get(genKey(i))
					if len(v) == 0 {
						i = index
					}

					i += uint64(*c)
					counts[index]++
				}
			}
		}()
	}

	wg.Wait()

	dur := time.Since(start)
	d := int64(dur)

	var n int
	for _, count := range counts {
		n += count
	}

	fmt.Printf(
		"%s get rate: %d op/s, mean: %d ns, took: %d s\n",
		name, int64(n)*1e6/(d/1e3), d/int64((n)*(*c)), int(dur.Seconds()),
	)
}

// test multiple get/one set
func testGetSet(name string, s DB) {
	var wg sync.WaitGroup

	ch := make(chan struct{})
	setCount := 0

	go func() {
		i := uint64(0)

		for {
			select {
			case <-ch:
				return
			default:
				if err := s.Set(genKey(i), data); err != nil {
					panic(err)
				}

				setCount++
				i++
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	counts := make([]int, *c)
	start := time.Now()

	for j := 0; j < *c; j++ {
		wg.Add(1)

		index := uint64(j)
		go func() {
			defer wg.Done()

			i := index

			for {
				select {
				case <-ctx.Done():
					return
				default:
					v, _ := s.Get(genKey(i))
					if len(v) == 0 {
						i = index
					}

					i += uint64(*c)
					counts[index]++
				}
			}
		}()
	}

	wg.Wait()

	close(ch)

	dur := time.Since(start)
	d := int64(dur)

	var n int
	for _, count := range counts {
		n += count
	}

	if setCount == 0 {
		fmt.Printf("%s setmixed rate: -1 op/s, mean: -1 ns, took: %d s\n", name, int(dur.Seconds()))
	} else {
		fmt.Printf(
			"%s setmixed rate: %d op/s, mean: %d ns, took: %d s\n",
			name, int64(setCount)*1e6/(d/1e3), d/int64(setCount), int(dur.Seconds()),
		)
	}

	fmt.Printf(
		"%s getmixed rate: %d op/s, mean: %d ns, took: %d s\n",
		name, int64(n)*1e6/(d/1e3), d/int64((n)*(*c)), int(dur.Seconds()),
	)
}

func testSet(name string, s DB) {
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	counts := make([]int, *c)
	start := time.Now()

	for j := 0; j < *c; j++ {
		wg.Add(1)

		index := uint64(j)
		go func() {
			defer wg.Done()

			i := index

			for {
				select {
				case <-ctx.Done():
					return
				default:
					if err := s.Set(genKey(i), data); err != nil {
						panic(err)
					}

					i += uint64(*c)
					counts[index]++
				}
			}
		}()
	}

	wg.Wait()

	dur := time.Since(start)
	d := int64(dur)

	var n int
	for _, count := range counts {
		n += count
	}

	fmt.Printf(
		"%s set rate: %d op/s, mean: %d ns, took: %d s\n",
		name, int64(n)*1e6/(d/1e3), d/int64((n)*(*c)), int(dur.Seconds()),
	)
}

func testDelete(name string, s DB) {
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	counts := make([]int, *c)
	start := time.Now()

	for j := 0; j < *c; j++ {
		wg.Add(1)

		index := uint64(j)
		go func() {
			defer wg.Done()

			i := index

			for {
				select {
				case <-ctx.Done():
					return
				default:
					if err := s.Del(genKey(i)); err != nil {
						panic(err)
					}

					i += uint64(*c)
					counts[index]++
				}
			}
		}()
	}

	wg.Wait()

	dur := time.Since(start)
	d := int64(dur)

	var n int
	for _, count := range counts {
		n += count
	}

	fmt.Printf(
		"%s del rate: %d op/s, mean: %d ns, took: %d s\n",
		name, int64(n)*1e6/(d/1e3), d/int64((n)*(*c)), int(dur.Seconds()),
	)
}

func genKey(i uint64) []byte {
	r := make([]byte, 9)
	r[0] = 'k'
	binary.BigEndian.PutUint64(r[1:], i)

	return r
}
