package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mwiater/induction"
)

func TestRun(t *testing.T) {
	oldLoad, oldList := loadConfig, listModels
	defer func() { loadConfig, listModels = oldLoad, oldList }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/models" {
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"example-model","loaded":true}]}`)
	}))
	defer srv.Close()
	loadConfig = func(...string) (*induction.Config, error) {
		return &induction.Config{Server: srv.URL}, nil
	}

	var output bytes.Buffer
	if err := run(&output); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.Contains(output.String(), "MODEL") || !strings.Contains(output.String(), "example-model") || !strings.Contains(output.String(), "LOADED") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestRunErrors(t *testing.T) {
	oldLoad, oldList := loadConfig, listModels
	defer func() { loadConfig, listModels = oldLoad, oldList }()

	loadConfig = func(...string) (*induction.Config, error) { return nil, errors.New("boom") }
	if err := run(io.Discard); err == nil {
		t.Fatal("expected config error")
	}

	loadConfig = func(...string) (*induction.Config, error) { return &induction.Config{Server: "server"}, nil }
	listModels = func(string, ...induction.ClientOption) error { return errors.New("boom") }
	if err := run(io.Discard); err == nil {
		t.Fatal("expected list error")
	}
}

func TestMain(t *testing.T) {
	oldRun, oldFatal := runMain, fatal
	defer func() { runMain, fatal = oldRun, oldFatal }()
	called := false
	fatal = func(...any) { called = true }
	runMain = func(io.Writer) error { return nil }
	main()
	runMain = func(io.Writer) error { return errors.New("boom") }
	main()
	if !called {
		t.Fatal("expected fatal call")
	}
}
