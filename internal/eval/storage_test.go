package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeModelNameAndSave(t *testing.T) {
	if SafeModelName("org/model:v1") != "org__model__v1" {
		t.Fatal("unsafe path name")
	}
	d := t.TempDir()
	r := &Result{Suite: SuiteResult{Name: "suite", ConfigHash: "sha256:test"}, Model: ModelResult{Name: "org/model"}, Status: "success"}
	p, e := SaveResult(d, r)
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	var got Result
	if e = json.Unmarshal(b, &got); e != nil {
		t.Fatal(e)
	}
	if got.Model.Name != "org/model" {
		t.Fatal(got.Model.Name)
	}
	if got.Suite.ConfigHash != "sha256:test" {
		t.Fatal(got.Suite.ConfigHash)
	}
	if _, e = os.Stat(filepath.Join(d, "data/evals/results/org__model/suite.json")); e != nil {
		t.Fatal(e)
	}
}
