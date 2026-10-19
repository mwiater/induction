package induction

import (
	"strings"
	"testing"
)

func TestValidatePipelineQuality(t *testing.T) {
	quality := &PipelineQuality{
		MinWords:         8,
		ExactHeading:     "# Chapter 1: Opening",
		RequiredPhrases:  []string{"Evelyn", "railway"},
		ForbiddenPhrases: []string{"the chapter ends", "in the next chapter"},
	}
	good := "# Chapter 1: Opening\n\nEvelyn entered the railway station and listened to the rain outside."
	if err := validatePipelineQuality(good, quality); err != nil {
		t.Fatalf("valid chapter rejected: %v", err)
	}
	checks := []string{
		"# Chapter 1: Opening\n\nEvelyn entered the station.",
		"# Chapter 2: Wrong\n\nEvelyn entered the railway station and listened to the rain outside.",
		"# Chapter 1: Opening\n\nEvelyn entered the railway station. The chapter ends here.",
		"# Chapter 1: Opening\n\nEvelyn entered the railway station. In the next chapter, the rain continued.",
	}
	for i, content := range checks {
		if err := validatePipelineQuality(content, quality); err == nil {
			t.Fatalf("case %d unexpectedly passed", i)
		}
	}
	if got := countPipelineWords(strings.TrimSpace(good)); got < 8 {
		t.Fatalf("word counter returned %d", got)
	}
}

func TestPipelineQualityValidation(t *testing.T) {
	valid := 1200
	maxRetries := 2
	p := &Pipeline{Name: "quality", Steps: []PipelineStep{{Name: "chapter", Model: "m", UserPrompt: "write", Quality: &PipelineQuality{MinWords: valid, MaxRetries: maxRetries}}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid quality config rejected: %v", err)
	}
	p.Steps[0].Quality.MaxRetries = 11
	if err := p.Validate(); err == nil {
		t.Fatal("invalid retry count accepted")
	}
}

func TestNovelgenLocalPipelineLoadsWithQualityContracts(t *testing.T) {
	p, err := LoadPipeline("pipelines/pipeline.novelgen.local.yaml")
	if err != nil {
		t.Fatalf("novel pipeline failed to load: %v", err)
	}
	if len(p.Steps) != 28 {
		t.Fatalf("step count = %d, want 28", len(p.Steps))
	}
	for _, step := range p.Steps {
		if len(step.Name) == len("chapter-01") && strings.HasPrefix(step.Name, "chapter-") && step.Quality == nil {
			t.Fatalf("chapter %q has no quality contract", step.Name)
		}
	}
}
