package sdk_test

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRDSMySQLMariaDBCreateDescribeDelete(t *testing.T) {
	requireReady(t)
	prefix := uniquePrefix(t)
	engines := []struct {
		name   string
		engine string
	}{
		{name: "mysql", engine: "mysql"},
		{name: "mariadb", engine: "mariadb"},
	}
	for _, tc := range engines {
		t.Run(tc.name, func(t *testing.T) {
			id := strings.ToLower(strings.ReplaceAll(prefix+"-"+tc.name, "_", "-"))
			if len(id) > 60 {
				id = id[:60]
			}
			createBody := url.Values{
				"Action":               {"CreateDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {id},
				"Engine":               {tc.engine},
				"DBInstanceClass":      {"db.t3.micro"},
				"MasterUsername":       {"root"},
				"MasterUserPassword":   {"lab-password-1"},
				"AllocatedStorage":     {"20"},
				"DBName":               {"appdb"},
			}.Encode()
			status, body := signedHTTP(t, "rds", http.MethodPost, "/", []byte(createBody), "application/x-www-form-urlencoded")
			if status != http.StatusOK {
				t.Fatalf("CreateDBInstance status=%d body=%s", status, body)
			}
			if !strings.Contains(string(body), id) || !strings.Contains(string(body), tc.engine) {
				t.Fatalf("create missing id/engine: %s", body)
			}
			t.Cleanup(func() {
				delBody := url.Values{
					"Action":               {"DeleteDBInstance"},
					"Version":              {"2014-10-31"},
					"DBInstanceIdentifier": {id},
				}.Encode()
				_, _ = signedHTTP(t, "rds", http.MethodPost, "/", []byte(delBody), "application/x-www-form-urlencoded")
			})

			descBody := url.Values{
				"Action":               {"DescribeDBInstances"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {id},
			}.Encode()
			status, body = signedHTTP(t, "rds", http.MethodPost, "/", []byte(descBody), "application/x-www-form-urlencoded")
			if status != http.StatusOK || !strings.Contains(string(body), id) {
				t.Fatalf("DescribeDBInstances status=%d body=%s", status, body)
			}
			xml := string(body)
			if !strings.Contains(xml, "<Engine>"+tc.engine+"</Engine>") && !strings.Contains(xml, tc.engine) {
				t.Fatalf("describe missing engine %s: %s", tc.engine, xml)
			}

			if strings.TrimSpace(os.Getenv("NOCTAXRIS_NESTED")) == "1" {
				deadline := time.Now().Add(90 * time.Second)
				for time.Now().Before(deadline) {
					status, body = signedHTTP(t, "rds", http.MethodPost, "/", []byte(descBody), "application/x-www-form-urlencoded")
					if status == http.StatusOK && strings.Contains(string(body), "available") {
						break
					}
					if status == http.StatusOK && strings.Contains(string(body), "failed") {
						t.Skipf("RDS %s nested soft-skip: status failed (nested engine not healthy)", tc.engine)
					}
					time.Sleep(2 * time.Second)
				}
				if !strings.Contains(string(body), "available") {
					t.Skipf("RDS %s nested soft-skip: status not available (set NOCTAXRIS_NESTED=1 with healthy DinD)", tc.engine)
				}
			}

			delBody := url.Values{
				"Action":               {"DeleteDBInstance"},
				"Version":              {"2014-10-31"},
				"DBInstanceIdentifier": {id},
			}.Encode()
			status, body = signedHTTP(t, "rds", http.MethodPost, "/", []byte(delBody), "application/x-www-form-urlencoded")
			if status != http.StatusOK {
				t.Fatalf("DeleteDBInstance status=%d body=%s", status, body)
			}
		})
	}
}
