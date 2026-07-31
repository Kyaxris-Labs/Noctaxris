package server

import "testing"

func TestParseELBv2LabMuxListenerPath(t *testing.T) {
	tests := []struct {
		prefix string
		path   string
		ok     bool
		acct   string
		lb     string
		port   int
		route  string
	}{
		{"alb", "/alb/123456789012/my-lb/443", true, "123456789012", "my-lb", 443, "/"},
		{"alb", "/alb/123456789012/my-lb/443/foo/bar", true, "123456789012", "my-lb", 443, "/foo/bar"},
		{"nlb", "/nlb/123456789012/nlb-1/80/", true, "123456789012", "nlb-1", 80, "/"},
		{"alb", "/nlb/123456789012/my-lb/443", false, "", "", 0, ""},
		{"alb", "/alb//my-lb/443", false, "", "", 0, ""},
	}
	for _, tc := range tests {
		gotOK := isELBv2LabMuxListenerPath(tc.prefix, tc.path)
		if gotOK != tc.ok {
			t.Errorf("isELBv2LabMuxListenerPath(%q, %q) = %v want %v", tc.prefix, tc.path, gotOK, tc.ok)
		}
		acct, lb, port, route, ok := parseELBv2LabMuxListenerPath(tc.prefix, tc.path)
		if ok != tc.ok {
			t.Errorf("parse ok=%v want %v path=%q prefix=%q", ok, tc.ok, tc.path, tc.prefix)
			continue
		}
		if !tc.ok {
			continue
		}
		if acct != tc.acct || lb != tc.lb || port != tc.port || route != tc.route {
			t.Errorf("parse(%q,%q) = (%q,%q,%d,%q) want (%q,%q,%d,%q)",
				tc.prefix, tc.path, acct, lb, port, route, tc.acct, tc.lb, tc.port, tc.route)
		}
	}
}

func TestELBv2LabMuxWrappersMatchPrefix(t *testing.T) {
	path := "/alb/111122223333/web/8080/api"
	if !isELBv2LabListenerPath(path) {
		t.Fatal("alb wrapper is path")
	}
	a, lb, p, r, ok := parseELBv2LabListenerPath(path)
	if !ok || a != "111122223333" || lb != "web" || p != 8080 || r != "/api" {
		t.Fatalf("alb parse: %q %q %d %q %v", a, lb, p, r, ok)
	}
	npath := "/nlb/111122223333/web/8080/api"
	if !isELBv2NLBLabListenerPath(npath) {
		t.Fatal("nlb wrapper is path")
	}
}
