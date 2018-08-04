package matcha

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"path"
	"strings"
	"time"

	"github.com/labstack/echo"
	octiconssvg "github.com/shurcooL/octiconssvg"
	nethtml "golang.org/x/net/html"
)

var publicDir = "public"

const pgpSigEndTag = "-----END PGP SIGNATURE-----"

func cleanupCommitMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if i := strings.Index(msg, pgpSigEndTag); i >= 0 {
		msg = msg[i+len(pgpSigEndTag):]
	}
	return msg
}

func splitCommitMessage(msg string) (summary string, description string) {
	msg = cleanupCommitMessage(msg)
	parts := strings.SplitN(msg, "\n", 2)
	summary = strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		return summary, ""
	}
	return summary, strings.TrimSpace(parts[1])
}

type breadcumbItem struct {
	Name string
	Path string
}

func pathBreadcumb(p string) []breadcumbItem {
	var breadcumb []breadcumbItem
	if p := strings.Trim(p, "/"); p != "" {
		names := strings.Split(p, "/")
		breadcumb = make([]breadcumbItem, len(names))
		for i, name := range names {
			breadcumb[i] = breadcumbItem{
				Name: name,
				Path: path.Join(names[:i+1]...),
			}
		}
	}
	return breadcumb
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", d/time.Second)
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", d/time.Minute)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", d/time.Hour)
	}
	return fmt.Sprintf("%d days", d/(24*time.Hour))
}

func formatDate(t time.Time, d time.Duration) string {
	if d < 365*24*time.Hour { // 1 year
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}

type templateRenderer struct {
	templates *template.Template
}

func (r *templateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

func loadTemplateRenderer() (echo.Renderer, error) {
	funcs := template.FuncMap{
		"icon": func(name string) template.HTML {
			var b bytes.Buffer
			nethtml.Render(&b, octiconssvg.Icon(name))
			return template.HTML(b.String())
		},
		"date": func(t time.Time) template.HTML {
			d := time.Since(t)

			var s string
			if d >= 0 && d < 30*24*time.Hour { // 1 month
				s = formatDuration(d) + " ago"
			} else {
				s = "on " + formatDate(t, d)
			}

			full := t.Format("Jan 02, 2006, 15:04 -0700")
			s = `<relative-time datetime="` + t.Format(time.RFC3339) + `" title="` + full + `">` + s + `</relative-time>`
			return template.HTML(s)
		},
		"commitSummary": func(msg string) string {
			summary, _ := splitCommitMessage(msg)
			return summary
		},
		"commitDescription": func(msg string) string {
			_, description := splitCommitMessage(msg)
			return description
		},
		"newlines": func(s string) template.HTML {
			return template.HTML(strings.Replace(template.HTMLEscapeString(s), "\n", "<br>", -1))
		},
	}

	t, err := template.New("").Funcs(funcs).ParseGlob(publicDir + "/views/*.html")
	if err != nil {
		return nil, err
	}

	return &templateRenderer{t}, nil
}
