package modelmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// UpdateState describes whether an installed artifact has a newer remote revision.
type UpdateState string

const (
	// Current indicates that the installed revision matches the remote revision.
	Current UpdateState = "CURRENT"
	// UpdateAvailable indicates that a newer remote revision exists.
	UpdateAvailable UpdateState = "UPDATE AVAILABLE"
	// RemoteMissing indicates that the installed file is absent remotely.
	RemoteMissing UpdateState = "REMOTE MISSING"
	// UpdateUnknown indicates that the remote revision could not be compared.
	UpdateUnknown UpdateState = "UNKNOWN"
)

// CheckUpdate compares an installed artifact with the selected remote files.
func CheckUpdate(installed Installation, revision string, files []ModelFile) UpdateState {
	if revision == "" {
		return UpdateUnknown
	}
	found := false
	for _, f := range files {
		if f.Path == installed.Manifest.ModelFile {
			found = true
			break
		}
	}
	if !found {
		return RemoteMissing
	}
	if revision == installed.Manifest.Revision {
		return Current
	}
	return UpdateAvailable
}

type transaction struct {
	FinalArtifact  string `json:"finalArtifact"`
	FinalManifest  string `json:"finalManifest"`
	StagedArtifact string `json:"stagedArtifact"`
	StagedManifest string `json:"stagedManifest"`
	BackupArtifact string `json:"backupArtifact"`
	BackupManifest string `json:"backupManifest"`
}

// UpdateInstallation downloads a replacement artifact and atomically installs
// its verified artifact and manifest.
func UpdateInstallation(ctx context.Context, hfPath, modelsPath string, installed Installation, revision string, remote ModelFile) (Manifest, error) {
	destination, err := ResolveDestination(modelsPath, installed.Manifest.RepositoryID, installed.Manifest.ModelFile)
	if err != nil {
		return Manifest{}, err
	}
	stage, err := os.MkdirTemp(destination.Directory, ".update-")
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(stage)
	cmd := exec.CommandContext(ctx, hfPath, "download", installed.Manifest.RepositoryID, installed.Manifest.ModelFile, "--revision", revision, "--local-dir", stage)
	cmd.Env = os.Environ()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Manifest{}, ctx.Err()
		}
		return Manifest{}, fmt.Errorf("hf update failed: %s", sanitizeDiagnostic(output.String()))
	}
	stagedArtifact := filepath.Join(stage, filepath.FromSlash(installed.Manifest.ModelFile))
	hash, err := HashFile(ctx, stagedArtifact)
	if err != nil {
		return Manifest{}, err
	}
	info, err := os.Stat(stagedArtifact)
	if err != nil {
		return Manifest{}, err
	}
	if remote.Size > 0 && info.Size() != remote.Size {
		return Manifest{}, fmt.Errorf("staged artifact size mismatch")
	}
	url, err := DownloadURL(installed.Manifest.RepositoryID, revision, installed.Manifest.ModelFile)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{SchemaVersion: 1, ModelFile: installed.Manifest.ModelFile, RepositoryID: installed.Manifest.RepositoryID, Revision: revision, DownloadURL: url, DownloadedAt: time.Now().UTC(), SizeBytes: info.Size(), ETag: remote.ETag, LFSOID: remote.LFSOID, SHA256: hash}
	stagedManifest := stagedArtifact + ".json"
	if err := WriteManifestAtomic(stagedManifest, manifest); err != nil {
		return Manifest{}, err
	}
	tx := transaction{destination.Artifact, destination.Manifest, stagedArtifact, stagedManifest, destination.Artifact + ".update-backup", destination.Manifest + ".update-backup"}
	record := destination.Manifest + ".transaction"
	data, _ := json.Marshal(tx)
	if err := os.WriteFile(record, data, 0o600); err != nil {
		return Manifest{}, err
	}
	rollback := func() {
		_ = os.Remove(tx.FinalArtifact)
		_ = os.Remove(tx.FinalManifest)
		_ = os.Rename(tx.BackupArtifact, tx.FinalArtifact)
		_ = os.Rename(tx.BackupManifest, tx.FinalManifest)
	}
	if err = os.Rename(tx.FinalArtifact, tx.BackupArtifact); err != nil {
		return Manifest{}, err
	}
	if err = os.Rename(tx.FinalManifest, tx.BackupManifest); err != nil {
		_ = os.Rename(tx.BackupArtifact, tx.FinalArtifact)
		return Manifest{}, err
	}
	if err = os.Rename(tx.StagedArtifact, tx.FinalArtifact); err != nil {
		rollback()
		return Manifest{}, err
	}
	if err = os.Rename(tx.StagedManifest, tx.FinalManifest); err != nil {
		rollback()
		return Manifest{}, err
	}
	_ = os.Remove(tx.BackupArtifact)
	_ = os.Remove(tx.BackupManifest)
	_ = os.Remove(record)
	return manifest, nil
}

// RecoverTransactions completes or rolls back interrupted updates below modelsPath.
func RecoverTransactions(modelsPath string) error {
	return filepath.WalkDir(modelsPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".transaction" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var tx transaction
		if json.Unmarshal(data, &tx) != nil {
			return fmt.Errorf("invalid update transaction %s", path)
		}
		for _, target := range []string{tx.FinalArtifact, tx.FinalManifest, tx.BackupArtifact, tx.BackupManifest} {
			if err := confined(modelsPath, target); err != nil {
				return err
			}
		}
		if _, err := os.Stat(tx.FinalArtifact); os.IsNotExist(err) {
			_ = os.Rename(tx.BackupArtifact, tx.FinalArtifact)
		}
		if _, err := os.Stat(tx.FinalManifest); os.IsNotExist(err) {
			_ = os.Rename(tx.BackupManifest, tx.FinalManifest)
		}
		_ = os.Remove(tx.BackupArtifact)
		_ = os.Remove(tx.BackupManifest)
		return os.Remove(path)
	})
}
