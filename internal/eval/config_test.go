package eval

import "testing"

func TestConfigValidation(t *testing.T) {
	c := &Config{Version: 1, Name: "suite", Evals: []Definition{{Task: "inspect_evals/arc_easy", Name: "arc", Tag: "science", Limit: 5}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Config){"version": func(x *Config) { x.Version = 0 }, "name": func(x *Config) { x.Name = "" }, "evals": func(x *Config) { x.Evals = nil }, "task": func(x *Config) { x.Evals[0].Task = "" }, "eval name": func(x *Config) { x.Evals[0].Name = "" }, "tag": func(x *Config) { x.Evals[0].Tag = "" }, "duplicate": func(x *Config) { x.Evals = append(x.Evals, x.Evals[0]) }, "duplicate tag": func(x *Config) { x.Evals = append(x.Evals, x.Evals[0]); x.Evals[1].Name = "other" }, "limit": func(x *Config) { x.Evals[0].Limit = -1 }, "unsafe task": func(x *Config) { x.Evals[0].Task = "inspect_evals/x;bad" }}
	for name, mutate := range tests {
		x := *c
		x.Evals = append([]Definition(nil), c.Evals...)
		mutate(&x)
		if err := x.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestHashChangesForSemanticConfig(t *testing.T) {
	a := &Config{Version: 1, Name: "x", Description: "one", Evals: []Definition{{Task: "inspect_evals/a", Name: "a", Tag: "a", Limit: 5}}}
	b := *a
	b.Evals = append([]Definition(nil), a.Evals...)
	ha, _ := a.Hash()
	b.Evals[0].Limit = 6
	hb, _ := b.Hash()
	if ha == hb {
		t.Fatal("semantic change did not change hash")
	}
}

func TestConfiguredBenchmarksFollowsCurrentSuite(t *testing.T) {
	existing := []BenchmarkResult{
		{Name: "arc", Task: "inspect_evals/arc_easy", Limit: 5, Samples: 5, Score: 1},
		{Name: "removed", Task: "inspect_evals/old", Limit: 5, Samples: 5, Score: 1},
		{Name: "changed", Task: "inspect_evals/old", Limit: 5, Samples: 5, Score: 1},
	}
	got := configuredBenchmarks(existing, []Definition{
		{Name: "arc", Task: "inspect_evals/arc_easy", Limit: 10},
		{Name: "changed", Task: "inspect_evals/new", Limit: 5},
		{Name: "new", Task: "inspect_evals/new", Limit: 5},
	})
	if len(got) != 1 || got[0].Name != "arc" || got[0].Limit != 5 {
		t.Fatalf("unexpected compatible benchmarks: %+v", got)
	}
}
