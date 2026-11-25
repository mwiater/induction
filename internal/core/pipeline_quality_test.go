package induction

import (
	"os"
	"path/filepath"
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

func TestPipelineQualityRetryFeedbackIncludesShortDraft(t *testing.T) {
	quality := &PipelineQuality{MinWords: 1100, MaxWords: 1600, MaxRetries: 2}
	draft := "## Part 1: The Face of the Anomaly\n\nEvelyn opened the clock."
	feedback := pipelineQualityRetryFeedback(quality, draft, validatePipelineQuality(draft, quality), 1)
	for _, want := range []string{"QUALITY RETRY 1/2", "at least 1100 words and at most 1600 words", "PREVIOUS DRAFT:", draft} {
		if !strings.Contains(feedback, want) {
			t.Fatalf("feedback missing %q: %s", want, feedback)
		}
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

func TestLoadPipelineWithChapterQualityContracts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chapters.yaml")
	data := `name: chapter-quality-fixture
steps:
  - name: chapter-01
    model: test-model
    userPrompt: Write the first chapter.
    quality:
      minWords: 8
      exactHeading: '# Chapter 1: Opening'
      requiredPhrases: [Evelyn]
      forbiddenPhrases: ['the chapter ends']
      maxRetries: 2
  - name: chapter-02
    model: test-model
    userPrompt: Write the second chapter.
    quality:
      minWords: 8
      exactHeading: '# Chapter 2: Arrival'
      requiredPhrases: [railway]
      maxRetries: 2
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write pipeline fixture: %v", err)
	}
	p, err := LoadPipeline(path)
	if err != nil {
		t.Fatalf("chapter pipeline failed to load: %v", err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("step count = %d, want 2", len(p.Steps))
	}
	for _, step := range p.Steps {
		if step.Quality == nil {
			t.Fatalf("chapter %q has no quality contract", step.Name)
		}
		if step.Quality.MinWords != 8 || step.Quality.MaxRetries != 2 {
			t.Fatalf("chapter %q quality contract was not decoded: %+v", step.Name, step.Quality)
		}
	}
	if err := validatePipelineQuality("# Chapter 1: Opening\n\nEvelyn entered the railway station and listened to the rain outside.", p.Steps[0].Quality); err != nil {
		t.Fatalf("loaded quality contract rejected a valid chapter: %v", err)
	}
	if err := validatePipelineQuality("# Chapter 1: Opening\n\nThe chapter ends in the railway station after the rain outside.", p.Steps[0].Quality); err == nil {
		t.Fatal("loaded quality contract accepted a forbidden phrase")
	}
}
