package matcha

import (
	"bytes"
	"html/template"
	"io"
	"path"
	"strings"

	"github.com/labstack/echo"
	"github.com/shurcooL/octiconssvg"
	nethtml "golang.org/x/net/html"
)

const pgpSigEndTag = "-----END PGP SIGNATURE-----"

func cleanupCommitMessage(msg string) string {
	if i := strings.Index(msg, pgpSigEndTag); i >= 0 {
		msg = msg[i+len(pgpSigEndTag):]
	}
	return msg
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

type templateRenderer struct {
	templates *template.Template
}

func (r *templateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

func loadTemplateRenderer() (echo.Renderer, error) {
	funcs := template.FuncMap{"icon": func(name string) template.HTML {
		var b bytes.Buffer
		nethtml.Render(&b, octiconssvg.Icon(name))
		return template.HTML(b.String())
	}}

	t, err := template.New("").Funcs(funcs).ParseGlob("public/views/*.html")
	if err != nil {
		return nil, err
	}

	return &templateRenderer{t}, nil
}
