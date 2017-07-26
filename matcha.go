package matcha

import (
	"html/template"
	"io"
	"net/http"

	"github.com/labstack/echo"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

type templateRenderer struct {
	templates *template.Template
}

func (r *templateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

func New(e *echo.Echo, dir string) {
	t := template.Must(template.ParseGlob("public/views/*.html"))
	e.Renderer = &templateRenderer{t}

	e.GET("/", func(c echo.Context) error {
		r, err := git.PlainOpen(dir)
		if err == git.ErrRepositoryNotExists {
			return c.String(http.StatusNotFound, "No such repository")
		} else if err != nil {
			return err
		}

		ref, err := r.Head()
		if err != nil {
			return err
		}

		commit, err := r.CommitObject(ref.Hash())
		if err != nil {
			return err
		}

		tree, err := commit.Tree()
		if err != nil {
			return err
		}

		var data struct{
			RepoName string
			Files []string
		}

		data.RepoName = dir

		err = tree.Files().ForEach(func(f *object.File) error {
			data.Files = append(data.Files, f.Name)
			return nil
		})
		if err != nil {
			return err
		}

		return c.Render(http.StatusOK, "tree-dir.html", data)
	})

	e.GET("/blob/:branch/*", func(c echo.Context) error {
		// TODO
		return nil
	})

	e.Static("/static", "public/node_modules")
}
