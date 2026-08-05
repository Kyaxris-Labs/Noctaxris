package acm_test

import (
	"encoding/json"
	"testing"

	acmsvc "github.com/Kyaxris-Labs/Noctaxris/internal/services/acm"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestACMJSONFormatters(t *testing.T) {
	c := store.ACMCertificate{
		CertificateARN: "arn:aws:acm:us-east-1:1:certificate/abc",
		DomainName:     "lab.example.com",
		Status:         "ISSUED",
	}
	reqRaw, err := acmsvc.RequestCertificateJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var reqOut map[string]any
	if err := json.Unmarshal(reqRaw, &reqOut); err != nil {
		t.Fatal(err)
	}
	if reqOut["CertificateArn"] != c.CertificateARN {
		t.Fatalf("request=%v", reqOut)
	}

	descRaw, err := acmsvc.DescribeCertificateJSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var descOut map[string]any
	if err := json.Unmarshal(descRaw, &descOut); err != nil {
		t.Fatal(err)
	}
	cert, _ := descOut["Certificate"].(map[string]any)
	if cert["DomainName"] != c.DomainName || cert["Type"] != "AMAZON_ISSUED" {
		t.Fatalf("describe=%v", descOut)
	}

	listRaw, err := acmsvc.ListCertificatesJSON([]store.ACMCertificate{c, {}})
	if err != nil {
		t.Fatal(err)
	}
	var listOut map[string]any
	if err := json.Unmarshal(listRaw, &listOut); err != nil {
		t.Fatal(err)
	}
	summaries, _ := listOut["CertificateSummaryList"].([]any)
	if len(summaries) != 2 {
		t.Fatalf("list=%v", listOut)
	}

	delRaw, err := acmsvc.DeleteCertificateJSON()
	if err != nil || string(delRaw) != "{}" {
		t.Fatalf("delete=%s err=%v", delRaw, err)
	}
}
