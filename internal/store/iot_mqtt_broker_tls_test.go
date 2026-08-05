package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestEnsureLabMQTTBrokerMaterial(t *testing.T) {
	st := openIoTStore(t)

	mat, err := st.EnsureLabMQTTBrokerMaterial()
	if err != nil {
		t.Fatal(err)
	}
	if mat.SecretsDir == "" || mat.MosquittoConf == "" || mat.BridgeCertPEM == "" || mat.CACertPEM == "" {
		t.Fatalf("material incomplete: %+v", mat)
	}
	if len(mat.Binds) < 5 {
		t.Fatalf("binds=%v", mat.Binds)
	}
	for _, p := range []string{mat.MosquittoConf, filepath.Join(mat.SecretsDir, "ca.crt"), filepath.Join(mat.SecretsDir, "server.crt")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	conf, err := os.ReadFile(mat.MosquittoConf)
	if err != nil {
		t.Fatal(err)
	}
	body := string(conf)
	if !strings.Contains(body, "require_certificate true") || !strings.Contains(body, "allow_anonymous false") {
		t.Fatalf("mosquitto conf insecure defaults: %s", body)
	}
	for _, b := range mat.Binds {
		if strings.Contains(b, "..") {
			t.Fatalf("bind path traversal risk: %s", b)
		}
		if !strings.HasSuffix(b, ":ro") {
			t.Fatalf("bind must be read-only: %s", b)
		}
	}

	again, err := st.EnsureLabMQTTBrokerMaterial()
	if err != nil {
		t.Fatal(err)
	}
	if again.BridgeCertPEM != mat.BridgeCertPEM || again.CACertPEM != mat.CACertPEM {
		t.Fatal("idempotent ensure changed cert material")
	}
}

func TestEnsureLabMQTTBrokerMaterialNilStore(t *testing.T) {
	var st *store.Store
	_, err := st.EnsureLabMQTTBrokerMaterial()
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("nil store err=%v", err)
	}
}
