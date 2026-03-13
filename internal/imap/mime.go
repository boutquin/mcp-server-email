package imap

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"

	"github.com/boutquin/mcp-server-email/internal/models"
	goimap "github.com/emersion/go-imap/v2"
)

// Static errors for MIME extraction operations.
var (
	errNotMultipart       = errors.New("message is not multipart")
	errNoBoundary         = errors.New("no boundary in multipart message")
	errAttachmentNotFound = errors.New("attachment not found in message parts")
)

// extractContentType returns the primary content type from a body structure.
// For multipart messages, it looks for the first text part.
func extractContentType(bs goimap.BodyStructure) string {
	switch part := bs.(type) {
	case *goimap.BodyStructureSinglePart:
		return strings.ToLower(part.Type + "/" + part.Subtype)
	case *goimap.BodyStructureMultiPart:
		// Look for the first text part in multipart messages
		for _, child := range part.Children {
			if sp, ok := child.(*goimap.BodyStructureSinglePart); ok {
				if strings.EqualFold(sp.Type, "text") {
					return strings.ToLower(sp.Type + "/" + sp.Subtype)
				}
			}
		}
		// Recurse into nested multipart
		for _, child := range part.Children {
			ct := extractContentType(child)
			if ct != "" {
				return ct
			}
		}
	}

	return ""
}

func extractAttachments(bs goimap.BodyStructure) []models.AttachmentInfo {
	var attachments []models.AttachmentInfo

	extractAttachmentsRecursive(bs, &attachments)

	return attachments
}

func extractAttachmentsRecursive(bs goimap.BodyStructure, out *[]models.AttachmentInfo) {
	switch part := bs.(type) {
	case *goimap.BodyStructureSinglePart:
		disp := part.Disposition()
		if disp != nil && strings.EqualFold(disp.Value, "attachment") {
			att := models.AttachmentInfo{
				Index:       len(*out),
				ContentType: part.Type + "/" + part.Subtype,
				Size:        int64(part.Size),
			}
			if name, ok := disp.Params["filename"]; ok {
				att.Filename = name
			} else if name, ok := part.Params["name"]; ok {
				att.Filename = name
			}

			*out = append(*out, att)
		}

	case *goimap.BodyStructureMultiPart:
		for _, child := range part.Children {
			extractAttachmentsRecursive(child, out)
		}
	}
}

// extractAttachmentByIndex parses a raw RFC 5322 message, walks its MIME parts,
// and returns the decoded body of the Nth attachment (0-indexed).
// Only parts with Content-Disposition: attachment are counted.
func extractAttachmentByIndex(rawMsg []byte, index int) ([]byte, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(rawMsg))
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("parse content-type: %w", err)
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, fmt.Errorf("%w: %s", errNotMultipart, mediaType)
	}

	boundary, ok := params["boundary"]
	if !ok {
		return nil, errNoBoundary
	}

	return walkMultipartForAttachment(msg.Body, boundary, index)
}

// walkMultipartForAttachment reads multipart parts, counting attachment-disposition
// parts, and returns the body of the one at the target index.
func walkMultipartForAttachment(body io.Reader, boundary string, targetIndex int) ([]byte, error) {
	reader := multipart.NewReader(body, boundary)
	attachIndex := 0

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("read multipart part: %w", err)
		}

		// Recurse into nested multipart.
		if data, found := tryNestedMultipart(part, targetIndex-attachIndex); found {
			return data, nil
		}

		if !isAttachmentPart(part) {
			continue
		}

		if attachIndex == targetIndex {
			r := decodeMIMEBody(part, part.Header.Get("Content-Transfer-Encoding"))

			data, readErr := io.ReadAll(r)
			if readErr != nil {
				return nil, fmt.Errorf("read attachment body: %w", readErr)
			}

			return data, nil
		}

		attachIndex++
	}

	return nil, fmt.Errorf("%w: index %d", errAttachmentNotFound, targetIndex)
}

// tryNestedMultipart checks if a MIME part is multipart and recursively searches for attachments.
// Returns the attachment data and true if found, or nil and false otherwise.
func tryNestedMultipart(part *multipart.Part, targetIndex int) ([]byte, bool) {
	ct := part.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "multipart/") {
		return nil, false
	}

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return nil, false
	}

	nestedBoundary, ok := params["boundary"]
	if !ok {
		return nil, false
	}

	result, err := walkMultipartForAttachment(part, nestedBoundary, targetIndex)
	if err != nil {
		return nil, false
	}

	return result, true
}

// isAttachmentPart returns true if the MIME part has Content-Disposition: attachment.
func isAttachmentPart(part *multipart.Part) bool {
	disp := part.Header.Get("Content-Disposition")
	if disp == "" {
		return false
	}

	dispType, _, _ := mime.ParseMediaType(disp)

	return strings.EqualFold(dispType, "attachment")
}

// decodeMIMEBody wraps a reader with the appropriate decoder for the given
// Content-Transfer-Encoding (base64, quoted-printable, or passthrough).
func decodeMIMEBody(r io.Reader, encoding string) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	default:
		return r
	}
}
