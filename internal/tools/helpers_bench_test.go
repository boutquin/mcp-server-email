package tools //nolint:testpackage // benchmark needs access to unexported htmlToText

import "testing"

// marketingHTML returns a realistic marketing email HTML body for benchmarking.
func marketingHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
<style>
  body { font-family: Arial, sans-serif; margin: 0; padding: 0; }
  .container { max-width: 600px; margin: auto; }
  .header { background-color: #4A90D9; color: white; padding: 20px; }
  .footer { font-size: 12px; color: #999; padding: 10px; }
</style>
</head>
<body>
<table width="100%" cellpadding="0" cellspacing="0">
  <tr>
    <td class="header">
      <h2>Weekly Newsletter - February 2026</h2>
    </td>
  </tr>
  <tr>
    <td>
      <p>Dear Subscriber,</p>
      <p>We are excited to announce our <b>Spring Collection</b> launch!</p>
      <img src="https://example.com/banner.png" alt="Spring Banner" />
      <h2>Featured Products</h2>
      <table>
        <tr><td>Product A</td><td>$29.99</td></tr>
        <tr><td>Product B</td><td>$49.99</td></tr>
        <tr><td>Product C</td><td>$19.99</td></tr>
      </table>
      <p>Visit our store: <a href="https://example.com/store">Shop Now</a></p>
      <br/>
      <p>Follow us on social media:</p>
      <a href="https://twitter.com/example">Twitter</a> |
      <a href="https://facebook.com/example">Facebook</a>
    </td>
  </tr>
  <tr>
    <td class="footer">
      <p>You received this email because you subscribed to our newsletter.</p>
      <p><a href="https://example.com/unsubscribe">Unsubscribe</a></p>
    </td>
  </tr>
</table>
</body>
</html>`
}

func BenchmarkHtmlToText(b *testing.B) {
	input := marketingHTML()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = htmlToText(input)
	}
}
