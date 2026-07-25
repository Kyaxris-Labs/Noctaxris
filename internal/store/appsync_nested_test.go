package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestParseAppSyncQuerySelectionsNested(t *testing.T) {
	sels, err := store.ParseAppSyncQuerySelections("{ parent { child } }")
	if err != nil {
		t.Fatalf("nested: %v", err)
	}
	if len(sels) != 1 || sels[0].Name != "parent" || len(sels[0].Children) != 1 || sels[0].Children[0].Name != "child" {
		t.Fatalf("unexpected selections %#v", sels)
	}

	sels, err = store.ParseAppSyncQuerySelections("query { a { b { c } } d }")
	if err != nil {
		t.Fatalf("depth3+sibling: %v", err)
	}
	if len(sels) != 2 || sels[0].Name != "a" || sels[1].Name != "d" {
		t.Fatalf("top %#v", sels)
	}
	if len(sels[0].Children) != 1 || sels[0].Children[0].Name != "b" {
		t.Fatalf("b %#v", sels[0].Children)
	}
	if len(sels[0].Children[0].Children) != 1 || sels[0].Children[0].Children[0].Name != "c" {
		t.Fatalf("c %#v", sels[0].Children[0].Children)
	}

	if _, err := store.ParseAppSyncQuerySelections("{ a { b { c { d } } } }"); err == nil {
		t.Fatal("expected depth>3 reject")
	} else if !errors.Is(err, store.ErrAppSyncBadRequest) || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("depth err: %v", err)
	}

	if _, err := store.ParseAppSyncQuerySelections("{ hello { } }"); err == nil {
		t.Fatal("expected empty nest reject")
	}

	fields, err := store.ParseAppSyncQueryFields("{ parent { child } sibling }")
	if err != nil || len(fields) != 2 || fields[0] != "parent" || fields[1] != "sibling" {
		t.Fatalf("flat names from nested: %v %#v", err, fields)
	}
}

func TestParseAppSyncSchemaFieldReturnTypes(t *testing.T) {
	sdl := `
type Query { getUser: User! list: [Item!]! }
type User { name: String profile: Profile }
type Profile { bio: String }
type Item { id: ID! }
`
	m := store.ParseAppSyncSchemaFieldReturnTypes(sdl)
	if m["Query"]["getUser"] != "User" {
		t.Fatalf("getUser: %#v", m["Query"])
	}
	if m["Query"]["list"] != "Item" {
		t.Fatalf("list: %#v", m["Query"])
	}
	if m["User"]["profile"] != "Profile" {
		t.Fatalf("profile: %#v", m["User"])
	}
	if m["Profile"]["bio"] != "String" {
		t.Fatalf("bio: %#v", m["Profile"])
	}
}

func TestResolveAppSyncFieldByType(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	api, err := st.CreateAppSyncGraphqlAPI(account, "us-east-1", "nest-api", store.AppSyncAuthAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateAppSyncDataSource(account, api.APIID, "UserDS", "AWS_LAMBDA",
		"arn:aws:lambda:us-east-1:"+account+":function:user", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAppSyncResolver(account, api.APIID, "User", "name", "UserDS"); err != nil {
		t.Fatal(err)
	}
	_, ds, err := st.ResolveAppSyncField(account, api.APIID, "User", "name")
	if err != nil || !strings.Contains(ds.LambdaFunctionARN, "function:user") {
		t.Fatalf("resolve User.name: %v %#v", err, ds)
	}
	if _, _, err := st.ResolveAppSyncField(account, api.APIID, "User", "missing"); !errors.Is(err, store.ErrAppSyncNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
