package induction

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// ArtifactPathContext contains the runtime values available to artifact path
// interpolation. Batch metadata is deliberately explicit: it is never
// inferred from a filename or from the number of completed items.
type ArtifactPathContext struct {
	InputSet   InputSet
	BatchID    string
	BatchIndex int
	IsBatch    bool
}

// ResolveArtifactPath resolves the small, deliberately fixed artifact path
// interpolation language. Filesystem safety remains the responsibility of
// persistPipelineArtifact.
func ResolveArtifactPath(configuredPath string, ctx ArtifactPathContext) (string, error) {
	if !strings.Contains(configuredPath, "{{") && !strings.Contains(configuredPath, "}}") {
		return configuredPath, nil
	}
	var out strings.Builder
	for pos := 0; pos < len(configuredPath); {
		open := strings.Index(configuredPath[pos:], "{{")
		close := strings.Index(configuredPath[pos:], "}}")
		if close >= 0 && (open < 0 || close < open) {
			return "", fmt.Errorf("malformed artifact path template %q", configuredPath)
		}
		if open < 0 {
			out.WriteString(configuredPath[pos:])
			break
		}
		open += pos
		out.WriteString(configuredPath[pos:open])
		end := strings.Index(configuredPath[open+2:], "}}")
		if end < 0 {
			return "", fmt.Errorf("malformed artifact path template %q", configuredPath)
		}
		end += open + 2
		variable := strings.TrimSpace(configuredPath[open+2 : end])
		if variable == "" {
			return "", fmt.Errorf("malformed artifact path template %q", configuredPath)
		}
		value, err := artifactPathVariable(variable, configuredPath, ctx)
		if err != nil {
			return "", err
		}
		out.WriteString(value)
		pos = end + 2
	}
	return out.String(), nil
}

func artifactPathVariable(variable, configuredPath string, ctx ArtifactPathContext) (string, error) {
	switch variable {
	case "source.filename", "source.basename", "source.extension":
		paths := ctx.InputSet.allPaths()
		if len(paths) != 1 {
			return "", fmt.Errorf("artifact path %q requires exactly one source file; pipeline input contains %d sources", configuredPath, len(paths))
		}
		filename := filepath.Base(paths[0])
		ext := filepath.Ext(filename)
		switch variable {
		case "source.filename":
			return filename, nil
		case "source.basename":
			return strings.TrimSuffix(filename, ext), nil
		default:
			return strings.TrimPrefix(ext, "."), nil
		}
	case "batch.id":
		if !ctx.IsBatch {
			return "", fmt.Errorf("artifact path %q references batch.id outside a batch run", configuredPath)
		}
		return ctx.BatchID, nil
	case "batch.index":
		if !ctx.IsBatch {
			return "", fmt.Errorf("artifact path %q references batch.index outside a batch run", configuredPath)
		}
		return strconv.Itoa(ctx.BatchIndex), nil
	default:
		return "", fmt.Errorf("unknown artifact path variable %q", variable)
	}
}

func defaultArtifactPathContext(p *Pipeline) ArtifactPathContext {
	if p == nil {
		return ArtifactPathContext{}
	}
	if p.Inputs != nil {
		return ArtifactPathContext{InputSet: *p.Inputs}
	}
	if len(p.Steps) > 0 {
		input := InputSet{}
		if p.Steps[0].Image != "" {
			input.Images = []string{p.Steps[0].Image}
		}
		if p.Steps[0].Document != "" {
			input.Documents = []string{p.Steps[0].Document}
		}
		return ArtifactPathContext{InputSet: input}
	}
	return ArtifactPathContext{}
}
