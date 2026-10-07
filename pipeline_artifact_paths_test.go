package induction

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveArtifactPath(t *testing.T) {
	ctx := ArtifactPathContext{
		InputSet:   InputSet{Files: []string{"/path/to/report.final.v2.pdf"}},
		BatchID:    "customer",
		BatchIndex: 2,
		IsBatch:    true,
	}
	tests := []struct {
		name, template, want string
	}{
		{"basename", "{{source.basename}}.json", "report.final.v2.json"},
		{"filename", "{{source.filename}}.json", "report.final.v2.pdf.json"},
		{"extension", "{{source.extension}}", "pdf"},
		{"nested", "analysis/{{source.basename}}.json", "analysis/report.final.v2.json"},
		{"multiple", "{{batch.index}}-{{batch.id}}-{{source.basename}}.json", "2-customer-report.final.v2.json"},
		{"static", "result.json", "result.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveArtifactPath(tt.template, ctx)
			if err != nil || got != tt.want {
				t.Fatalf("got %q, err %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestResolveArtifactPathErrorsAndMultipleSources(t *testing.T) {
	tests := []struct {
		name, template string
		ctx            ArtifactPathContext
		contains       string
	}{
		{"zero sources", "{{source.basename}}.json", ArtifactPathContext{}, "contains 0 sources"},
		{"multiple sources", "{{source.basename}}.json", ArtifactPathContext{InputSet: InputSet{Documents: []string{"a.pdf", "b.pdf"}}}, "contains 2 sources"},
		{"batch outside batch", "{{batch.id}}.json", ArtifactPathContext{}, "outside a batch run"},
		{"unknown variable", "{{source.foo}}.json", ArtifactPathContext{}, "unknown artifact path variable"},
		{"unknown namespace", "{{foo.bar}}.json", ArtifactPathContext{}, "unknown artifact path variable"},
		{"unterminated", "{{source.basename", ArtifactPathContext{}, "malformed artifact path template"},
		{"stray close", "source.basename}}", ArtifactPathContext{}, "malformed artifact path template"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveArtifactPath(tt.template, tt.ctx)
			if err == nil || !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("err=%v, want substring %q", err, tt.contains)
			}
		})
	}
	got, err := ResolveArtifactPath("combined.json", ArtifactPathContext{InputSet: InputSet{Documents: []string{"a.pdf", "b.pdf"}}})
	if err != nil || got != "combined.json" {
		t.Fatalf("static multi-source path got %q, err %v", got, err)
	}
}

func TestResolveArtifactPathNoExtension(t *testing.T) {
	got, err := ResolveArtifactPath("{{source.basename}}.{{source.extension}}", ArtifactPathContext{InputSet: InputSet{Files: []string{filepath.Join("/tmp", "README")}}})
	if err != nil || got != "README." {
		t.Fatalf("got %q, err %v", got, err)
	}
}
