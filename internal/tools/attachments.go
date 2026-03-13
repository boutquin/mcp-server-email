package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/mark3labs/mcp-go/mcp"
)

const bytesPerMB = 1024 * 1024

// Attachment validation errors.
var (
	errAttachmentPathRequired  = errors.New("path is required")
	errAttachmentPathAbsolute  = errors.New("path must be absolute")
	errAttachmentNotFound      = errors.New("file not found")
	errAttachmentNotRegular    = errors.New("not a regular file")
	errAttachmentTooLarge      = errors.New("exceeds size limit")
	errAttachmentTotalTooLarge = errors.New("total attachment size exceeds limit")
)

// AttachmentInput represents a file attachment provided by the caller.
type AttachmentInput struct {
	Path        string `json:"path"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// parseAttachments extracts and validates attachment inputs from a tool request.
// Returns nil if no attachments are present.
func parseAttachments(
	req mcp.CallToolRequest, maxFileSizeBytes, maxTotalSizeBytes int64,
) ([]smtp.SendAttachment, error) {
	args := req.GetArguments()

	raw, ok := args["attachments"]
	if !ok || raw == nil {
		return nil, nil
	}

	// Re-marshal then unmarshal to get proper typed structs.
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid attachments: %w", err)
	}

	var inputs []AttachmentInput

	err = json.Unmarshal(data, &inputs)
	if err != nil {
		return nil, fmt.Errorf("invalid attachments: %w", err)
	}

	if len(inputs) == 0 {
		return nil, nil
	}

	err = validateAttachments(inputs, maxFileSizeBytes, maxTotalSizeBytes)
	if err != nil {
		return nil, err
	}

	result := make([]smtp.SendAttachment, len(inputs))
	for i := range inputs {
		if inputs[i].Filename == "" {
			inputs[i].Filename = filepath.Base(inputs[i].Path)
		}

		result[i] = smtp.SendAttachment{
			Path:     inputs[i].Path,
			Filename: inputs[i].Filename,
		}
	}

	return result, nil
}

// validateAttachments validates a slice of attachment inputs.
// It checks that each path is absolute, exists, is a regular file, and is within
// the size limits. maxFileSizeBytes is the per-file limit and maxTotalSizeBytes
// is the cumulative limit for all attachments.
func validateAttachments(
	attachments []AttachmentInput, maxFileSizeBytes, maxTotalSizeBytes int64,
) error {
	var totalSize int64

	for i, att := range attachments {
		if att.Path == "" {
			return fmt.Errorf("attachment %d: %w", i, errAttachmentPathRequired)
		}

		if !filepath.IsAbs(att.Path) {
			return fmt.Errorf("attachment %d: %w, got %q", i, errAttachmentPathAbsolute, att.Path)
		}

		info, err := os.Stat(att.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("attachment %d: %w: %s", i, errAttachmentNotFound, att.Path)
			}

			return fmt.Errorf("attachment %d: %w", i, err)
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("attachment %d: %w: %s", i, errAttachmentNotRegular, att.Path)
		}

		if info.Size() > maxFileSizeBytes {
			return fmt.Errorf(
				"attachment %d: file size %d bytes exceeds %d MB limit: %w",
				i, info.Size(), maxFileSizeBytes/bytesPerMB, errAttachmentTooLarge,
			)
		}

		totalSize += info.Size()
	}

	if totalSize > maxTotalSizeBytes {
		return fmt.Errorf(
			"total attachment size %d bytes exceeds %d MB limit: %w",
			totalSize, maxTotalSizeBytes/bytesPerMB, errAttachmentTotalTooLarge,
		)
	}

	return nil
}
