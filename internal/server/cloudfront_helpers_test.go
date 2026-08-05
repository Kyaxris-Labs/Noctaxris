package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCloudFrontInvalidationAndUpdateHelpers(t *testing.T) {
	srv, st, _ := newTestServerStore(t)
	handler := srv.Handler()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateBucket(testAccountID, "cf-cov-bucket"); err != nil {
		t.Fatal(err)
	}

	create := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateDistribution", "cloudfront", map[string]any{
		"DistributionConfig": map[string]any{
			"CallerReference": "cf-cov-ref",
			"Comment":         "cov",
			"Enabled":         true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "cf-cov-bucket", "OriginType": "s3", "S3OriginConfig": map[string]any{}},
				},
			},
			"DefaultCacheBehavior": map[string]any{
				"TargetOriginId": "o1",
			},
			"CacheBehaviors": map[string]any{
				"Items": []map[string]any{
					{"PathPattern": "/api/*", "TargetOriginId": "o1"},
				},
			},
		},
	}, now)
	if create.Code != http.StatusOK {
		t.Fatalf("CreateDistribution status=%d body=%q", create.Code, create.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	dist, _ := created["Distribution"].(map[string]any)
	id, _ := dist["Id"].(string)
	etag, _ := dist["ETag"].(string)
	if id == "" {
		t.Fatalf("missing distribution id: %s", create.Body.String())
	}

	cfg := mustJSONTarget(t, handler, "CloudFront_2016_01_28.GetDistributionConfig", "cloudfront", map[string]any{
		"Id": id,
	}, now)
	if cfg.Code != http.StatusOK {
		t.Fatalf("GetDistributionConfig status=%d body=%q", cfg.Code, cfg.Body.String())
	}
	if etag == "" {
		etag = cfg.Header().Get("ETag")
	}

	updBadMatch := mustJSONTarget(t, handler, "CloudFront_2016_01_28.UpdateDistribution", "cloudfront", map[string]any{
		"Id":      id,
		"IfMatch": "wrong-etag",
		"DistributionConfig": map[string]any{
			"Comment": "changed",
			"Enabled": true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "cf-cov-bucket", "OriginType": "s3"},
				},
			},
			"DefaultCacheBehavior": map[string]any{
				"TargetOriginId": "o1",
			},
		},
	}, now)
	if updBadMatch.Code != http.StatusBadRequest || !strings.Contains(updBadMatch.Body.String(), "InvalidIfMatchVersion") {
		t.Fatalf("UpdateDistribution bad IfMatch want InvalidIfMatchVersion status=%d body=%q", updBadMatch.Code, updBadMatch.Body.String())
	}

	upd := mustJSONTarget(t, handler, "CloudFront_2016_01_28.UpdateDistribution", "cloudfront", map[string]any{
		"Id":      id,
		"IfMatch": etag,
		"DistributionConfig": map[string]any{
			"Comment": "changed",
			"Enabled": true,
			"Origins": map[string]any{
				"Items": []map[string]any{
					{"Id": "o1", "DomainName": "cf-cov-bucket", "OriginType": "s3"},
				},
			},
			"DefaultCacheBehavior": map[string]any{
				"TargetOriginId": "o1",
			},
			"CacheBehaviors": map[string]any{
				"Items": []map[string]any{
					{"PathPattern": "/v2/*", "TargetOriginId": "o1"},
				},
			},
		},
	}, now)
	if upd.Code != http.StatusOK {
		t.Fatalf("UpdateDistribution status=%d body=%q", upd.Code, upd.Body.String())
	}

	invMissing := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateInvalidation", "cloudfront", map[string]any{
		"DistributionId": "EDOESNOTEXIST",
		"InvalidationBatch": map[string]any{
			"CallerReference": "inv-missing",
			"Paths": map[string]any{
				"Items": []any{"/*"},
			},
		},
	}, now)
	if invMissing.Code != http.StatusNotFound {
		t.Fatalf("CreateInvalidation missing dist want 404 status=%d body=%q", invMissing.Code, invMissing.Body.String())
	}

	inv := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateInvalidation", "cloudfront", map[string]any{
		"DistributionId": id,
		"InvalidationBatch": map[string]any{
			"CallerReference": "inv-1",
			"Paths": map[string]any{
				"Items": []any{"/index.html", map[string]any{"Path": "/assets/*"}},
			},
		},
	}, now)
	if inv.Code != http.StatusOK {
		t.Fatalf("CreateInvalidation status=%d body=%q", inv.Code, inv.Body.String())
	}
	var invOut map[string]any
	_ = json.Unmarshal(inv.Body.Bytes(), &invOut)
	invalidation, _ := invOut["Invalidation"].(map[string]any)
	invID, _ := invalidation["Id"].(string)
	if invID == "" {
		t.Fatalf("missing invalidation id: %s", inv.Body.String())
	}

	invFlat := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateInvalidation", "cloudfront", map[string]any{
		"DistributionId": id,
		"InvalidationBatch": map[string]any{
			"CallerReference": "inv-2",
			"Paths":           []any{"/flat/*"},
		},
	}, now)
	if invFlat.Code != http.StatusOK {
		t.Fatalf("CreateInvalidation flat paths status=%d body=%q", invFlat.Code, invFlat.Body.String())
	}

	getInv := mustJSONTarget(t, handler, "CloudFront_2016_01_28.GetInvalidation", "cloudfront", map[string]any{
		"DistributionId": id,
		"Id":             invID,
	}, now)
	if getInv.Code != http.StatusOK || !strings.Contains(getInv.Body.String(), invID) {
		t.Fatalf("GetInvalidation status=%d body=%q", getInv.Code, getInv.Body.String())
	}

	getInvMissing := mustJSONTarget(t, handler, "CloudFront_2016_01_28.GetInvalidation", "cloudfront", map[string]any{
		"DistributionId": id,
		"Id":             "IXXXXXXXX",
	}, now)
	if getInvMissing.Code != http.StatusNotFound || !strings.Contains(getInvMissing.Body.String(), "NoSuchInvalidation") {
		t.Fatalf("GetInvalidation missing want NoSuchInvalidation status=%d body=%q", getInvMissing.Code, getInvMissing.Body.String())
	}

	listInv := mustJSONTarget(t, handler, "CloudFront_2016_01_28.ListInvalidations", "cloudfront", map[string]any{
		"DistributionId": id,
	}, now)
	if listInv.Code != http.StatusOK || !strings.Contains(listInv.Body.String(), invID) {
		t.Fatalf("ListInvalidations status=%d body=%q", listInv.Code, listInv.Body.String())
	}

	unknown := mustJSONTarget(t, handler, "CloudFront_2016_01_28.CreateStreamingDistribution", "cloudfront", map[string]any{}, now)
	if unknown.Code != http.StatusNotImplemented || !strings.Contains(unknown.Body.String(), "InvalidAction") {
		t.Fatalf("unknown CloudFront action want InvalidAction status=%d body=%q", unknown.Code, unknown.Body.String())
	}
}
