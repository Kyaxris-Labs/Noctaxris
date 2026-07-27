package store_test

import (
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestKinesisEFOConsumerLifecycle(t *testing.T) {
	st := openKinesisStore(t)
	account := "000000000001"
	region := "us-east-1"
	if _, err := st.CreateKinesisStream(account, region, "efo", 1); err != nil {
		t.Fatal(err)
	}
	c, err := st.RegisterKinesisConsumer(account, region, "efo", "app")
	if err != nil {
		t.Fatal(err)
	}
	if c.ConsumerARN == "" || c.ConsumerStatus != "ACTIVE" {
		t.Fatalf("consumer=%+v", c)
	}
	got, err := st.DescribeKinesisConsumer(account, "efo", "app", "")
	if err != nil || got.ConsumerName != "app" {
		t.Fatalf("describe=%+v err=%v", got, err)
	}
	got, err = st.DescribeKinesisConsumer(account, "", "", c.ConsumerARN)
	if err != nil || got.ConsumerName != "app" {
		t.Fatalf("describe arn=%+v err=%v", got, err)
	}
	list, err := st.ListKinesisConsumers(account, "efo")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, _, cont, err := st.SubscribeToShardLab(account, c.ConsumerARN, store.LabKinesisShardID(0), "TRIM_HORIZON", "", 10); err != nil {
		t.Fatal(err)
	} else if cont == "" {
		// empty stream still returns continuation iterator id
	}
	if err := st.DeregisterKinesisConsumer(account, "efo", "app", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DescribeKinesisConsumer(account, "efo", "app", ""); !errors.Is(err, store.ErrKinesisConsumerNotFound) {
		t.Fatalf("want not found got %v", err)
	}
}

func TestKinesisUpdateShardCount(t *testing.T) {
	st := openKinesisStore(t)
	account := "000000000001"
	if _, err := st.CreateKinesisStream(account, "us-east-1", "scale", 1); err != nil {
		t.Fatal(err)
	}
	st2, err := st.UpdateKinesisShardCount(account, "scale", 3)
	if err != nil || st2.ShardCount != 3 {
		t.Fatalf("st=%+v err=%v", st2, err)
	}
	desc, err := st.DescribeKinesisStream(account, "scale")
	if err != nil || desc.ShardCount != 3 {
		t.Fatalf("desc=%+v err=%v", desc, err)
	}
	ids := store.LabKinesisShardIDs(desc.ShardCount)
	if len(ids) != 3 {
		t.Fatalf("ids=%v", ids)
	}
	start, end := store.LabKinesisHashKeyRange(0, 3)
	if start == "" || end == "" {
		t.Fatal("empty hash range")
	}
	if _, err := st.UpdateKinesisShardCount(account, "scale", 5); !errors.Is(err, store.ErrKinesisInvalidShard) {
		t.Fatalf("want invalid got %v", err)
	}
}
