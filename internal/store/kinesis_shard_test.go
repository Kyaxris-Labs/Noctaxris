package store_test

import (
	"fmt"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLabKinesisShardIDAndHash(t *testing.T) {
	if got := store.LabKinesisShardID(0); got != "shardId-000000000000" {
		t.Fatalf("shard0=%q", got)
	}
	if got := store.LabKinesisShardID(3); got != "shardId-000000000003" {
		t.Fatalf("shard3=%q", got)
	}
	a := store.HashPartitionKeyToShard("pk-stable", 2)
	b := store.HashPartitionKeyToShard("pk-stable", 2)
	if a != b {
		t.Fatalf("hash not stable: %d vs %d", a, b)
	}
	if a < 0 || a >= 2 {
		t.Fatalf("hash out of range: %d", a)
	}
}

func TestKinesisMultiShardPutAndGet(t *testing.T) {
	st := openKinesisStore(t)
	account := "000000000001"
	region := "us-east-1"

	stream, err := st.CreateKinesisStream(account, region, "ms", 2)
	if err != nil {
		t.Fatal(err)
	}
	if stream.ShardCount != 2 {
		t.Fatalf("ShardCount=%d want 2", stream.ShardCount)
	}

	_, err = st.CreateKinesisStream(account, region, "too-many", 5)
	if err == nil {
		t.Fatal("want error for ShardCount > 4")
	}

	// Find two partition keys that land on different shards.
	var key0, key1 string
	for i := 0; i < 1000; i++ {
		pk := fmt.Sprintf("pk-%d", i)
		idx := store.HashPartitionKeyToShard(pk, 2)
		if idx == 0 && key0 == "" {
			key0 = pk
		}
		if idx == 1 && key1 == "" {
			key1 = pk
		}
		if key0 != "" && key1 != "" {
			break
		}
	}
	if key0 == "" || key1 == "" {
		t.Fatal("could not find keys for both shards")
	}

	seq0, shard0, err := st.PutKinesisRecord(account, "ms", key0, []byte("on-shard-0"))
	if err != nil {
		t.Fatal(err)
	}
	if shard0 != store.LabKinesisShardID(0) {
		t.Fatalf("shard0=%q", shard0)
	}
	seq1, shard1, err := st.PutKinesisRecord(account, "ms", key1, []byte("on-shard-1"))
	if err != nil {
		t.Fatal(err)
	}
	if shard1 != store.LabKinesisShardID(1) {
		t.Fatalf("shard1=%q", shard1)
	}
	if seq0 == "" || seq1 == "" || seq0 == seq1 {
		t.Fatalf("seqs=%q %q", seq0, seq1)
	}

	desc, err := st.DescribeKinesisStream(account, "ms")
	if err != nil {
		t.Fatal(err)
	}
	if desc.ShardCount != 2 {
		t.Fatalf("describe ShardCount=%d", desc.ShardCount)
	}
	ids := store.LabKinesisShardIDs(desc.ShardCount)
	if len(ids) != 2 {
		t.Fatalf("shard ids=%v", ids)
	}

	it0, err := st.GetKinesisShardIterator(account, "ms", store.LabKinesisShardID(0), "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs0, _, err := st.GetKinesisRecords(it0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs0) != 1 || string(recs0[0].Data) != "on-shard-0" {
		t.Fatalf("shard0 recs=%v", recs0)
	}

	it1, err := st.GetKinesisShardIterator(account, "ms", store.LabKinesisShardID(1), "TRIM_HORIZON", "")
	if err != nil {
		t.Fatal(err)
	}
	recs1, _, err := st.GetKinesisRecords(it1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs1) != 1 || string(recs1[0].Data) != "on-shard-1" {
		t.Fatalf("shard1 recs=%v", recs1)
	}

	if _, err := st.GetKinesisShardIterator(account, "ms", store.LabKinesisShardID(2), "TRIM_HORIZON", ""); err != store.ErrKinesisInvalidShard {
		t.Fatalf("want invalid shard got %v", err)
	}
}
