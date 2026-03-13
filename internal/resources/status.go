package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/imap"
	"github.com/mark3labs/mcp-go/mcp"
)

// Version is set at build time via ldflags (-X main.version), or falls back to
// debug.ReadBuildInfo, or "dev" for local builds.
var Version = resolveVersion() //nolint:gochecknoglobals // set by ldflags or build info

func resolveVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}

	return "dev"
}

type accountStatusInfo struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Connected bool   `json:"connected"`
}

type rateLimitIMAPInfo struct {
	RequestsPerMinute int `json:"requestsPerMinute"`
}

type rateLimitSMTPInfo struct {
	SendsPerHour int `json:"sendsPerHour"`
}

type rateLimitInfo struct {
	IMAP rateLimitIMAPInfo `json:"imap"`
	SMTP rateLimitSMTPInfo `json:"smtp"`
}

type statusInfo struct {
	Server         string              `json:"server"`
	Version        string              `json:"version"`
	Accounts       []accountStatusInfo `json:"accounts"`
	DefaultAccount string              `json:"defaultAccount"`
	RateLimit      rateLimitInfo       `json:"rateLimit"`
	Runtime        string              `json:"runtime"`
}

// StatusResource returns the email://status resource definition.
func StatusResource() mcp.Resource {
	return mcp.NewResource(
		"email://status",
		"Email MCP Server Status",
		mcp.WithResourceDescription("Current connection status, accounts, and server version"),
		mcp.WithMIMEType("application/json"),
	)
}

// StatusHandler returns the handler for the status resource.
func StatusHandler(
	cfg *config.Config, ops imap.Operations,
) func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		statuses := ops.AccountStatus()

		accounts := make([]accountStatusInfo, 0, len(statuses))
		for _, s := range statuses {
			accounts = append(accounts, accountStatusInfo{
				ID:        s.ID,
				Email:     s.Email,
				Connected: s.Connected,
			})
		}

		info := statusInfo{
			Server:         "mcp-server-email",
			Version:        Version,
			Accounts:       accounts,
			DefaultAccount: cfg.DefaultAccount,
			RateLimit: rateLimitInfo{
				IMAP: rateLimitIMAPInfo{RequestsPerMinute: cfg.IMAPRateLimitRPM},
				SMTP: rateLimitSMTPInfo{SendsPerHour: cfg.SMTPRateLimitRPH},
			},
			Runtime: fmt.Sprintf("Go %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH),
		}

		b, _ := json.MarshalIndent(info, "", "  ")

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "email://status",
				Text:     string(b),
				MIMEType: "application/json",
			},
		}, nil
	}
}
