package cli

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRedistributeReport(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "image.png"), "fixture")
	writeTestFile(t, filepath.Join(dir, "pipeline-one.yaml"), `name: one
steps:
  - name: vision
    model: vision-model
    userPrompt: inspect
    image: image.png
  - name: decide
    model: decision-model
    userPrompt: decide
    decision:
      candidates:
        yes: yes
        no: no
  - name: json
    model: vision-model
    userPrompt: serialize
    output:
      type: json
      artifact: result.json
`)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		body := `{"data":[{"id":"vision-model","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}},{"id":"decision-model","architecture":{"input_modalities":["text"],"output_modalities":["decisions"]}}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	report, err := buildRedistributeReport(context.Background(), "http://server", dir, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Models) != 2 || report.Models[0].CurrentUsageCount != 2 || report.Models[1].CurrentUsageCount != 1 {
		t.Fatalf("unexpected model usage: %#v", report.Models)
	}
	if got := report.Models[0].OutputModalities; strings.Join(got, ",") != "text,json,json_object" {
		t.Fatalf("vision output modalities = %#v", got)
	}
	if got := report.Models[1].OutputModalities; strings.Join(got, ",") != "decisions" {
		t.Fatalf("decision output modalities = %#v", got)
	}
	steps := report.Pipelines[0].Steps
	if strings.Join(steps[0].StepInputModalities, ",") != "text,image" || strings.Join(steps[0].StepOutputModalities, ",") != "text" {
		t.Fatalf("vision step = %#v", steps[0])
	}
	if strings.Join(steps[1].StepOutputModalities, ",") != "decisions" || strings.Join(steps[2].StepOutputModalities, ",") != "json" {
		t.Fatalf("special outputs = %#v %#v", steps[1], steps[2])
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
