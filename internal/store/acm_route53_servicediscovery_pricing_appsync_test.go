package store_test

import (
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestACMRequestDescribeListDelete(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	cert, err := st.RequestACMCertificate(account, "us-east-1", "lab.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if cert.Status != "ISSUED" || !strings.Contains(cert.CertPEM, "BEGIN CERTIFICATE") {
		t.Fatalf("cert=%#v", cert)
	}
	got, err := st.DescribeACMCertificate(account, cert.CertificateARN)
	if err != nil || got.DomainName != "lab.example.com" {
		t.Fatalf("describe: %v %#v", err, got)
	}
	list, err := st.ListACMCertificates(account)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %#v", err, list)
	}
	if err := st.DeleteACMCertificate(account, cert.CertificateARN); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DescribeACMCertificate(account, cert.CertificateARN); err != store.ErrACMNotFound {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestRoute53ZoneAndRecords(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	zone, err := st.CreateRoute53HostedZone(account, "lab.example.com", "ref-1", false)
	if err != nil {
		t.Fatal(err)
	}
	err = st.ChangeRoute53ResourceRecordSets(account, zone.ID, []store.Route53Change{
		{Action: "CREATE", Name: "www.lab.example.com", Type: "A", TTL: 60, Records: []string{"127.0.0.1"}},
		{Action: "UPSERT", Name: "alias.lab.example.com", Type: "CNAME", TTL: 60, Records: []string{"www.lab.example.com."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sets, err := st.ListRoute53ResourceRecordSets(account, zone.ID)
	if err != nil || len(sets) != 2 {
		t.Fatalf("list: %v %#v", err, sets)
	}
	if err := st.DeleteRoute53HostedZone(account, zone.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceDiscoveryRegisterDiscover(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	ns, err := st.CreateSDNamespace(account, "us-east-1", "lab.local", "DNS_PRIVATE", "", "vpc-lab1")
	if err != nil {
		t.Fatal(err)
	}
	if ns.Vpc != "vpc-lab1" {
		t.Fatalf("vpc=%q", ns.Vpc)
	}
	svc, err := st.CreateSDService(account, "us-east-1", ns.ID, "api", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.RegisterSDInstance(account, svc.ID, "i-1", map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DiscoverSDInstances(account, "lab.local", "api", ""); err == nil {
		t.Fatal("expected Vpc required for private DNS discover")
	}
	wrong, err := st.DiscoverSDInstances(account, "lab.local", "api", "vpc-other")
	if err != nil || len(wrong) != 0 {
		t.Fatalf("wrong vpc should yield empty: %v %#v", err, wrong)
	}
	found, err := st.DiscoverSDInstances(account, "lab.local", "api", "vpc-lab1")
	if err != nil || len(found) != 1 || found[0].Attributes["AWS_INSTANCE_IPV4"] != "10.0.0.5" {
		t.Fatalf("discover: %v %#v", err, found)
	}
	if err := st.DeregisterSDInstance(account, svc.ID, "i-1"); err != nil {
		t.Fatal(err)
	}
}

func TestPricingStaticCatalog(t *testing.T) {
	st := openTestStore(t)
	services, err := st.DescribePricingServices("")
	if err != nil || len(services) < 2 {
		t.Fatalf("describe: %v %#v", err, services)
	}
	vals, err := st.GetPricingAttributeValues("AmazonEC2", "instanceType")
	if err != nil || len(vals) < 1 {
		t.Fatalf("attrs: %v %#v", err, vals)
	}
	products, err := st.GetPricingProducts("AmazonEC2", map[string]string{"instanceType": "t3.micro"})
	if err != nil || len(products) != 1 || !strings.Contains(products[0], "t3.micro") {
		t.Fatalf("products: %v %#v", err, products)
	}
}

func TestAppSyncAPIKeyAndResolver(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	api, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "lab-api", store.AppSyncAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "bad", "AMAZON_COGNITO_USER_POOLS"); err == nil {
		t.Fatal("expected cognito reject")
	}
	if err := st.StartAppSyncSchemaCreation(account, api.APIID, "type Query { hello: String }"); err != nil {
		t.Fatal(err)
	}
	key, err := st.CreateAppSyncAPIKey(account, api.APIID, 0)
	if err != nil || !strings.HasPrefix(key.APIKey, "da2-") {
		t.Fatalf("api key: %v %#v", err, key)
	}
	gotAcct, gotAPI, err := st.LookupAppSyncAPIKey(key.APIKey)
	if err != nil || gotAcct != account || gotAPI != api.APIID {
		t.Fatalf("lookup plaintext key: acct=%s api=%s err=%v", gotAcct, gotAPI, err)
	}
	if _, _, err := st.LookupAppSyncAPIKey("da2-" + strings.Repeat("00", 16)); err == nil {
		t.Fatal("lookup with wrong key should fail")
	}
	_, err = st.CreateAppSyncDataSource(account, api.APIID, "HelloFn", "AWS_LAMBDA",
		"arn:aws:lambda:us-east-1:"+account+":function:hello", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAppSyncResolver(account, api.APIID, "Query", "hello", "HelloFn")
	if err != nil {
		t.Fatal(err)
	}
	field, err := store.ParseAppSyncQueryField("query { hello }")
	if err != nil || field != "hello" {
		t.Fatalf("parse: %v %q", err, field)
	}
	_, ds, err := st.ResolveAppSyncQueryField(account, api.APIID, "hello")
	if err != nil || !strings.Contains(ds.LambdaFunctionARN, "function:hello") {
		t.Fatalf("resolve: %v %#v", err, ds)
	}
	iamAPI, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "iam-api", store.AppSyncAuthIAM)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAppSyncAPIKey(account, iamAPI.APIID, 0); err == nil {
		t.Fatal("expected api key reject on IAM api")
	}
}
