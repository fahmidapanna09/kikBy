package auth

import (
	"html"
	"strings"
	"time"
)

// emailSpec describes one transactional e-mail. Every field is plain text: renderEmail HTML-escapes it.
type emailSpec struct {
	Preheader string // grey preview line shown next to the subject in the inbox
	Title     string // big heading inside the card
	Intro     string // one short paragraph under the heading
	Code      string // one-time code shown in a large box (optional; used instead of a button)
	Button    string // call-to-action label
	Link      string // call-to-action URL (also printed as a copy-paste fallback)
	Expiry    string // e.g. "This link expires in 24 hours." (optional)
	Tip       string // small boxed security tip (optional)
	Footer    string // "didn't request this?" line (optional)
	ToEmail   string // shown in the footer so people know why they got it (optional)
}

const (
	brandName   = "KikBite"
	brandColor  = "#4f46e5"
	brandColor2 = "#7c3aed"
)

// renderEmail returns a table-based, inline-styled HTML e-mail that renders the same in Gmail, Outlook,
// Apple Mail and mobile clients. The button is "bulletproof" (VML fallback for desktop Outlook), there is a
// hidden preheader, a dark-mode override, and a copy-paste fallback link. No images are needed.
func renderEmail(s emailSpec) string {
	e := html.EscapeString
	var b strings.Builder
	w := func(parts ...string) {
		for _, p := range parts {
			b.WriteString(p)
		}
	}

	w(`<!doctype html><html lang="en" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office"><head>`,
		`<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`,
		`<meta name="color-scheme" content="light dark"><meta name="supported-color-schemes" content="light dark">`,
		`<title>`, e(s.Title), `</title>`,
		`<!--[if mso]><noscript><xml><o:OfficeDocumentSettings><o:PixelsPerInch>96</o:PixelsPerInch></o:OfficeDocumentSettings></xml></noscript><![endif]-->`,
		`<style>`,
		`body{margin:0;padding:0;width:100%!important;-webkit-text-size-adjust:100%;-ms-text-size-adjust:100%}`,
		`table{border-collapse:collapse}img{border:0;line-height:100%;outline:none;text-decoration:none}`,
		`a{color:`, brandColor, `}`,
		`@media only screen and (max-width:600px){.wrap{padding:16px 12px!important}.card{padding:28px 22px!important;border-radius:14px!important}.h1{font-size:22px!important}}`,
		`@media (prefers-color-scheme:dark){`,
		`.bg{background:#0f1115!important}.card{background:#181b22!important;border-color:#2a2e38!important}`,
		`.txt{color:#e7e9ee!important}.muted{color:#a3a9b7!important}.tip{background:#20243a!important;border-color:#33385a!important;color:#cfd3f7!important}.rule{border-top-color:#2a2e38!important}`,
		`.codebox{background:#20243a!important;border-color:#33385a!important;color:#ffffff!important}`,
		`.fallback{background:#20232c!important;color:#c9cedb!important}}`,
		`</style></head>`)

	w(`<body class="bg" style="margin:0;padding:0;background:#f4f5f8;">`)

	// hidden preheader (plus padding so the client doesn't pull in body text after it)
	if s.Preheader != "" {
		w(`<div style="display:none;max-height:0;overflow:hidden;opacity:0;mso-hide:all;font-size:1px;line-height:1px;color:#f4f5f8;">`,
			e(s.Preheader), strings.Repeat("&#847;&zwnj;&nbsp;", 40), `</div>`)
	}

	w(`<table role="presentation" class="bg" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f4f5f8;"><tr>`,
		`<td align="center" class="wrap" style="padding:40px 16px;">`,
		`<table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:560px;">`)

	// brand header
	w(`<tr><td align="left" style="padding:0 4px 20px;">`,
		`<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>`,
		`<td width="36" height="36" align="center" valign="middle" style="width:36px;height:36px;border-radius:10px;background:`, brandColor, `;background-image:linear-gradient(135deg,`, brandColor, `,`, brandColor2, `);`,
		`font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:19px;font-weight:800;color:#ffffff;line-height:36px;">K</td>`,
		`<td style="padding-left:10px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:19px;font-weight:700;color:#1b1d21;" class="txt">`, e(brandName), `</td>`,
		`</tr></table></td></tr>`)

	// card
	w(`<tr><td class="card" style="background:#ffffff;border:1px solid #e6e8ee;border-radius:16px;padding:36px 36px 32px;`,
		`font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">`)

	w(`<h1 class="h1 txt" style="margin:0 0 12px;font-size:24px;line-height:1.3;font-weight:700;color:#1b1d21;">`, e(s.Title), `</h1>`)
	w(`<p class="txt" style="margin:0 0 28px;font-size:16px;line-height:1.6;color:#3b3f4a;">`, e(s.Intro), `</p>`)

	// one-time code
	if s.Code != "" {
		w(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 8px;"><tr>`,
			`<td align="center" class="codebox" style="background:#f3f4ff;border:1px solid #dfe1fb;border-radius:12px;padding:18px 12px 18px 22px;`,
			`font-family:'SFMono-Regular',Consolas,'Liberation Mono',Menlo,monospace;font-size:34px;line-height:1.2;font-weight:700;letter-spacing:10px;color:#1b1d21;">`,
			e(s.Code), `</td></tr></table>`)
	}

	// bulletproof button
	if s.Link != "" && s.Button != "" {
		w(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 8px;"><tr><td>`,
			`<!--[if mso]><v:roundrect xmlns:v="urn:schemas-microsoft-com:vml" href="`, e(s.Link), `" style="height:48px;v-text-anchor:middle;width:240px;" arcsize="17%" stroke="f" fillcolor="`, brandColor, `">`,
			`<w:anchorlock xmlns:w="urn:schemas-microsoft-com:office:word"/><center style="color:#ffffff;font-family:Arial,sans-serif;font-size:16px;font-weight:bold;">`, e(s.Button), `</center></v:roundrect><![endif]-->`,
			`<!--[if !mso]><!-- --><a href="`, e(s.Link), `" target="_blank" style="display:inline-block;min-width:200px;text-align:center;background:`, brandColor, `;`,
			`background-image:linear-gradient(135deg,`, brandColor, `,`, brandColor2, `);color:#ffffff;text-decoration:none;font-size:16px;font-weight:600;line-height:1;`,
			`padding:16px 28px;border-radius:10px;">`, e(s.Button), `</a><!--<![endif]-->`,
			`</td></tr></table>`)
	}

	if s.Expiry != "" {
		w(`<p class="muted" style="margin:12px 0 0;font-size:13px;line-height:1.5;color:#6b7280;">`, e(s.Expiry), `</p>`)
	}

	// security tip
	if s.Tip != "" {
		w(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:28px 0 0;"><tr>`,
			`<td class="tip" style="background:#f3f4ff;border:1px solid #dfe1fb;border-radius:10px;padding:14px 16px;font-size:13px;line-height:1.55;color:#3b3f6a;">`,
			`<strong style="color:`, brandColor, `;">Security tip &nbsp;</strong>`, e(s.Tip), `</td></tr></table>`)
	}

	// fallback link
	if s.Link != "" {
		w(`<hr class="rule" style="border:none;border-top:1px solid #eceef3;margin:28px 0 20px;">`,
			`<p class="muted" style="margin:0 0 8px;font-size:13px;line-height:1.5;color:#6b7280;">Button not working? Copy and paste this link into your browser:</p>`,
			`<p class="fallback" style="margin:0;padding:10px 12px;background:#f6f7fa;border-radius:8px;font-size:12px;line-height:1.5;color:#4b5060;word-break:break-all;">`, e(s.Link), `</p>`)
	}

	if s.Footer != "" {
		w(`<p class="muted" style="margin:20px 0 0;font-size:13px;line-height:1.5;color:#6b7280;">`, e(s.Footer), `</p>`)
	}

	w(`</td></tr>`)

	// footer outside the card
	w(`<tr><td align="center" class="muted" style="padding:22px 12px 0;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;line-height:1.6;color:#8b90a0;">`)
	if s.ToEmail != "" {
		w(`This e-mail was sent to `, e(s.ToEmail), `.<br>`)
	}
	w(`&copy; `, e(time.Now().UTC().Format("2006")), ` `, e(brandName), `. All rights reserved.<br>`,
		`This is an automated message, please don't reply.</td></tr>`)

	w(`</table></td></tr></table></body></html>`)
	return b.String()
}
