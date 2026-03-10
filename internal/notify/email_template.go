package notify

import (
	"bytes"
	"html/template"
)

var emailTemplate = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f5f5f5">
<table width="100%" cellpadding="0" cellspacing="0" style="background:#f5f5f5;padding:20px 0">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1)">
  <tr>
    <td style="background:{{if .IsSuccess}}#16a34a{{else if .IsCancelled}}#6b7280{{else}}#dc2626{{end}};padding:16px 24px">
      <h1 style="margin:0;color:#fff;font-size:20px">
        {{if .IsSuccess}}Build Passed{{else if .IsCancelled}}Build Cancelled{{else}}Build Failed{{end}}
      </h1>
    </td>
  </tr>
  <tr>
    <td style="padding:24px">
      <table width="100%" cellpadding="4" cellspacing="0" style="font-size:14px;color:#374151">
        <tr>
          <td style="font-weight:600;width:120px;vertical-align:top">Project</td>
          <td>{{.ProjectName}}</td>
        </tr>
        <tr>
          <td style="font-weight:600;vertical-align:top">Build</td>
          <td>#{{.BuildNumber}}</td>
        </tr>
        <tr>
          <td style="font-weight:600;vertical-align:top">Branch</td>
          <td>{{.Branch}}</td>
        </tr>
        <tr>
          <td style="font-weight:600;vertical-align:top">Commit</td>
          <td><code style="background:#f3f4f6;padding:2px 6px;border-radius:4px;font-size:13px">{{.ShortSHA}}</code></td>
        </tr>
        <tr>
          <td style="font-weight:600;vertical-align:top">Message</td>
          <td>{{.CommitMessage}}</td>
        </tr>
        <tr>
          <td style="font-weight:600;vertical-align:top">Author</td>
          <td>{{.CommitAuthor}}</td>
        </tr>
        <tr>
          <td style="font-weight:600;vertical-align:top">Duration</td>
          <td>{{.DurationString}}</td>
        </tr>
      </table>
      {{if .BuildURL}}
      <div style="margin-top:20px">
        <a href="{{.BuildURL}}" style="display:inline-block;background:#4f46e5;color:#fff;padding:10px 20px;border-radius:6px;text-decoration:none;font-size:14px;font-weight:500">View Build</a>
      </div>
      {{end}}
    </td>
  </tr>
  <tr>
    <td style="padding:16px 24px;background:#f9fafb;font-size:12px;color:#9ca3af;text-align:center">
      Sent by FeatherCI
    </td>
  </tr>
</table>
</td></tr>
</table>
</body>
</html>`))

// renderEmailHTML renders the email template for a build event.
func renderEmailHTML(event BuildEvent) (string, error) {
	var buf bytes.Buffer
	if err := emailTemplate.Execute(&buf, event); err != nil {
		return "", err
	}
	return buf.String(), nil
}
