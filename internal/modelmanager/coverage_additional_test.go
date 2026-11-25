package modelmanager

import (
	"strings"
	"testing"
)

func TestInstalledRepositoryAndMatchingHelpers(t *testing.T) {
	index := InstalledIndex{Installations: []Installation{
		{Manifest: Manifest{SchemaVersion: 1, RepositoryID: "z/repo", ModelFile: "z.gguf"}},
		{Manifest: Manifest{SchemaVersion: 1, RepositoryID: "a/repo", ModelFile: "a.gguf"}},
		{Manifest: Manifest{SchemaVersion: 1, RepositoryID: "z/repo", ModelFile: "other.gguf"}},
	}}
	if got := InstalledRepositories(index); strings.Join(got, ",") != "a/repo,z/repo" {
		t.Fatalf("repositories = %v", got)
	}
	if !HasInstalledRepository(index, "a/repo") || HasInstalledRepository(index, "missing") {
		t.Fatal("repository presence helper is incorrect")
	}
	item := index.Installations[0]
	if InstallationModelID(item) != "z/repo/z.gguf" || !InstallationMatches(item, "z/repo/z.gguf") || InstallationMatches(item, "z/repo/nope.gguf") {
		t.Fatal("v1 installation matching is incorrect")
	}
	sharded := Installation{Manifest: Manifest{SchemaVersion: 2, RepositoryID: "org/model", Files: []ManifestFile{{ModelFile: "model-00001-of-00002.gguf"}, {ModelFile: "model-00002-of-00002.gguf"}}}}
	if InstallationModelID(sharded) != "org/model/model.gguf" || !InstallationMatches(sharded, "org/model/model-00002-of-00002.gguf") {
		t.Fatal("sharded installation matching is incorrect")
	}
}

func TestInstalledFormattingHelpers(t *testing.T) {
	for _, tc := range []struct {
		size int64
		want string
	}{
		{0, "unknown"}, {512, "512 B"}, {1024, "1.0 KiB"}, {2 << 20, "2.0 MiB"},
	} {
		if got := formatBytes(tc.size); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.size, got, tc.want)
		}
	}
	if formatRate(0) != "calculating…" || formatETA(0, 1) != "0s" || formatETA(1, 0) != "calculating…" {
		t.Fatal("zero rate/remaining formatting is incorrect")
	}
	if got := shortRevision("123456789012345"); got != "123456789012" {
		t.Fatalf("shortRevision = %q", got)
	}
}
