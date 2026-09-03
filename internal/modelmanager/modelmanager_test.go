package modelmanager

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakeHub struct{ searches map[string][]SearchResult }

func (f fakeHub) Search(_ context.Context, _ string, _ int, provider string) ([]SearchResult, error) {
	return f.searches[provider], nil
}
func (fakeHub) ListFiles(context.Context, string) (string, []ModelFile, error) {
	return "sha", nil, nil
}

func TestRankPreferredAndDeduplicate(t *testing.T) {
	client := fakeHub{map[string][]SearchResult{"": {{ID: "other/x", Provider: "other"}, {ID: "acme/a", Provider: "acme"}}, "acme": {{ID: "acme/a", Provider: "acme"}, {ID: "acme/b", Provider: "acme"}}}}
	got, err := SearchRanked(context.Background(), client, "x", 3, []string{"acme"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{got[0].ID, got[1].ID, got[2].ID}
	if !reflect.DeepEqual(ids, []string{"acme/a", "acme/b", "other/x"}) {
		t.Fatalf("unexpected ranking: %v", ids)
	}
}

func TestDestinationConfinement(t *testing.T) {
	base := t.TempDir()
	if _, err := ResolveDestination(base, "org/repo", "../escape.gguf"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	destination, err := ResolveDestination(base, "org/repo", "nested/model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(destination.Directory) != filepath.Join(base, "org") {
		t.Fatalf("unexpected destination: %+v", destination)
	}
}

func TestFilterAndShardSet(t *testing.T) {
	files := []ModelFile{{Path: "readme.md"}, {Path: "model-Q5_K_M.gguf", Quantization: "Q5_K_M"}, {Path: "model-Q4_K_M.gguf", Quantization: "Q4_K_M"}}
	got := FilterFiles(files, nil, nil, []string{"Q4_K_M"}, false)
	if len(got) != 2 || got[0].Path != "model-Q4_K_M.gguf" {
		t.Fatalf("unexpected filter: %+v", got)
	}
	shards := []ModelFile{{Path: "model-00002-of-00002.gguf"}, {Path: "model-00001-of-00002.gguf"}}
	stem, set, err := ShardSet(shards[0], shards)
	if err != nil || stem != "model.gguf" || set[0].Path != "model-00001-of-00002.gguf" {
		t.Fatalf("unexpected shard set %q %+v %v", stem, set, err)
	}
}

func TestManifestIndexVerifyRemove(t *testing.T) {
	base := t.TempDir()
	destination, err := ResolveDestination(base, "org/repo", "model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(destination.Directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(destination.Artifact, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := HashFile(context.Background(), destination.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 1, ModelFile: "model.gguf", RepositoryID: "org/repo", Revision: "abc", DownloadURL: "https://example.invalid", DownloadedAt: time.Now(), SizeBytes: 5, SHA256: hash}
	if err = WriteManifestAtomic(destination.Manifest, manifest); err != nil {
		t.Fatal(err)
	}
	index, err := BuildInstalledIndex(base)
	if err != nil || len(index.Installations) != 1 {
		t.Fatalf("index: %+v %v", index, err)
	}
	verification, err := Verify(context.Background(), index.Installations[0])
	if err != nil || verification.Status != "VERIFIED" {
		t.Fatalf("verify: %+v %v", verification, err)
	}
	if err = RemoveInstallation(base, index.Installations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(destination.Artifact); !os.IsNotExist(err) {
		t.Fatalf("artifact still exists: %v", err)
	}
}

func TestFitBoundaries(t *testing.T) {
	if got := EstimateFit(70, 100, 0, 0).Classification; got != FitLikely {
		t.Fatal(got)
	}
	if got := EstimateFit(95, 100, 0, 0).Classification; got != FitMarginal {
		t.Fatal(got)
	}
	if got := EstimateFit(101, 100, 0, 0).Classification; got != FitTooLarge {
		t.Fatal(got)
	}
	if got := EstimateFit(0, 100, 0, 0).Classification; got != FitUnknown {
		t.Fatal(got)
	}
}

func TestParseByteSizeAndCheckUpdate(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int64
	}{
		{"1KB", 1000}, {"2MiB", 2 << 20}, {"1.5GB", 1500000000},
	} {
		got, err := ParseByteSize(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("ParseByteSize(%q) = %d, %v; want %d", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"12", "bad", "-1GB"} {
		if _, err := ParseByteSize(input); err == nil {
			t.Errorf("ParseByteSize(%q) unexpectedly succeeded", input)
		}
	}
	installation := Installation{Manifest: Manifest{RepositoryID: "acme/model", ModelFile: "model.gguf", Revision: "old"}}
	if got := CheckUpdate(installation, "", nil); got != UpdateUnknown {
		t.Errorf("empty revision state = %q", got)
	}
	if got := CheckUpdate(installation, "new", nil); got != RemoteMissing {
		t.Errorf("missing file state = %q", got)
	}
	files := []ModelFile{{Path: "model.gguf"}}
	if got := CheckUpdate(installation, "old", files); got != Current {
		t.Errorf("current state = %q", got)
	}
	if got := CheckUpdate(installation, "new", files); got != UpdateAvailable {
		t.Errorf("update state = %q", got)
	}
}

func TestInstalledIndexLookups(t *testing.T) {
	index := InstalledIndex{Installations: []Installation{{Manifest: Manifest{RepositoryID: "acme/model", ModelFile: "model.gguf", Revision: "rev"}}}}
	if index.RepositoryCount("acme/model") != 1 || index.RepositoryCount("other/model") != 0 {
		t.Fatalf("unexpected repository counts")
	}
	if index.FileState(t.TempDir(), "acme/model", "model.gguf", "rev") != FileInstalled {
		t.Fatal("expected installed file")
	}
	if index.FileState(t.TempDir(), "acme/model", "model.gguf", "other") != FileDifferentRevision {
		t.Fatal("expected different revision")
	}
}

func TestOldHFModelsCommandFallsBackToHubAPI(t *testing.T) {
	hf := filepath.Join(t.TempDir(), "hf")
	script := "#!/bin/sh\necho \"hf: error: invalid choice: 'models'\" >&2\nexit 2\n"
	if err := os.WriteFile(hf, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &HFCLIClient{Path: hf, APIBaseURL: "https://hub.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `[{"id":"acme/model","sha":"abc","downloads":3}]`
		if request.URL.Path == "/api/models/acme/model" {
			body = `{"id":"acme/model","sha":"abc","siblings":[{"rfilename":"model.gguf","size":5}]}`
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	results, err := client.Search(context.Background(), "model", 1, "")
	if err != nil || len(results) != 1 || results[0].ID != "acme/model" {
		t.Fatalf("search fallback: %+v %v", results, err)
	}
	revision, files, err := client.ListFiles(context.Background(), "acme/model")
	if err != nil || revision != "abc" || len(files) != 1 || files[0].Path != "model.gguf" {
		t.Fatalf("files fallback: %q %+v %v", revision, files, err)
	}
}

func TestInteractiveRepositoryFileConfirmationFlow(t *testing.T) {
	m := NewInteractiveModel(context.Background(), fakeHub{}, InteractiveOptions{SearchResults: 10, ModelsPath: t.TempDir(), HFPath: "hf"}, "tiny")
	updated, _ := m.Update(searchCompletedMsg{query: "tiny", results: []SearchResult{{ID: "acme/model", Provider: "acme"}}})
	m = updated.(Model)
	if m.Screen != ScreenRepositories {
		t.Fatalf("screen=%v", m.Screen)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.Screen != ScreenLoadingFiles {
		t.Fatalf("screen=%v", m.Screen)
	}
	updated, _ = m.Update(filesCompletedMsg{repository: "acme/model", revision: "abc", files: []ModelFile{{Path: "model-Q4_K_M.gguf", Size: 1024}}})
	m = updated.(Model)
	if m.Screen != ScreenFiles {
		t.Fatalf("screen=%v", m.Screen)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.Screen != ScreenConfirm || !strings.Contains(m.View(), "y confirm") {
		t.Fatalf("confirmation not shown: screen=%v view=%q", m.Screen, m.View())
	}
}

func TestInstalledActionsUseListAndConfirmation(t *testing.T) {
	installation := Installation{Manifest: Manifest{SchemaVersion: 1, RepositoryID: "acme/model", ModelFile: "model.gguf", Revision: "abc"}, ArtifactPath: "/models/acme/model/model.gguf", ManifestPath: "/models/acme/model/model.gguf.json"}
	index := InstalledIndex{Installations: []Installation{installation}}
	details := NewInstalledModel(context.Background(), nil, InteractiveOptions{}, ActionDetails, index, "")
	updated, _ := details.Update(tea.KeyMsg{Type: tea.KeyEnter})
	details = updated.(InstalledModel)
	if details.screen != "details" || !strings.Contains(details.View(), "acme/model/model.gguf") {
		t.Fatalf("details screen: %q", details.View())
	}
	remove := NewInstalledModel(context.Background(), nil, InteractiveOptions{}, ActionRemove, index, "")
	updated, _ = remove.Update(tea.KeyMsg{Type: tea.KeyEnter})
	remove = updated.(InstalledModel)
	if remove.screen != "confirm" || !strings.Contains(remove.View(), "y confirm") {
		t.Fatalf("remove confirmation: %q", remove.View())
	}
	verify := NewInstalledModel(context.Background(), nil, InteractiveOptions{}, ActionVerify, index, "")
	updated, cmd := verify.Update(tea.KeyMsg{Type: tea.KeyEnter})
	verify = updated.(InstalledModel)
	if verify.screen != "working" || cmd == nil {
		t.Fatalf("verify did not start asynchronously")
	}
}

func TestInteractionLog(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	temporary := t.TempDir()
	if err = os.Chdir(temporary); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)
	if err = LogInteraction("test_event", "model=acme/model", "value=line\nbreak"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(temporary, InteractionLogPath))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "event=test_event") || !strings.Contains(text, "model=acme/model") || strings.Contains(text, "line\nbreak") {
		t.Fatalf("unexpected log: %q", text)
	}
}

func TestHTTPDownloadReportsDownloadAndValidationProgress(t *testing.T) {
	payload := []byte("model payload")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", ContentLength: int64(len(payload)), Body: io.NopCloser(strings.NewReader(string(payload))), Header: make(http.Header)}, nil
	})}
	base := t.TempDir()
	updates := []ProgressUpdate{}
	manifest, err := DownloadHTTP(context.Background(), client, DownloadRequest{Repository: "acme/model", File: "model.gguf", Revision: "abc", ModelsPath: base, Size: int64(len(payload))}, func(update ProgressUpdate) { updates = append(updates, update) })
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SHA256 == "" {
		t.Fatal("missing SHA-256")
	}
	sawDownload, sawValidation := false, false
	for _, update := range updates {
		if update.Phase == PhaseDownloading && update.CompletedBytes == int64(len(payload)) {
			sawDownload = true
		}
		if update.Phase == PhaseValidating && update.CompletedBytes == int64(len(payload)) {
			sawValidation = true
		}
	}
	if !sawDownload || !sawValidation {
		t.Fatalf("incomplete progress: %+v", updates)
	}
	destination, _ := ResolveDestination(base, "acme/model", "model.gguf")
	if data, err := os.ReadFile(destination.Artifact); err != nil || !reflect.DeepEqual(data, payload) {
		t.Fatalf("artifact: %q %v", data, err)
	}
}

func TestProgressETAFormatting(t *testing.T) {
	cases := []struct {
		remaining int64
		rate      float64
		want      string
	}{{100, 0, "calculating…"}, {0, 10, "0s"}, {120, 2, "1m00s"}, {7320, 2, "1h01m"}}
	for _, test := range cases {
		if got := formatETA(test.remaining, test.rate); got != test.want {
			t.Fatalf("formatETA(%d,%f)=%q want %q", test.remaining, test.rate, got, test.want)
		}
	}
}
