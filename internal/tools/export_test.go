package tools

// Export internal functions for testing.

var ValidateAttachments = validateAttachments //nolint:gochecknoglobals // test export

var BuildDraftMessage = buildDraftMessage //nolint:gochecknoglobals // test export

var HTMLToText = htmlToText //nolint:gochecknoglobals // test export

var ExtractEmailAddress = extractEmailAddress //nolint:gochecknoglobals // test export

var DraftContentIsHTML = draftContentIsHTML //nolint:gochecknoglobals // test export

var FormatBytes = formatBytes //nolint:gochecknoglobals // test export

var AppendDroppedNote = appendDroppedNote //nolint:gochecknoglobals // test export

var EnsureTargetIncluded = ensureTargetIncluded //nolint:gochecknoglobals // test export
