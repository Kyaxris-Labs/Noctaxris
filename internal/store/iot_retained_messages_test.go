package store_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestIoTRetainedMessagesPutListAccountIsolation(t *testing.T) {
	st := openIoTStore(t)
	a1 := "000000000001"
	a2 := "000000000002"
	region := "us-east-1"

	empty, err := st.ListIoTRetainedMessages(a1, region)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty list=%v", empty)
	}

	if err := st.PutIoTRetainedMessage(a1, region, "lab/one", []byte("alpha"), 1); err != nil {
		t.Fatal(err)
	}
	if err := st.PutIoTRetainedMessage(a1, region, "lab/two", []byte("beta"), 0); err != nil {
		t.Fatal(err)
	}
	if err := st.PutIoTRetainedMessage(a2, region, "lab/one", []byte("other"), 1); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListIoTRetainedMessages(a1, region)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("list a1 len=%d got=%v", len(got), got)
	}
	byTopic := map[string]store.IoTRetainedMessage{}
	for _, m := range got {
		byTopic[m.Topic] = m
		if m.LastModified <= 0 {
			t.Fatalf("missing lastModified on %s", m.Topic)
		}
	}
	one := byTopic["lab/one"]
	if !bytes.Equal(one.Payload, []byte("alpha")) || one.QoS != 1 {
		t.Fatalf("lab/one %+v", one)
	}
	two := byTopic["lab/two"]
	if !bytes.Equal(two.Payload, []byte("beta")) || two.QoS != 0 {
		t.Fatalf("lab/two %+v", two)
	}

	other, err := st.ListIoTRetainedMessages(a2, region)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].Topic != "lab/one" || !bytes.Equal(other[0].Payload, []byte("other")) {
		t.Fatalf("list a2=%v", other)
	}

	if err := st.PutIoTRetainedMessage(a1, region, "lab/one", []byte("alpha2"), 2); err != nil {
		t.Fatal(err)
	}
	got, err = st.ListIoTRetainedMessages(a1, region)
	if err != nil {
		t.Fatal(err)
	}
	byTopic = map[string]store.IoTRetainedMessage{}
	for _, m := range got {
		byTopic[m.Topic] = m
	}
	if !bytes.Equal(byTopic["lab/one"].Payload, []byte("alpha2")) || byTopic["lab/one"].QoS != 2 {
		t.Fatalf("upsert lab/one %+v", byTopic["lab/one"])
	}

	if err := st.PutIoTRetainedMessage(a1, region, "lab/two", nil, 0); err != nil {
		t.Fatal(err)
	}
	got, err = st.ListIoTRetainedMessages(a1, region)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Topic != "lab/one" {
		t.Fatalf("after clear list=%v", got)
	}

	if err := st.PutIoTRetainedMessage(a1, region, "", []byte("x"), 0); err == nil || !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty topic err=%v", err)
	}

	gotOne, found, err := st.GetIoTRetainedMessage(a1, region, "lab/one")
	if err != nil || !found {
		t.Fatalf("get lab/one found=%v err=%v", found, err)
	}
	if !bytes.Equal(gotOne.Payload, []byte("alpha2")) || gotOne.QoS != 2 {
		t.Fatalf("get lab/one %+v", gotOne)
	}
	_, found, err = st.GetIoTRetainedMessage(a1, region, "lab/two")
	if err != nil || found {
		t.Fatalf("cleared lab/two found=%v err=%v", found, err)
	}
	_, found, err = st.GetIoTRetainedMessage(a2, region, "lab/one")
	if err != nil || !found {
		t.Fatalf("get a2 lab/one found=%v err=%v", found, err)
	}
	_, found, err = st.GetIoTRetainedMessage(a1, region, "missing/topic")
	if err != nil || found {
		t.Fatalf("missing found=%v err=%v", found, err)
	}
	_, _, err = st.GetIoTRetainedMessage(a1, region, "")
	if err == nil || !errors.Is(err, store.ErrIoTBadRequest) {
		t.Fatalf("empty topic get err=%v", err)
	}
}
