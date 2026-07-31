package compute

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
)

func TestDirTreeTarForAbsolute(t *testing.T) {
	raw, err := dirTreeTarForAbsolute(CodeBuildWorkspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(raw))
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
	}
	want := []string{"codebuild/", "codebuild/src/"}
	if len(names) != len(want) {
		t.Fatalf("names=%v want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names[%d]=%q want %q", i, names[i], want[i])
		}
	}
}
