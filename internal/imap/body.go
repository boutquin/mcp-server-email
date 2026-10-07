package imap

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	_ "github.com/emersion/go-message/charset" // Register charset conversion to UTF-8.
	"github.com/emersion/go-message/mail"
)

// decodeMessageBody selects the first inline text body, excluding attachments.
// The MIME reader decodes transfer encoding and converts the charset to UTF-8.
func decodeMessageBody(raw []byte) (string, string, error) {
	reader, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return "", "", fmt.Errorf("read MIME message: %w", err)
	}

	defer func() { _ = reader.Close() }()

	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			return "", "", nil
		}

		if partErr != nil {
			return "", "", fmt.Errorf("read MIME part: %w", partErr)
		}

		header, inline := part.Header.(*mail.InlineHeader)
		if !inline {
			continue
		}

		contentType, _, typeErr := header.ContentType()
		if typeErr != nil {
			return "", "", fmt.Errorf("read MIME content type: %w", typeErr)
		}

		if contentType != "text/plain" && contentType != "text/html" {
			continue
		}

		body, bodyErr := io.ReadAll(part.Body)
		if bodyErr != nil {
			return "", "", fmt.Errorf("decode MIME body: %w", bodyErr)
		}

		return string(body), contentType, nil
	}
}
