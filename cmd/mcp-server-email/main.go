package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/boutquin/mcp-server-email/internal/config"
	"github.com/boutquin/mcp-server-email/internal/imap"
	applog "github.com/boutquin/mcp-server-email/internal/log"
	"github.com/boutquin/mcp-server-email/internal/resources"
	"github.com/boutquin/mcp-server-email/internal/smtp"
	"github.com/boutquin/mcp-server-email/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

// version is set at build time by GoReleaser via ldflags:
//
//	-X main.version={{.Version}}
var version = "dev"

func main() {
	// Wire build-time version into the status resource.
	if version != "dev" {
		resources.Version = version
	}

	// Initialize structured logging from environment (LOG_LEVEL, LOG_FORMAT).
	logger := applog.Init(os.Stderr)
	slog.SetDefault(logger)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// Create connection pools
	imapPool := imap.NewPool(cfg)
	smtpPool := smtp.NewPool(cfg)

	// Create MCP server with tools and resources.
	s := newServer(cfg, imapPool, smtpPool)

	// Serve over stdio — MCP framework uses standard log.Logger for error reporting.
	stdLogger := log.New(os.Stderr, "[mcp-email] ", log.LstdFlags)

	// Signal handling for graceful shutdown
	_, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh

		logger.Info("shutting down", slog.String("signal", sig.String()))

		cancel()

		shutdownTimeout := time.Duration(cfg.PoolCloseTimeoutMS) * time.Millisecond

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		imapPool.Close(shutdownCtx)
		smtpPool.Close()

		os.Exit(0)
	}()

	err = server.ServeStdio(s, server.WithErrorLogger(stdLogger))

	cancel()

	if err != nil {
		logger.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// newServer creates the MCP server with all tools and resources registered.
func newServer(cfg *config.Config, imapPool *imap.Pool, smtpPool *smtp.Pool) *server.MCPServer {
	s := server.NewMCPServer(
		"mcp-server-email",
		resources.Version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, true),
		server.WithRecovery(),
		server.WithInstructions(
			"Email MCP server for IMAP/SMTP access. Supports multiple accounts, "+
				"full CRUD operations (read, send, move, delete, flag), folder management, and drafts. "+
				"Use email_accounts to list configured accounts. "+
				"Rate limit: "+strconv.Itoa(cfg.IMAPRateLimitRPM)+" IMAP req/min, "+
				strconv.Itoa(cfg.SMTPRateLimitRPH)+" SMTP sends/hour per account.",
		),
	)

	tools.RegisterAll(s, imapPool, smtpPool, tools.LimitsFromConfig(cfg))
	s.AddResource(resources.StatusResource(), resources.StatusHandler(cfg, imapPool))

	return s
}
