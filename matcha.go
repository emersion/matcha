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

type server struct {
	dir string
	r *git.Repository
}

func (s *server) tree(c echo.Context, branch, p string) error {
	if branch != "master" {
		// TODO
		return c.String(http.StatusNotFound, "No such branch")
	}
	if p == "" {
		p = "/"
	}

	ref, err := s.r.Head()
	if err != nil {
		return err
	}

	commit, err := s.r.CommitObject(ref.Hash())
	if err != nil {
		return err
	}

	tree, err := commit.Tree()
	if err != nil {
		return err
	}

	if p != "/" {
		tree, err = tree.Tree(p)
		if err == object.ErrDirectoryNotFound {
			return c.String(http.StatusNotFound, "No such directory")
		} else if err != nil {
			return err
		}
	}

	var data struct{
		RepoName string
		Dir, DirSep string
		Entries []object.TreeEntry
	}

	data.RepoName = filepath.Base(s.dir)
	data.Dir = p
	data.Entries = tree.Entries

	data.DirSep = "/"+data.Dir+"/"
	if data.Dir == "/" {
		data.DirSep = "/"
	}

	return c.Render(http.StatusOK, "tree-dir.html", data)
}

func (s *server) blob(c echo.Context, branch, p string) error {
	if branch != "master" {
		// TODO
		return c.String(http.StatusNotFound, "No such branch")
	}

	ref, err := s.r.Head()
	if err != nil {
		return err
	}

	commit, err := s.r.CommitObject(ref.Hash())
	if err != nil {
		return err
	}

	f, err := commit.File(p)
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

	s := &server{dir, r}

	e.GET("/", func(c echo.Context) error {
		return s.tree(c, "master", "/")
	})

	e.GET("/tree/:ref/*", func(c echo.Context) error {
		return s.tree(c, c.Param("ref"), c.Param("*"))
	})

	e.GET("/blob/:ref/*", func(c echo.Context) error {
		return s.blob(c, c.Param("ref"), c.Param("*"))
	})

	e.Static("/static", "public/node_modules")

	return nil
}
