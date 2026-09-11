package modelmanager

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// FitClass is the advisory classification produced by EstimateFit.
type FitClass string

const (
	// FitLikely indicates that the estimate leaves a 10% memory margin.
	FitLikely FitClass = "LIKELY FITS"
	// FitMarginal indicates that the estimate fits without the 10% margin.
	FitMarginal FitClass = "MARGINAL"
	// FitTooLarge indicates that the estimate exceeds available memory.
	FitTooLarge FitClass = "TOO LARGE"
	// FitUnknown indicates that artifact or memory size is unavailable.
	FitUnknown FitClass = "UNKNOWN"
)

// FitEstimate contains the advisory memory-fit calculation for an artifact.
type FitEstimate struct {
	Classification         FitClass `json:"classification"`
	ArtifactBytes          int64    `json:"artifactBytes"`
	EstimatedRequiredBytes int64    `json:"estimatedRequiredBytes"`
	AvailableRAMBytes      int64    `json:"availableRamBytes,omitempty"`
	AvailableVRAMBytes     int64    `json:"availableVramBytes,omitempty"`
	RuntimeOverhead        float64  `json:"runtimeOverhead"`
	Advisory               bool     `json:"advisory"`
}

// ParseByteSize parses decimal or binary byte units such as MB, GB, MiB, and GiB.
func ParseByteSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	upper := strings.ToUpper(value)
	// Match the longest suffix first. Otherwise the shorter B suffix can
	// claim values such as 1KB before KB gets a chance to match.
	units := []struct {
		suffix     string
		multiplier float64
	}{
		{"KIB", 1 << 10}, {"MIB", 1 << 20}, {"GIB", 1 << 30}, {"TIB", 1 << 40},
		{"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(upper, unit.suffix) {
			number := strings.TrimSpace(upper[:len(upper)-len(unit.suffix)])
			parsed, err := strconv.ParseFloat(number, 64)
			if err != nil || parsed < 0 {
				return 0, fmt.Errorf("invalid memory size %q", value)
			}
			return int64(parsed * unit.multiplier), nil
		}
	}
	return 0, fmt.Errorf("memory size %q requires a unit", value)
}

// DetectRAM returns host physical memory in bytes, or zero when unavailable.
func DetectRAM() int64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var kb int64
		if _, err := fmt.Sscanf(scanner.Text(), "MemTotal: %d kB", &kb); err == nil {
			return kb * 1024
		}
	}
	return 0
}

// EstimateFit classifies whether an artifact is likely to fit after overhead.
func EstimateFit(artifact, ram, vram int64, overhead float64) FitEstimate {
	result := FitEstimate{Classification: FitUnknown, ArtifactBytes: artifact, AvailableRAMBytes: ram, AvailableVRAMBytes: vram, RuntimeOverhead: overhead, Advisory: true}
	if overhead < 0 {
		overhead = 0
	}
	result.EstimatedRequiredBytes = int64(float64(artifact) * (1 + overhead))
	available := ram
	if vram > available {
		available = vram
	}
	if artifact <= 0 || available <= 0 {
		return result
	}
	if result.EstimatedRequiredBytes <= available*9/10 {
		result.Classification = FitLikely
	} else if result.EstimatedRequiredBytes <= available {
		result.Classification = FitMarginal
	} else {
		result.Classification = FitTooLarge
	}
	return result
}
