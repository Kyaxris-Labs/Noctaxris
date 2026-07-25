package store_test

import (
	"encoding/json"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestLineFromFirehoseRecordDataJSONAndV2(t *testing.T) {
	account := "000000000001"
	v2 := store.FormatVPCFlowLogV2Line(store.VPCFlowRecord{
		Version: 2, AccountID: account, InterfaceID: "eni-x",
		SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 1, DstPort: 2,
		Protocol: 6, Packets: 1, Bytes: 1, Start: 1, End: 2,
		Action: "ACCEPT", LogStatus: "OK",
	})
	got, err := store.LineFromFirehoseRecordData(account, []byte(v2))
	if err != nil || got != v2 {
		t.Fatalf("v2 passthrough got=%q err=%v", got, err)
	}

	raw, _ := json.Marshal(store.VPCFlowRecord{
		Version: 2, InterfaceID: "eni-y", SrcAddr: "10.1.0.1", DstAddr: "10.1.0.2",
		SrcPort: 80, DstPort: 443, Protocol: 6, Packets: 2, Bytes: 128,
		Start: 100, End: 101, Action: "REJECT", LogStatus: "OK",
	})
	got, err = store.LineFromFirehoseRecordData(account, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || got[0] != '2' {
		t.Fatalf("json line=%q", got)
	}
}
