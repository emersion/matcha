package matcha

import (
	"html/template"
	"io"
	"net/http"
	"path/filepath"

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

func New(e *echo.Echo, dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	t := template.Must(template.ParseGlob("public/views/*.html"))
	e.Renderer = &templateRenderer{t}

	r, err := git.PlainOpen(dir)
	if err == git.ErrRepositoryNotExists {
		return err //return c.String(http.StatusNotFound, "No such repository")
	} else if err != nil {
		return err
	}

	e.GET("/", func(c echo.Context) error {
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

		data.RepoName = filepath.Base(dir)

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
		if branch := c.Param("branch"); branch != "master" {
			// TODO
			return c.String(http.StatusNotFound, "No such branch")
		}
		path := c.Param("*")

		ref, err := r.Head()
		if err != nil {
			return err
		}

		commit, err := r.CommitObject(ref.Hash())
		if err != nil {
			return err
		}

		f, err := commit.File(path)
		if err == object.ErrFileNotFound {
			return c.String(http.StatusNotFound, "No such file")
		} else if err != nil {
			return err
		}

		r, err := f.Reader()
		if err != nil {
			return err
		}
		defer r.Close()

		// TODO: autodetect file type
		mediaType := "application/octet-stream"
		if binary, err := f.IsBinary(); err == nil && !binary {
			mediaType = "text/plain"
		}

		// TODO: set filename
		return c.Stream(http.StatusOK, mediaType, r)
	})

	e.Static("/static", "public/node_modules")

	return nil
}
