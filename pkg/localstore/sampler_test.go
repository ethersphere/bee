package localstore

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestReserveSampler(t *testing.T) {
	const chunkCountPerPO = 10
	const maxPO = 10
	var chs []swarm.Chunk

	db := newTestDB(t, &Options{
		Capacity:        1000,
		ReserveCapacity: 1000,
	})

	for po := 0; po < maxPO; po++ {
		for i := 0; i < chunkCountPerPO; i++ {
			ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), po).WithBatch(0, 3, 2, false)
			chs = append(chs, ch)
		}
	}

	_, err := db.Put(context.Background(), storage.ModePutSync, chs...)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("reserve size", reserveSizeTest(db, chunkCountPerPO*maxPO))

	t.Run("reserve sample", func(t *testing.T) {
		sample, err := db.ReserveSample(context.TODO(), []byte("anchor"), 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(sample.Items) != sampleSize {
			t.Fatalf("incorrect no of sample items exp %d found %d", sampleSize, len(sample.Items))
		}
		for i := 0; i < len(sample.Items)-2; i++ {
			if bytes.Compare(
				sample.Items[i].TransformedAddress.Bytes(),
				sample.Items[i+1].TransformedAddress.Bytes(),
			) != -1 {
				t.Fatalf("incorrect order of samples %+q", sample.Items)
			}
		}
	})
}
