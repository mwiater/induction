package modelmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// HashFile computes the SHA-256 digest of path while honoring ctx cancellation.
func HashFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Verification contains the result of checking an installed artifact digest.
type Verification struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Model          string `json:"model"`
	ExpectedSHA256 string `json:"expectedSha256"`
	ActualSHA256   string `json:"actualSha256,omitempty"`
	Status         string `json:"status"`
	SizeBytes      int64  `json:"sizeBytes"`
}

// Verify hashes installation and compares it with the recorded manifest digest.
func Verify(ctx context.Context, installation Installation) (Verification, error) {
	if installation.Manifest.SchemaVersion == 2 {
		result := Verification{SchemaVersion: 1, Model: installation.Manifest.RepositoryID, Status: "VERIFIED"}
		if len(installation.ArtifactPaths) != len(installation.Manifest.Files) {
			result.Status = "MISSING"
			return result, nil
		}
		for i, file := range installation.Manifest.Files {
			actual, err := HashFile(ctx, installation.ArtifactPaths[i])
			if os.IsNotExist(err) {
				result.Status = "MISSING"
				return result, nil
			}
			if err != nil {
				return result, err
			}
			result.SizeBytes += file.SizeBytes
			if file.SHA256 == "" {
				result.Status = "UNVERIFIED"
			} else if !strings.EqualFold(actual, file.SHA256) {
				result.Status = "CHANGED"
				return result, nil
			}
		}
		return result, nil
	}
	v := Verification{SchemaVersion: 1, Model: installation.Manifest.RepositoryID + "/" + installation.Manifest.ModelFile, ExpectedSHA256: installation.Manifest.SHA256, SizeBytes: installation.Manifest.SizeBytes}
	actual, err := HashFile(ctx, installation.ArtifactPath)
	if os.IsNotExist(err) {
		v.Status = "MISSING"
		return v, nil
	}
	if err != nil {
		return v, err
	}
	v.ActualSHA256 = actual
	if installation.Manifest.SHA256 == "" {
		v.Status = "UNVERIFIED"
	} else if strings.EqualFold(actual, installation.Manifest.SHA256) {
		v.Status = "VERIFIED"
	} else {
		v.Status = "CHANGED"
	}
	return v, nil
}

// FindInstallation returns the installation matching a repository or model ID.
func FindInstallation(index InstalledIndex, model string) (Installation, error) {
	for _, item := range index.Installations {
		if item.Manifest.RepositoryID+"/"+item.Manifest.ModelFile == model {
			return item, nil
		}
		for _, file := range item.Manifest.Files {
			if item.Manifest.RepositoryID+"/"+file.ModelFile == model {
				return item, nil
			}
		}
	}
	return Installation{}, fmt.Errorf("installed model %q not found", model)
}

// RemoveInstallation removes the artifacts and manifest belonging to item.
func RemoveInstallation(modelsPath string, item Installation) error {
	paths := item.ArtifactPaths
	if len(paths) == 0 && item.ArtifactPath != "" {
		paths = []string{item.ArtifactPath}
	}
	for _, path := range paths {
		if err := confined(modelsPath, path); err != nil {
			return err
		}
	}
	if err := confined(modelsPath, item.ManifestPath); err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(item.ManifestPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
