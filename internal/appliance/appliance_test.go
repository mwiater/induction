package appliance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var onePixelPNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0xf0, 0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}

func validFinalJSON() string {
	return `{"subjects":"a","composition":"b","environmental_context":"c","title":"t","caption":"c","image_category":"photo","activities":[],"keywords":[],"visible_text":[],"distinctive_details":[],"lighting":"l","color_palette":[],"photographic_technique":"p","image_quality":"q","editing_or_reproduction_clues":"e","location_estimate":{"value":null,"confidence":0,"evidence":[],"caveats":"unknown"},"camera_estimate":{"value":null,"confidence":0,"evidence":[],"caveats":"unknown","camera_category":null},"film_estimate":{"value":null,"confidence":0,"evidence":[],"caveats":"unknown","medium":"unknown","film_type":null,"film_stock":null},"capture_date_estimate":{"value":null,"confidence":0,"evidence":[],"caveats":"unknown","earliest_year":null,"latest_year":null,"scene_era":null}}`
}

func testAssets(t *testing.T) Assets {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	p, err := os.ReadFile(filepath.Join(root, "appliances", "image-meta", "pipeline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := os.ReadFile(filepath.Join(root, "appliances", "image-meta", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return Assets{Pipeline: p, Config: c}
}

func TestEmbeddedAssetsMatchPolicy(t *testing.T) {
	p, c, m, err := parseAssets(testAssets(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 3 || p.Steps[len(p.Steps)-1].Name != "return-json" || len(m.Models) != 3 {
		t.Fatalf("unexpected manifest or pipeline: %#v %#v", m, p)
	}
	for i, want := range []string{"Gemma-4-E4B-UNC-Q8_K_P", "Granite-Vision-4.1-4B-Q4_K_M", "MiniCPM-V-4.7-35B-A3B-Q4_K_M"} {
		if m.Models[i] != want {
			t.Fatalf("manifest model[%d] = %q, want %q", i, m.Models[i], want)
		}
	}
	if c.Server == "" {
		t.Fatal("endpoint missing")
	}
}

func TestFinalSchemaValidationAndNoExtraProperties(t *testing.T) {
	if err := validateFinal([]byte(validFinalJSON())); err != nil {
		t.Fatal(err)
	}
	if err := validateFinal([]byte(`{"subjects":"a"}`)); err == nil {
		t.Fatal("extra property accepted")
	}
	if err := validateFinal([]byte(strings.Replace(validFinalJSON(), `"title":"t"`, `"title":1`, 1))); err == nil {
		t.Fatal("non-string property accepted")
	}
	if err := validateFinal([]byte(strings.Replace(validFinalJSON(), `"evidence":[]`, `"evidence":[1]`, 1))); err == nil {
		t.Fatal("non-string evidence item accepted")
	}
	if err := validateFinal([]byte(strings.Replace(validFinalJSON(), `"confidence":0`, `"confidence":2`, 1))); err == nil {
		t.Fatal("out-of-range confidence accepted")
	}
	if err := validateFinal([]byte(strings.Replace(validFinalJSON(), `"caveats":"unknown"`, `"caveats":"unknown","unexpected":true`, 1))); err == nil {
		t.Fatal("nested additional property accepted")
	}
	if err := validateFinal([]byte(strings.Replace(validFinalJSON(), `"capture_date_estimate":{"value":null`, `"capture_date_estimate":{"value":2024.5`, 1))); err == nil {
		t.Fatal("fractional year accepted")
	}
	if err := validateFinal([]byte(strings.Replace(validFinalJSON(), `"confidence":0,"evidence"`, `"evidence"`, 1))); err == nil {
		t.Fatal("missing nested required property accepted")
	}
}

func TestDiscoverySortsAndPreservesConflicts(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("b.png", onePixelPNG, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("a.jpg", onePixelPNG, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("bad.jpg", []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	log, err := openJournal(dir, Assets{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.close()
	items, failures, err := discover(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	if failures != 1 || len(items) != 2 || items[0].name != "a.jpg" || items[1].name != "b.png" {
		t.Fatalf("items=%#v failures=%d", items, failures)
	}
}

func TestDiscoveryIgnoresSymlinksAndDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.png"), onePixelPNG, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.png", filepath.Join(dir, "link.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	log, err := openJournal(dir, Assets{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.close()
	items, failures, err := discover(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	if failures != 0 || len(items) != 1 || items[0].name != "real.png" {
		t.Fatalf("items=%#v failures=%d", items, failures)
	}
}

func TestJournalLockIsExclusive(t *testing.T) {
	dir := t.TempDir()
	first, err := openJournal(dir, Assets{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	second, err := openJournal(dir, Assets{})
	if err == nil {
		second.close()
		t.Fatal("second journal acquired the run lock")
	}
}

func TestExistingOutputRejectsMalformedAndHardlinkedFiles(t *testing.T) {
	dir := t.TempDir()
	malformed := filepath.Join(dir, "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{"subjects":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if state, err := existingOutput(malformed); err != nil || state != outputConflict {
		t.Fatalf("malformed output state=%v err=%v", state, err)
	}
	original := filepath.Join(dir, "original.json")
	hardlink := filepath.Join(dir, "hardlink.json")
	if err := os.WriteFile(original, []byte(validFinalJSON()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, hardlink); err != nil {
		t.Fatal(err)
	}
	if state, err := existingOutput(hardlink); err != nil || state != outputConflict {
		t.Fatalf("hardlinked output state=%v err=%v", state, err)
	}
}

func TestPublisherDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	log, err := openJournal(dir, Assets{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.close()
	target := filepath.Join(dir, "image.jpg.json")
	if err := publish(log, target, []byte(validFinalJSON()), "image.jpg"); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(target)
	if err := publish(log, target, []byte(strings.Replace(validFinalJSON(), `"title":"t"`, `"title":"different"`, 1)), "image.jpg"); err == nil {
		t.Fatal("publisher clobbered existing output")
	}
	current, _ := os.ReadFile(target)
	if string(current) != string(original) {
		t.Fatal("existing output changed")
	}
}

func TestArgumentsAreRejectedBeforePreflight(t *testing.T) {
	assets := testAssets(t)
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if got := Run([]string{"--help"}, assets); got != ExitArguments {
		t.Fatalf("exit=%d", got)
	}
	data, err := os.ReadFile(logName)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("rejection was not logged")
	}
}

func TestApplianceEndToEndSubprocess(t *testing.T) {
	if os.Getenv("INDUCTION_APPLIANCE_SUBPROCESS") == "1" {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/health", "/v1/health":
				_, _ = w.Write([]byte(`{}`))
			case "/v1/models":
				_, _ = w.Write([]byte(`{"data":[{"id":"Gemma-4-E4B-UNC-Q8_K_P","state":"loaded","architecture":{"input_modalities":["text","image","audio"],"output_modalities":["text"]}},{"id":"Granite-Vision-4.1-4B-Q4_K_M","state":"loaded","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}},{"id":"MiniCPM-V-4.7-35B-A3B-Q4_K_M","state":"loaded","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}}]}`))
			case "/v1/chat/completions":
				var req map[string]any
				_ = json.NewDecoder(r.Body).Decode(&req)
				content := `subjects`
				if format, ok := req["response_format"].(map[string]any); ok && format["type"] == "json_object" {
					content = validFinalJSON()
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + strconvQuote(content) + `}}]}`))
			case "/props":
				_, _ = w.Write([]byte(`{}`))
			case "/slots":
				_, _ = w.Write([]byte(`[]`))
			case "/metrics":
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte{})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		assets := testAssets(t)
		assets.Config = []byte(strings.Replace(string(assets.Config), "http://192.168.0.239:9998", server.URL, 1))
		code := Run(nil, assets)
		os.Exit(code)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.png"), onePixelPNG, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestApplianceEndToEndSubprocess")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "INDUCTION_APPLIANCE_SUBPROCESS=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		logData, _ := os.ReadFile(filepath.Join(dir, logName))
		t.Fatalf("subprocess failed: %v\n%s\nlog=%s", err, out, logData)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sample.png.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateFinal(data); err != nil {
		t.Fatal(err)
	}
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }
