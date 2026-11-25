package modelmanager

import "testing"

func TestDetectQuantizationAndDownloadURL(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "q4", file: "model-Q4_K_M.gguf", want: "Q4_K_M"},
		{name: "bf16", file: "model.BF16.gguf", want: "BF16"},
		{name: "unknown", file: "README.md", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DetectQuantization(test.file); got != test.want {
				t.Fatalf("DetectQuantization(%q) = %q, want %q", test.file, got, test.want)
			}
		})
	}
	url, err := DownloadURL("org/model", "abc123", "nested/model file.gguf")
	if err != nil || url != "https://huggingface.co/org/model/resolve/abc123/nested/model%20file.gguf" {
		t.Fatalf("DownloadURL() = (%q, %v)", url, err)
	}
	for _, filename := range []string{"", "../model.gguf"} {
		if _, err := DownloadURL("org/model", "abc123", filename); err == nil {
			t.Fatalf("DownloadURL(%q) accepted unsafe filename", filename)
		}
	}
}

func TestFilterFilesAndMMProjFiles(t *testing.T) {
	files := []ModelFile{{Path: "model-Q8.gguf", Quantization: "Q8"}, {Path: "model-Q4.gguf", Quantization: "Q4"}, {Path: "README.md"}}
	filtered := FilterFiles(files, nil, nil, []string{"Q4", "Q8"}, false)
	if len(filtered) != 2 || filtered[0].Quantization != "Q4" {
		t.Fatalf("FilterFiles() = %#v", filtered)
	}
	projectors := MMProjFiles([]ModelFile{{Path: "z-mmproj.gguf"}, {Path: "a-mmproj.gguf"}, {Path: "model.gguf"}})
	if len(projectors) != 2 || projectors[0].Path != "a-mmproj.gguf" {
		t.Fatalf("MMProjFiles() = %#v", projectors)
	}
}
