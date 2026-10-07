package tools_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/boutquin/mcp-server-email/internal/models"
	"github.com/boutquin/mcp-server-email/internal/tools"
)

const (
	testDraftHTML     = "text/html"
	testDraftFilename = "sample.bin"
)

func TestDraftSendPreservesHTMLAndAttachments(t *testing.T) {
	t.Parallel()

	data := []byte{0, 1, 127, 255}
	imapMock := &mockIMAPOps{
		getMessageResult: &models.Email{
			To: []string{"self@example.com"}, Body: "<p>Salut</p>", ContentType: testDraftHTML,
		},
		getDraftResult:        []byte("Content-Type: multipart/mixed; boundary=x\r\n\r\n--x--"),
		getAttachmentsResult:  []models.AttachmentInfo{{Filename: testDraftFilename, Index: 0}},
		getAttachmentData:     data,
		getAttachmentFilename: testDraftFilename,
	}
	smtpMock := &mockSMTPOps{}

	result, err := tools.DraftSendHandler(imapMock, smtpMock)(t.Context(), toolReq(map[string]any{"id": "test:Drafts:42"}))
	if err != nil || result.IsError {
		t.Fatalf("send failed: %v, %+v", err, result)
	}

	req := smtpMock.lastSendReq
	if !req.IsHTML || len(req.Attachments) != 1 {
		t.Fatalf("HTML/attachments = %v/%d", req.IsHTML, len(req.Attachments))
	}

	if req.Attachments[0].Filename != testDraftFilename || !bytes.Equal(req.Attachments[0].Data, data) {
		t.Fatal("attachment changed")
	}
}

func TestDraftSendAttachmentFailureKeepsDraft(t *testing.T) {
	t.Parallel()

	imapMock := &mockIMAPOps{
		getMessageResult:     &models.Email{To: []string{"self@example.com"}, Body: "Salut"},
		getAttachmentsResult: []models.AttachmentInfo{{Filename: testDraftFilename, Index: 0}},
		getAttachmentErr:     io.ErrUnexpectedEOF,
	}
	smtpMock := &mockSMTPOps{}

	result, err := tools.DraftSendHandler(imapMock, smtpMock)(t.Context(), toolReq(map[string]any{"id": "test:Drafts:42"}))
	if err != nil || !result.IsError || smtpMock.sendCalled {
		t.Fatalf("attachment failure should prevent send: %v, %+v", err, result)
	}

	if imapMock.deleteDraftCalled {
		t.Fatal("draft was deleted")
	}
}
