class McpServerEmail < Formula
  desc "Multi-account IMAP/SMTP email MCP server"
  homepage "https://github.com/boutquin/mcp-server-email"
  version "VERSION"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/boutquin/mcp-server-email/releases/download/vVERSION/mcp-server-email_VERSION_darwin_arm64.tar.gz"
      sha256 "SHA256"
    else
      url "https://github.com/boutquin/mcp-server-email/releases/download/vVERSION/mcp-server-email_VERSION_darwin_amd64.tar.gz"
      sha256 "SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/boutquin/mcp-server-email/releases/download/vVERSION/mcp-server-email_VERSION_linux_arm64.tar.gz"
      sha256 "SHA256"
    else
      url "https://github.com/boutquin/mcp-server-email/releases/download/vVERSION/mcp-server-email_VERSION_linux_amd64.tar.gz"
      sha256 "SHA256"
    end
  end

  def install
    bin.install "mcp-server-email"
  end

  test do
    assert_match "mcp-server-email", shell_output("#{bin}/mcp-server-email --version 2>&1", 0)
  end
end
