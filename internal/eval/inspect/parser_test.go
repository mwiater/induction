package inspect

import "testing"

func TestParseZeroScoreAndSamples(t *testing.T) {
	p, e := Parse([]byte(`{"results":{"samples":[1,2,3,4,5],"scores":{"accuracy":{"value":0}}}}`))
	if e != nil {
		t.Fatal(e)
	}
	if p.Score != 0 || p.Samples != 5 {
		t.Fatalf("%+v", p)
	}
}

func TestParseInspectResultsUsesLimitedCompletedSamples(t *testing.T) {
	p, err := Parse([]byte(`{"eval":{"dataset":{"samples":1172}},"results":{"total_samples":5,"completed_samples":5,"scores":[{"metrics":{"accuracy":{"value":0.8},"stderr":{"value":0.2}}}],"headline":{"metric":"accuracy"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Samples != 5 || p.Score != 0.8 || p.Metrics["stderr"] != 0.2 {
		t.Fatalf("unexpected parsed result: %+v", p)
	}
}

func TestParseInspectResultsToleratesNonFiniteSampleValues(t *testing.T) {
	p, err := Parse([]byte(`{"samples":[{"score":NaN}],"results":{"total_samples":5,"completed_samples":5,"scores":[{"metrics":{"mean":{"value":0.25},"stderr":{"value":0.1}}}],"headline":{"metric":"mean"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Samples != 5 || p.Score != 0.25 || p.Metrics["stderr"] != 0.1 {
		t.Fatalf("unexpected parsed result: %+v", p)
	}
}

func TestParseMalformed(t *testing.T) {
	if _, e := Parse([]byte("not json")); e == nil {
		t.Fatal("expected error")
	}
}
