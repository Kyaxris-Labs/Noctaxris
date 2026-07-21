package store_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func openLambdaStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	key, err := store.LoadOrCreateMasterKey(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLambdaCreateGetUpdateCodeDelete(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip1 := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	sum1 := sha256.Sum256(zip1)

	created, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "hello-world",
		RoleARN:      "arn:aws:iam::000000000001:role/lambda-exec",
		Runtime:      store.LambdaRuntimePython312,
		Handler:      "app.handler",
		Timeout:      10,
		Memory:       256,
		Env:          map[string]string{"STAGE": "lab"},
		Description:  "lab fn",
		Zip:          zip1,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantARN := "arn:aws:lambda:us-east-1:000000000001:function:hello-world"
	if created.FunctionARN != wantARN {
		t.Fatalf("arn=%q want %q", created.FunctionARN, wantARN)
	}
	if created.CodeSHA256 != hex.EncodeToString(sum1[:]) {
		t.Fatalf("sha=%q", created.CodeSHA256)
	}
	if created.State != store.LambdaStateActive {
		t.Fatalf("state=%q", created.State)
	}
	absZip := filepath.Join(st.DataRoot(), created.CodePath)
	if _, err := os.Stat(absZip); err != nil {
		t.Fatalf("missing zip %s: %v", absZip, err)
	}
	codeDir := filepath.Join(st.DataRoot(), "lambda", account, "hello-world", "code")
	appPath := filepath.Join(codeDir, "app.py")
	if _, err := os.Stat(appPath); err != nil {
		t.Fatalf("missing unpacked app.py: %v", err)
	}
	// Nested DinD must traverse API-owned trees (UID 65532); private modes break Invoke.
	for _, p := range []string{
		filepath.Join(st.DataRoot(), "lambda"),
		filepath.Join(st.DataRoot(), "lambda", account),
		filepath.Join(st.DataRoot(), "lambda", account, "hello-world"),
		codeDir,
	} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm()&0o005 == 0 {
			t.Fatalf("%s mode=%o want other-executable", p, info.Mode().Perm())
		}
	}
	appInfo, err := os.Stat(appPath)
	if err != nil {
		t.Fatal(err)
	}
	if appInfo.Mode().Perm()&0o004 == 0 {
		t.Fatalf("app.py mode=%o want other-readable", appInfo.Mode().Perm())
	}

	got, err := st.GetFunction(account, "hello-world")
	if err != nil {
		t.Fatal(err)
	}
	if got.Handler != "app.handler" || got.Env["STAGE"] != "lab" || got.Timeout != 10 {
		t.Fatalf("got=%+v", got)
	}

	zip2 := testZip(t, map[string]string{"app.py": "def handler(e,c): return {'ok': True}"})
	sum2 := sha256.Sum256(zip2)
	updated, err := st.UpdateFunctionCode(account, "hello-world", zip2)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CodeSHA256 != hex.EncodeToString(sum2[:]) {
		t.Fatalf("updated sha=%q", updated.CodeSHA256)
	}
	data, err := os.ReadFile(filepath.Join(st.DataRoot(), updated.CodePath))
	if err != nil {
		t.Fatal(err)
	}
	if !bytesEqual(data, zip2) {
		t.Fatalf("zip on disk mismatch")
	}

	cfg, err := st.UpdateFunctionConfiguration(account, "hello-world", store.UpdateFunctionConfigurationMeta{
		RoleARN: "arn:aws:iam::000000000001:role/other",
		Timeout: 30,
		Memory:  512,
		Handler: "app.other",
		Env:     map[string]string{"STAGE": "prod"},
		Runtime: store.LambdaRuntimePython312,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RoleARN != "arn:aws:iam::000000000001:role/other" || cfg.Timeout != 30 || cfg.Memory != 512 || cfg.Handler != "app.other" || cfg.Env["STAGE"] != "prod" {
		t.Fatalf("cfg=%+v", cfg)
	}

	if err := st.DeleteFunction(account, "hello-world"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetFunction(account, "hello-world"); !errors.Is(err, store.ErrNoSuchFunction) {
		t.Fatalf("want ErrNoSuchFunction, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.DataRoot(), "lambda", account, "hello-world")); !os.IsNotExist(err) {
		t.Fatalf("want dir removed, err=%v", err)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLambdaListAndRejectDuplicate(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"a.py": "x=1"})

	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "fn-a",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "a.handler", Timeout: 3, Memory: 128, Zip: zip,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "fn-b",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "b.handler", Timeout: 3, Memory: 128, Zip: zip,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "fn-a",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "a.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if !errors.Is(err, store.ErrFunctionAlreadyExists) {
		t.Fatalf("want ErrFunctionAlreadyExists, got %v", err)
	}

	list, err := st.ListFunctions(account)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list=%+v", list)
	}
	if list[0].FunctionName != "fn-a" || list[1].FunctionName != "fn-b" {
		t.Fatalf("order=%+v", list)
	}
}

func TestLambdaInvalidFunctionName(t *testing.T) {
	st := openLambdaStore(t)
	zip := testZip(t, map[string]string{"a.py": "x=1"})
	_, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: "000000000001", Region: "us-east-1", FunctionName: "1bad",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: "python3.12",
		Handler: "a.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if !errors.Is(err, store.ErrInvalidFunctionName) {
		t.Fatalf("want ErrInvalidFunctionName, got %v", err)
	}
	if err := store.ValidateFunctionName("ok_name-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateFunctionName(""); !errors.Is(err, store.ErrInvalidFunctionName) {
		t.Fatalf("empty: %v", err)
	}
}

func TestFunctionARN(t *testing.T) {
	got := store.FunctionARN("000000000001", "us-west-2", "demo")
	want := "arn:aws:lambda:us-west-2:000000000001:function:demo"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if store.FunctionARN("000000000001", "", "demo") != "arn:aws:lambda:us-east-1:000000000001:function:demo" {
		t.Fatal("default region")
	}
}

func createTestFunction(t *testing.T, st *store.Store, account, name string, zip []byte) store.LambdaFunction {
	t.Helper()
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: name,
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: zip,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fn
}

func TestLambdaPublishVersionFreezesSnapshot(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip1 := testZip(t, map[string]string{"app.py": "v=1"})
	createTestFunction(t, st, account, "snap-fn", zip1)

	v1, err := st.PublishVersion(account, "snap-fn")
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 {
		t.Fatalf("version=%d want 1", v1.Version)
	}
	if v1.Handler != "app.handler" {
		t.Fatalf("handler=%q", v1.Handler)
	}
	wantARN := "arn:aws:lambda:us-east-1:000000000001:function:snap-fn:1"
	if v1.VersionARN != wantARN {
		t.Fatalf("version arn=%q want %q", v1.VersionARN, wantARN)
	}

	zip2 := testZip(t, map[string]string{"app.py": "v=2"})
	if _, err := st.UpdateFunctionCode(account, "snap-fn", zip2); err != nil {
		t.Fatal(err)
	}

	resolved, executed, err := st.ResolveFunction(account, "snap-fn", "1")
	if err != nil {
		t.Fatal(err)
	}
	if executed != "1" {
		t.Fatalf("executed=%q want 1", executed)
	}
	if resolved.CodeSHA256 != v1.CodeSHA256 {
		t.Fatalf("version 1 code changed after $LATEST update: got %q want %q", resolved.CodeSHA256, v1.CodeSHA256)
	}

	latest, executed, err := st.ResolveFunction(account, "snap-fn", "$LATEST")
	if err != nil {
		t.Fatal(err)
	}
	if executed != "$LATEST" {
		t.Fatalf("executed=%q want $LATEST", executed)
	}
	if latest.CodeSHA256 == v1.CodeSHA256 {
		t.Fatal("$LATEST should differ from frozen version 1 after code update")
	}

	v2, err := st.PublishVersion(account, "snap-fn")
	if err != nil {
		t.Fatal(err)
	}
	if v2.Version != 2 {
		t.Fatalf("version=%d want 2", v2.Version)
	}
}

func TestLambdaCreateAliasResolveAndGetByQualifier(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	zip := testZip(t, map[string]string{"app.py": "x=1"})
	createTestFunction(t, st, account, "alias-fn", zip)

	v1, err := st.PublishVersion(account, "alias-fn")
	if err != nil {
		t.Fatal(err)
	}

	alias, err := st.CreateLambdaAlias(account, "alias-fn", "prod", v1.Version, "")
	if err != nil {
		t.Fatal(err)
	}
	if alias.AliasName != "prod" || alias.FunctionVersion != 1 {
		t.Fatalf("alias=%+v", alias)
	}
	wantAliasARN := "arn:aws:lambda:us-east-1:000000000001:function:alias-fn:prod"
	if alias.AliasARN != wantAliasARN {
		t.Fatalf("alias arn=%q want %q", alias.AliasARN, wantAliasARN)
	}

	resolved, executed, err := st.ResolveFunction(account, "alias-fn", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if executed != "1" {
		t.Fatalf("executed=%q want 1", executed)
	}
	if resolved.CodeSHA256 != v1.CodeSHA256 {
		t.Fatalf("alias resolved wrong code sha")
	}

	got, err := st.GetFunctionByQualifier(account, "alias-fn", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "prod" {
		t.Fatalf("version field=%q want prod", got.Version)
	}

	byVersion, err := st.GetFunctionByQualifier(account, "alias-fn", "1")
	if err != nil {
		t.Fatal(err)
	}
	if byVersion.Version != "1" {
		t.Fatalf("version field=%q want 1", byVersion.Version)
	}
}

func TestLambdaParseFunctionQualifier(t *testing.T) {
	cases := []struct {
		raw, wantName, wantQual string
	}{
		{"hello", "hello", "$LATEST"},
		{"hello:1", "hello", "1"},
		{"hello:prod", "hello", "prod"},
		{"arn:aws:lambda:us-east-1:000000000001:function:hello", "hello", "$LATEST"},
		{"arn:aws:lambda:us-east-1:000000000001:function:hello:2", "hello", "2"},
		{"arn:aws:lambda:us-east-1:000000000001:function:hello:prod", "hello", "prod"},
	}
	for _, tc := range cases {
		name, qual := store.ParseFunctionQualifier(tc.raw)
		if name != tc.wantName || qual != tc.wantQual {
			t.Fatalf("ParseFunctionQualifier(%q) = (%q,%q) want (%q,%q)", tc.raw, name, qual, tc.wantName, tc.wantQual)
		}
	}
}

func TestLambdaPublishLayerAttachAndResolve(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	layerZip := testZip(t, map[string]string{"python/layerlib.py": "LAYER_VALUE='from-layer'\n"})
	layer, err := st.PublishLayerVersion(store.PublishLayerVersionMeta{
		AccountID: account,
		Region:    "us-east-1",
		LayerName: "shared-lib",
		Zip:       layerZip,
	})
	if err != nil {
		t.Fatal(err)
	}
	if layer.Version != 1 {
		t.Fatalf("version=%d want 1", layer.Version)
	}
	wantLayerARN := "arn:aws:lambda:us-east-1:000000000001:layer:shared-lib:1"
	if layer.LayerARN != wantLayerARN {
		t.Fatalf("layer arn=%q want %q", layer.LayerARN, wantLayerARN)
	}
	if _, err := os.Stat(filepath.Join(st.DataRoot(), "lambda", account, "layers", "shared-lib", "versions", "1", "code", "python", "layerlib.py")); err != nil {
		t.Fatalf("missing unpacked layer file: %v", err)
	}

	fnZip := testZip(t, map[string]string{"app.py": "import layerlib\ndef handler(e,c): return {'layer': layerlib.LAYER_VALUE}\n"})
	fn, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "layer-fn",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: fnZip,
		Layers: []string{layer.LayerARN},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fn.Layers) != 1 || fn.Layers[0] != wantLayerARN {
		t.Fatalf("layers=%v want [%q]", fn.Layers, wantLayerARN)
	}

	paths, err := st.ResolveLayerCodeDirs(st.DataRoot(), account, fn.Layers)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths=%v", paths)
	}
	wantPath := store.LayerCodeDirInContainer(st.DataRoot(), account, "shared-lib", 1)
	if paths[0] != wantPath {
		t.Fatalf("path=%q want %q", paths[0], wantPath)
	}

	got, err := st.GetLayerVersion(account, "shared-lib", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.CodeSHA256 != layer.CodeSHA256 {
		t.Fatalf("get layer sha mismatch")
	}
	list, err := st.ListLayerVersions(account, "shared-lib")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.DeleteLayerVersion(account, "shared-lib", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetLayerVersion(account, "shared-lib", 1); !errors.Is(err, store.ErrNoSuchLayer) {
		t.Fatalf("want ErrNoSuchLayer, got %v", err)
	}
}

func TestLambdaLayersMaxAndInvalidARN(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	layerZip := testZip(t, map[string]string{"python/x.py": "x=1"})
	layer, err := st.PublishLayerVersion(store.PublishLayerVersionMeta{
		AccountID: account, Region: "us-east-1", LayerName: "one", Zip: layerZip,
	})
	if err != nil {
		t.Fatal(err)
	}
	arns := make([]string, 0, store.MaxLambdaLayers+1)
	for i := 0; i < store.MaxLambdaLayers+1; i++ {
		arns = append(arns, layer.LayerARN)
	}
	fnZip := testZip(t, map[string]string{"app.py": "def handler(e,c): return e"})
	_, err = st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "too-many",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: fnZip, Layers: arns,
	})
	if !errors.Is(err, store.ErrTooManyLayers) {
		t.Fatalf("want ErrTooManyLayers, got %v", err)
	}
	_, err = st.CreateFunction(store.CreateFunctionMeta{
		AccountID: account, Region: "us-east-1", FunctionName: "bad-arn",
		RoleARN: "arn:aws:iam::000000000001:role/r", Runtime: store.LambdaRuntimePython312,
		Handler: "app.handler", Timeout: 3, Memory: 128, Zip: fnZip,
		Layers: []string{"arn:aws:lambda:us-east-1:999999999999:layer:one:1"},
	})
	if !errors.Is(err, store.ErrInvalidLayerARN) {
		t.Fatalf("want ErrInvalidLayerARN, got %v", err)
	}
}

func TestLambdaCreateFunctionImage(t *testing.T) {
	st := openLambdaStore(t)
	account := "000000000001"
	imageURI := "public.ecr.aws/lambda/python:3.12"

	created, err := st.CreateFunction(store.CreateFunctionMeta{
		AccountID:    account,
		Region:       "us-east-1",
		FunctionName: "img-fn",
		RoleARN:      "arn:aws:iam::000000000001:role/lambda-exec",
		Handler:      "app.handler",
		Timeout:      10,
		Memory:       256,
		PackageType:  store.LambdaPackageTypeImage,
		ImageURI:     imageURI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.PackageType != store.LambdaPackageTypeImage {
		t.Fatalf("PackageType=%q want Image", created.PackageType)
	}
	if created.ImageURI != imageURI {
		t.Fatalf("ImageURI=%q", created.ImageURI)
	}
	if created.CodePath != "" {
		t.Fatalf("CodePath=%q want empty for image", created.CodePath)
	}

	got, err := st.GetFunction(account, "img-fn")
	if err != nil {
		t.Fatal(err)
	}
	if got.PackageType != store.LambdaPackageTypeImage || got.ImageURI != imageURI {
		t.Fatalf("got=%+v", got)
	}

	updated, err := st.UpdateFunctionImageCode(account, "img-fn", "public.ecr.aws/lambda/python:3.12-v2")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ImageURI != "public.ecr.aws/lambda/python:3.12-v2" {
		t.Fatalf("updated ImageURI=%q", updated.ImageURI)
	}

	pub, err := st.PublishVersion(account, "img-fn")
	if err != nil {
		t.Fatal(err)
	}
	if pub.PackageType != store.LambdaPackageTypeImage || pub.ImageURI != updated.ImageURI {
		t.Fatalf("published=%+v", pub)
	}
}
