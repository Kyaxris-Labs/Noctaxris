package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestVPCFlowCreateAndInjectS3(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	if _, err := st.CreateBucket(account, "flow-logs-bucket"); err != nil {
		t.Fatal(err)
	}
	fl, err := st.CreateVPCFlowLog(account, store.CreateVPCFlowLogInput{
		ResourceIDs:        []string{"vpc-0abc123def456789"},
		ResourceType:       "VPC",
		TrafficType:        "ALL",
		LogDestinationType: "s3",
		LogDestination:     "arn:aws:s3:::flow-logs-bucket/prefix",
		Region:             "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(fl.FlowLogID, "fl-") {
		t.Fatalf("flowLogId=%q", fl.FlowLogID)
	}
	at := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	n, err := st.InjectVPCFlowLogs(account, fl.FlowLogID, nil, at)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("delivered=%d want 2 sample lines", n)
	}
	listed, err := st.ListObjectsV2(account, "flow-logs-bucket", "prefix/AWSLogs/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Contents) == 0 {
		t.Fatal("expected S3 flow log object")
	}
	_, body, err := st.GetObject(account, "flow-logs-bucket", listed.Contents[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, " ACCEPT OK") || !strings.Contains(text, " REJECT OK") {
		t.Fatalf("body missing ACCEPT/REJECT samples: %q", text)
	}
	if !strings.HasPrefix(strings.TrimSpace(strings.Split(text, "\n")[0]), "2 ") {
		t.Fatalf("first line not v2 format: %q", text)
	}
}

func TestVPCFlowInjectCloudWatchLogs(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	group := "/vpc/flow"
	if _, err := st.CreateLogGroup(account, "us-east-1", group); err != nil {
		t.Fatal(err)
	}
	logARN := "arn:aws:logs:us-east-1:" + account + ":log-group:" + group
	fl, err := st.CreateVPCFlowLog(account, store.CreateVPCFlowLogInput{
		ResourceIDs:        []string{"eni-0abc123def456789"},
		ResourceType:       "NetworkInterface",
		LogDestinationType: "cloud-watch-logs",
		LogDestination:     logARN,
		Region:             "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.InjectVPCFlowLogs(account, fl.FlowLogID, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("delivered=%d", n)
	}
	stream := store.VPCFlowLabLogStreamName(fl.FlowLogID)
	events, err := st.GetLogEvents(account, group, stream, 0, 0, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatalf("events=%d", len(events))
	}
	combined := events[0].Message + events[1].Message
	if !strings.Contains(combined, "ACCEPT") || !strings.Contains(combined, "REJECT") {
		t.Fatalf("messages=%v", events)
	}
}

func TestFormatVPCFlowLogV2Line(t *testing.T) {
	line := store.FormatVPCFlowLogV2Line(store.VPCFlowRecord{
		Version: 2, AccountID: "123456789012", InterfaceID: "eni-abc",
		SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 1234, DstPort: 443,
		Protocol: 6, Packets: 1, Bytes: 100, Start: 1000, End: 1060,
		Action: "ACCEPT", LogStatus: "OK",
	})
	want := "2 123456789012 eni-abc 10.0.0.1 10.0.0.2 1234 443 6 1 100 1000 1060 ACCEPT OK"
	if line != want {
		t.Fatalf("line=%q want %q", line, want)
	}
}
