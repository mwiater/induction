// Package appliance provides the implementation for this subsystem.
package appliance

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	// Register JPEG and PNG decoders for image.Decode.
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// maxImageBytes identifies a fixed value used internally by this package.
const maxImageBytes = 32 << 20

// supportedImage performs the internal operation described by its signature.
func supportedImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}

// validateImage performs the internal operation described by its signature.
func validateImage(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty image")
	}
	return validateImageBytes(data, filepath.Ext(path))
}

// validateImageAt performs the internal operation described by its signature.
func validateImageAt(dirfd int, name string) error {
	data, info, err := readWorkspaceFile(dirfd, name, maxImageBytes)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("empty image")
	}
	return validateImageBytes(data, filepath.Ext(name))
}

// validateImageBytes performs the internal operation described by its signature.
func validateImageBytes(data []byte, extension string) error {
	if len(data) == 0 {
		return fmt.Errorf("empty image")
	}
	if strings.EqualFold(extension, ".webp") {
		if !isWebP(data) {
			return fmt.Errorf("invalid WebP container")
		}
		return nil
	}
	if typ := http.DetectContentType(data); !strings.HasPrefix(typ, "image/") {
		return fmt.Errorf("not an image MIME type: %s", typ)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	return nil
}

// isWebP performs the internal operation described by its signature.
func isWebP(data []byte) bool {
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return false
	}
	containerSize := binary.LittleEndian.Uint32(data[4:8])
	if containerSize < 12 || uint64(containerSize)+8 > uint64(len(data)) {
		return false
	}
	if string(data[12:16]) != "VP8 " && string(data[12:16]) != "VP8L" && string(data[12:16]) != "VP8X" {
		return false
	}
	chunkSize := binary.LittleEndian.Uint32(data[16:20])
	return uint64(chunkSize)+20 <= uint64(len(data))
}
