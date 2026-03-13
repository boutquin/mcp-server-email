package imap

import (
	"testing"

	goimap "github.com/emersion/go-imap/v2"
)

// buildRealisticBodyStructure creates a multipart/mixed body structure
// with text body and three attachments for benchmarking.
func buildRealisticBodyStructure() *goimap.BodyStructureMultiPart {
	return &goimap.BodyStructureMultiPart{
		Children: []goimap.BodyStructure{
			&goimap.BodyStructureSinglePart{
				Type:    "TEXT",
				Subtype: "PLAIN",
				Size:    256,
			},
			&goimap.BodyStructureSinglePart{
				Type:    "APPLICATION",
				Subtype: "PDF",
				Size:    102400,
				Extended: &goimap.BodyStructureSinglePartExt{
					Disposition: &goimap.BodyStructureDisposition{
						Value:  "attachment",
						Params: map[string]string{"filename": "report.pdf"},
					},
				},
			},
			&goimap.BodyStructureSinglePart{
				Type:    "IMAGE",
				Subtype: "PNG",
				Size:    51200,
				Extended: &goimap.BodyStructureSinglePartExt{
					Disposition: &goimap.BodyStructureDisposition{
						Value:  "attachment",
						Params: map[string]string{"filename": "screenshot.png"},
					},
				},
			},
			&goimap.BodyStructureSinglePart{
				Type:    "APPLICATION",
				Subtype: "ZIP",
				Size:    204800,
				Extended: &goimap.BodyStructureSinglePartExt{
					Disposition: &goimap.BodyStructureDisposition{
						Value:  "attachment",
						Params: map[string]string{"filename": "archive.zip"},
					},
				},
			},
		},
	}
}

func BenchmarkExtractAttachments(b *testing.B) {
	bs := buildRealisticBodyStructure()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = extractAttachments(bs)
	}
}
