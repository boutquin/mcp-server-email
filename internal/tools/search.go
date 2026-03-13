package tools

import (
	"context"
	"time"

	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SearchTool returns the email_search tool definition.
func SearchTool() mcp.Tool {
	return mcp.NewTool("email_search",
		mcp.WithDescription("Search messages by subject and body"),
		mcp.WithString("query", mcp.Description("Search query (subject and body)"), mcp.Required()),
		mcp.WithString("account", mcp.Description("Account ID (defaults to default account)")),
		mcp.WithString("folder", mcp.Description("Folder to search (default: INBOX)")),
		mcp.WithString("from", mcp.Description("Filter by sender")),
		mcp.WithString("to", mcp.Description("Filter by recipient")),
		mcp.WithString("since", mcp.Description("Messages since date (YYYY-MM-DD)")),
		mcp.WithString("before", mcp.Description("Messages before date (YYYY-MM-DD)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 50, max 500)")),
		mcp.WithBoolean("includeBody", mcp.Description("Include message body (default false)")),
	)
}

// validateDateRange validates since/before date strings and their ordering.
// Returns an error message if validation fails, or empty string on success.
func validateDateRange(since, before string) string {
	var sinceTime, beforeTime time.Time

	if since != "" {
		var err error

		sinceTime, err = time.Parse("2006-01-02", since)
		if err != nil {
			return "invalid 'since' date format — use YYYY-MM-DD"
		}
	}

	if before != "" {
		var err error

		beforeTime, err = time.Parse("2006-01-02", before)
		if err != nil {
			return "invalid 'before' date format — use YYYY-MM-DD"
		}
	}

	if !sinceTime.IsZero() && !beforeTime.IsZero() && !sinceTime.Before(beforeTime) {
		return "'since' date must be before 'before' date"
	}

	return ""
}

// SearchHandler returns the handler for email_search.
func SearchHandler(ops imap.Operations) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("query is required"), nil
		}

		accountID := resolveAccountID(req.GetString("account", ""), ops.DefaultAccountID())
		folder := req.GetString("folder", "INBOX")
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		since := req.GetString("since", "")
		before := req.GetString("before", "")
		limit := req.GetInt("limit", defaultLimit)
		includeBody := req.GetBool("includeBody", false)

		if limit > maxLimit {
			limit = maxLimit
		}

		if limit < 1 {
			limit = defaultLimit
		}

		if errMsg := validateDateRange(since, before); errMsg != "" {
			return mcp.NewToolResultError(errMsg), nil
		}

		emails, err := ops.Search(ctx, accountID, folder, query, from, to, since, before, limit, includeBody)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if includeBody {
			convertHTMLBodies(emails)
		}

		return jsonResult(map[string]any{
			"account":  accountID,
			"folder":   folder,
			"query":    query,
			"messages": emails,
			"count":    len(emails),
		}), nil
	}
}
