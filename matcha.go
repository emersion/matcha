package matcha

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo"
	"github.com/shurcooL/octiconssvg"
	nethtml "golang.org/x/net/html"
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
		DirName, DirSep string
		Parents []string
		Entries []object.TreeEntry
	}

	data.RepoName = filepath.Base(s.dir)
	data.Entries = tree.Entries

	dir, file := path.Split(p)
	data.DirName = file
	if dir := strings.Trim(dir, "/"); dir != "" {
		data.Parents = strings.Split(dir, "/")
	}

	data.DirSep = "/"+p+"/"
	if p == "/" {
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

	funcs := template.FuncMap{"icon": func(name string) template.HTML {
		var b bytes.Buffer
		nethtml.Render(&b, octiconssvg.Icon(name))
		return template.HTML(b.String())
	}}
	t := template.Must(template.New("").Funcs(funcs).ParseGlob("public/views/*.html"))
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
