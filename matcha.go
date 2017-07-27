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
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

const pgpSigEndTag = "-----END PGP SIGNATURE-----"

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

type headerData struct {
	RepoName string
}

func (s *server) headerData() *headerData {
	return &headerData{
		RepoName: filepath.Base(s.dir),
	}
}

func (s *server) tree(c echo.Context, refName, p string) error {
	if refName != "master" {
		// TODO
		return c.String(http.StatusNotFound, "No such ref")
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

	if p == "" {
		p = "/"
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
		*headerData
		DirName, DirSep string
		Parents []string
		Entries []object.TreeEntry
	}

	data.headerData = s.headerData()
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

func (s *server) raw(c echo.Context, refName, p string) error {
	if refName != "master" {
		// TODO
		return c.String(http.StatusNotFound, "No such ref")
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

func (s *server) branches(c echo.Context) error {
	branches, err := s.r.Branches()
	if err != nil {
		return err
	}
	defer branches.Close()

	var data struct{
		*headerData
		Branches []string
	}

	data.headerData = s.headerData()

	err = branches.ForEach(func(ref *plumbing.Reference) error {
		data.Branches = append(data.Branches, ref.Name().Short())
		return nil
	})
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "branches.html", data)
}

func (s *server) commits(c echo.Context, refName string) error {
	if refName != "master" {
		// TODO
		return c.String(http.StatusNotFound, "No such ref")
	}

	commits, err := s.r.Log(&git.LogOptions{})
	if err != nil {
		return err
	}
	defer commits.Close()

	var data struct{
		*headerData
		Commits []*object.Commit
	}

	data.headerData = s.headerData()

	err = commits.ForEach(func(c *object.Commit) error {
		if i := strings.Index(c.Message, pgpSigEndTag); i >= 0 {
			c.Message = c.Message[i+len(pgpSigEndTag):]
		}

		data.Commits = append(data.Commits, c)
		return nil
	})
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "commits.html", data)
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
	e.GET("/tree/:ref", func(c echo.Context) error {
		return s.tree(c, c.Param("ref"), "")
	})
	e.GET("/tree/:ref/*", func(c echo.Context) error {
		return s.tree(c, c.Param("ref"), c.Param("*"))
	})
	e.GET("/raw/:ref/*", func(c echo.Context) error {
		return s.raw(c, c.Param("ref"), c.Param("*"))
	})
	e.GET("/branches", s.branches)
	e.GET("/commits/:ref", func(c echo.Context) error {
		return s.commits(c, c.Param("ref"))
	})

	e.Static("/static", "public/node_modules")

	return nil
}
