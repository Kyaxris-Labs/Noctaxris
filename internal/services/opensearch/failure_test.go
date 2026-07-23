package opensearch

import (
	"strings"
	"testing"
)

func TestClassifyOpenSearchNestedFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		evidence  string
		wantSub   string
		wantEmpty bool
	}{
		{
			name:      "empty",
			evidence:  "",
			wantEmpty: true,
		},
		{
			name: "max_map_count bootstrap",
			evidence: "[ERROR][o.o.b.BootstrapChecks] max virtual memory areas " +
				"vm.max_map_count [65530] is too low, increase to at least [262144]",
			wantSub: "vm.max_map_count",
		},
		{
			name:     "memory lock",
			evidence: "bootstrap check failure [memory lock]: memory locking requested but not available",
			wantSub:  "memory lock",
		},
		{
			name:     "mlockall",
			evidence: "Unable to lock JVM Memory: error=12, reason=Cannot allocate memory (mlockall)",
			wantSub:  "memory lock",
		},
		{
			name:     "generic wait",
			evidence: "compute: data-plane healthy wait: context deadline exceeded",
			wantSub:  "CreateFailed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyOpenSearchNestedFailure(tc.evidence)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("got %q want empty", got)
				}
				return
			}
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.wantSub)) {
				t.Fatalf("got %q want substring %q", got, tc.wantSub)
			}
			if tc.wantSub == "CreateFailed" && !strings.Contains(got, "does not change host sysctl") {
				t.Fatalf("generic hint missing fail-closed sysctl note: %q", got)
			}
		})
	}
}
