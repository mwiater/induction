package induction

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DefaultAttachmentMaxBytes is the default maximum attachment size, in bytes,
// used by applications that need a conservative 10 MiB upload limit.
const DefaultAttachmentMaxBytes int64 = 10 << 20

// ImageDataURL reads a local image and returns an OpenAI-compatible data URL.
// Empty, unsupported, and oversized files are rejected before they are sent.
func ImageDataURL(path string, maxBytes int64) (string, error) {
	return attachmentDataURL(path, maxBytes, true)
}

// FileDataURL reads a local document and returns a base64 data URL. The
// filename is retained separately because some servers require it in the file
// content part.
func FileDataURL(path string, maxBytes int64) (dataURL string, filename string, err error) {
	dataURL, err = attachmentDataURL(path, maxBytes, false)
	if err != nil {
		return "", "", err
	}
	return dataURL, filepath.Base(path), nil
}

func attachmentDataURL(path string, maxBytes int64, imageOnly bool) (string, error) {
	if maxBytes <= 0 {
		return "", errors.New("attachment size limit must be positive")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open attachment: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat attachment: %w", err)
	}
	if info.Size() == 0 {
		return "", errors.New("attachment is empty")
	}
	if info.Size() > maxBytes {
		return "", fmt.Errorf("attachment is too large: %d bytes exceeds %d-byte limit", info.Size(), maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read attachment: %w", err)
	}
	mimeType := http.DetectContentType(data)
	// DetectContentType reports SVG as text/xml; the extension is safe to use
	// here because SVG is an explicitly supported image format.
	if strings.EqualFold(filepath.Ext(path), ".svg") && strings.Contains(strings.ToLower(string(data)), "<svg") {
		mimeType = "image/svg+xml"
	}
	if imageOnly && !strings.HasPrefix(mimeType, "image/") {
		return "", fmt.Errorf("unsupported image MIME type %q", mimeType)
	}
	if !imageOnly && mimeType != "application/pdf" && mimeType != "text/plain" && mimeType != "text/markdown" {
		return "", fmt.Errorf("unsupported document MIME type %q", mimeType)
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// UploadFile uploads a document through the OpenAI-compatible /v1/files API.
// The endpoint is optional across llama.cpp-compatible servers; callers should
// treat a 404 as an unsupported file feature and may use FileDataURL instead.
func (c *Client) UploadFile(ctx context.Context, filename string, content io.Reader) (*UploadedFile, error) {
	if strings.TrimSpace(filename) == "" || content == nil {
		return nil, errors.New("filename and content are required")
	}
	return c.uploadFile(ctx, filename, content)
}

// UploadedFile identifies a document accepted by the server's file API.
type UploadedFile struct {
	// ID is the server-assigned identifier used to reference the uploaded file.
	ID string `json:"id"`
	// Object identifies the API resource type returned by the server.
	Object string `json:"object,omitempty"`
	// Filename is the basename associated with the uploaded content.
	Filename string `json:"filename,omitempty"`
}

func (c *Client) uploadFile(ctx context.Context, filename string, content io.Reader) (*UploadedFile, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, content); err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/files", bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upload file returned %d", resp.StatusCode)
	}
	var result UploadedFile
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode uploaded file: %w", err)
	}
	if result.ID == "" {
		return nil, errors.New("upload response did not include a file ID")
	}
	if result.Filename == "" {
		result.Filename = filepath.Base(filename)
	}
	return &result, nil
}

// DeleteFile removes a previously uploaded file by server-assigned ID.
func (c *Client) DeleteFile(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("file ID is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.endpoint+"/v1/files/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.clientHTTP().Do(req)
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("delete file returned %d", resp.StatusCode)
	}
	return nil
}
