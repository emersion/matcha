package matcha

import (
	"html/template"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labstack/echo"
	"github.com/russross/blackfriday"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/filemode"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/storer"
)

func repo(c echo.Context) (*git.Repository, error) {
	v := c.Get("repo")
	if v == nil {
		return nil, echo.ErrNotFound
	}
	return v.(*git.Repository), nil
}

type server struct{}

type headerData struct {
	RepoName string
	RepoPath string
}

func (s *server) headerData(c echo.Context) *headerData {
	return &headerData{
		RepoName: c.Get("repo-name").(string),
		RepoPath: c.Get("repo-path").(string),
	}
}

func (s *server) commitFromRev(r *git.Repository, revName string) (*object.Commit, error) {
	// First try to resolve a hash
	commit, err := r.CommitObject(plumbing.NewHash(revName))
	if err != plumbing.ErrObjectNotFound {
		return commit, err
	}

	// Then a branch
	ref, err := r.Reference(plumbing.ReferenceName("refs/heads/"+revName), true)
	if err == nil {
		return r.CommitObject(ref.Hash())
	} else if err != plumbing.ErrReferenceNotFound {
		return nil, err
	}

	// Finally a tag
	ref, err = r.Reference(plumbing.ReferenceName("refs/tags/"+revName), true)
	if err != nil {
		return nil, err
	}

	tag, err := r.TagObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	return tag.Commit()
}

func (s *server) lastCommits(r *git.Repository, from *object.Commit, patterns []string) ([]*object.Commit, error) {
	last := make([]*object.Commit, len(patterns))
	/*for i := range last {
		last[i] = from
	}
	return last, nil*/

	remaining := len(patterns)

	commits, err := r.Log(&git.LogOptions{From: from.Hash})
	if err != nil {
		return nil, err
	}

	err = commits.ForEach(func(c *object.Commit) error {
		ctree, err := c.Tree()
		if err != nil {
			return err
		}

		parents := 0
		err = c.Parents().ForEach(func(p *object.Commit) error {
			parents++

			ptree, err := p.Tree()
			if err != nil {
				return err
			}

			changes, err := ptree.Diff(ctree)
			if err != nil {
				return err
			}

			for _, change := range changes {
				for i, pattern := range patterns {
					if last[i] == nil && strings.HasPrefix(change.To.Name, pattern) {
						last[i] = c
						remaining--
						if remaining == 0 {
							return storer.ErrStop
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		if parents == 0 {
			for i, l := range last {
				if l == nil {
					last[i] = c
					remaining--
				}
			}
		}

		if remaining == 0 {
			return storer.ErrStop
		}
		return nil
	})

	return last, err
}

type treeEntry struct {
	*object.TreeEntry
	LastCommit *object.Commit
}

func (s *server) tree(c echo.Context, revName, p string) error {
	r, err := repo(c)
	if err != nil {
		return err
	}

	commit, err := s.commitFromRev(r, revName)
	if err == plumbing.ErrReferenceNotFound {
		return c.String(http.StatusNotFound, "No such revision")
	} else if err != nil {
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

	var data struct {
		*headerData
		Revision        string
		DirName, DirSep string
		Parents         []breadcumbItem
		Entries         []treeEntry
		LastCommit      *object.Commit
		ReadMe          template.HTML
	}

	data.headerData = s.headerData(c)
	data.Revision = revName

	sort.Slice(tree.Entries, func(i, j int) bool {
		a, b := &tree.Entries[i], &tree.Entries[j]
		if a.Mode&filemode.Dir != 0 {
			return true
		}
		if b.Mode&filemode.Dir != 0 {
			return false
		}
		return a.Name < b.Name
	})

	patterns := make([]string, 0, len(tree.Entries)+1)
	pathPattern := p + "/"
	if p == "/" {
		pathPattern = ""
	}
	patterns = append(patterns, pathPattern)
	for _, e := range tree.Entries {
		pattern := e.Name
		if p != "/" {
			pattern = path.Join(p, pattern)
		}
		if e.Mode&filemode.Dir != 0 {
			pattern += "/"
		}
		patterns = append(patterns, pattern)
	}

	lastCommits, err := s.lastCommits(r, commit, patterns)
	if err != nil {
		return err
	}

	data.Entries = make([]treeEntry, len(tree.Entries))
	data.LastCommit = lastCommits[0]
	for i := range tree.Entries {
		data.Entries[i] = treeEntry{&tree.Entries[i], lastCommits[i+1]}
	}

	for _, e := range tree.Entries {
		name := strings.TrimSuffix(e.Name, path.Ext(e.Name))
		if strings.EqualFold(name, "README") && e.Mode&filemode.Regular != 0 {
			f, err := tree.TreeEntryFile(&e)
			if err != nil {
				return err
			}

			raw, err := f.Contents()
			if err != nil {
				return err
			}

			rendered := blackfriday.MarkdownCommon([]byte(raw))
			data.ReadMe = template.HTML(string(rendered))
			break
		}
	}

	dirpath, filepath := path.Split(p)
	data.DirName = filepath
	data.Parents = pathBreadcumb(dirpath)

	data.DirSep = "/" + p + "/"
	if p == "/" {
		data.DirSep = "/"
	}

	return c.Render(http.StatusOK, "tree.html", data)
}

func (s *server) blob(c echo.Context, revName, p string) error {
	r, err := repo(c)
	if err != nil {
		return err
	}

	commit, err := s.commitFromRev(r, revName)
	if err == plumbing.ErrReferenceNotFound {
		return c.String(http.StatusNotFound, "No such revision")
	} else if err != nil {
		return err
	}

	f, err := commit.File(p)
	if err == object.ErrFileNotFound {
		return c.String(http.StatusNotFound, "No such file")
	} else if err != nil {
		return err
	}

	var data struct {
		*headerData
		Revision                      string
		Filepath, Filename, Extension string
		Parents                       []breadcumbItem
		IsBinary                      bool
		Rendered                      template.HTML
		Contents                      string
	}

	data.headerData = s.headerData(c)
	data.Revision = revName

	dirpath, filename := path.Split(p)
	data.Filepath = p
	data.Filename = filename
	data.Extension = strings.TrimLeft(path.Ext(p), ".")
	data.Parents = pathBreadcumb(dirpath)

	if f.Size > 1024*1024 {
		data.IsBinary = true
	} else if binary, err := f.IsBinary(); err != nil || binary {
		data.IsBinary = true
	}

	if !data.IsBinary {
		contents, err := f.Contents()
		if err != nil {
			return err
		}
		data.Contents = contents

		switch data.Extension {
		case "md", "markdown":
			rendered := blackfriday.MarkdownCommon([]byte(contents))
			data.Rendered = template.HTML(string(rendered))
		}
	}

	return c.Render(http.StatusOK, "blob.html", data)
}

func (s *server) raw(c echo.Context, revName, p string) error {
	r, err := repo(c)
	if err != nil {
		return err
	}

	commit, err := s.commitFromRev(r, revName)
	if err == plumbing.ErrReferenceNotFound {
		return c.String(http.StatusNotFound, "No such revision")
	} else if err != nil {
		return err
	}

	f, err := commit.File(p)
	if err == object.ErrFileNotFound {
		return c.String(http.StatusNotFound, "No such file")
	} else if err != nil {
		return err
	}

	rr, err := f.Reader()
	if err != nil {
		return err
	}
	defer rr.Close()

	// TODO: autodetect file type
	mediaType := "application/octet-stream"
	if binary, err := f.IsBinary(); err == nil && !binary {
		mediaType = "text/plain"
	}

	// TODO: set filename
	return c.Stream(http.StatusOK, mediaType, rr)
}

func (s *server) branches(c echo.Context) error {
	r, err := repo(c)
	if err != nil {
		return err
	}

	branches, err := r.Branches()
	if err != nil {
		return err
	}
	defer branches.Close()

	var data struct {
		*headerData
		Branches []string
	}

	data.headerData = s.headerData(c)

	err = branches.ForEach(func(ref *plumbing.Reference) error {
		data.Branches = append(data.Branches, ref.Name().Short())
		return nil
	})
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "branches.html", data)
}

func (s *server) tags(c echo.Context) error {
	r, err := repo(c)
	if err != nil {
		return err
	}

	tags, err := r.TagObjects()
	if err != nil {
		return err
	}
	defer tags.Close()

	var data struct {
		*headerData
		Tags []*object.Tag
	}

	data.headerData = s.headerData(c)

	err = tags.ForEach(func(t *object.Tag) error {
		data.Tags = append(data.Tags, t)
		return nil
	})
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "tags.html", data)
}

func (s *server) commits(c echo.Context, revName string) error {
	r, err := repo(c)
	if err != nil {
		return err
	}

	commit, err := s.commitFromRev(r, revName)
	if err == plumbing.ErrReferenceNotFound {
		return c.String(http.StatusNotFound, "No such revision")
	} else if err != nil {
		return err
	}

	commits, err := r.Log(&git.LogOptions{From: commit.Hash})
	if err != nil {
		return err
	}
	defer commits.Close()

	var data struct {
		*headerData
		Commits []*object.Commit
	}

	data.headerData = s.headerData(c)

	err = commits.ForEach(func(c *object.Commit) error {
		data.Commits = append(data.Commits, c)
		return nil
	})
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "commits.html", data)
}

func (s *server) commit(c echo.Context, hash string) error {
	r, err := repo(c)
	if err != nil {
		return err
	}

	commit, err := r.CommitObject(plumbing.NewHash(hash))
	if err == plumbing.ErrObjectNotFound {
		return c.String(http.StatusNotFound, "No such commit")
	} else if err != nil {
		return err
	}

	var data struct {
		*headerData
		Commit *object.Commit
		Diff   string
	}

	data.headerData = s.headerData(c)
	data.Commit = commit

	if len(commit.ParentHashes) > 0 {
		// TODO
		parent, err := r.CommitObject(commit.ParentHashes[0])
		if err != nil {
			return err
		}

		patch, err := parent.Patch(commit)
		if err != nil {
			return err
		}

		data.Diff = patch.String()
	}

	return c.Render(http.StatusOK, "commit.html", data)
}

func New(e *echo.Echo, dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	e.Renderer, err = loadTemplateRenderer()
	if err != nil {
		return err
	}

	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			parent, dirname := filepath.Split(dir)
			dirParts := append([]string{parent}, dirname)
			reqParts := strings.Split(req.URL.Path, "/")

			for i := 0; i <= len(reqParts); i++ {
				parts := append(dirParts, reqParts[:i]...)
				p := filepath.Join(parts...)

				r, err := git.PlainOpen(p)
				if err == git.ErrRepositoryNotExists {
					continue
				} else if err != nil {
					return err
				}

				req.URL.Path = "/" + path.Join(reqParts[i:]...)
				c.Set("repo", r)
				c.Set("repo-name", parts[len(dirParts)+i-1])
				c.Set("repo-path", "/"+path.Join(reqParts[:i]...))
				return next(c)
			}

			return next(c)
		}
	})

	s := &server{}

	e.GET("/", func(c echo.Context) error {
		return s.tree(c, "master", "/")
	})
	e.GET("/tree/:ref", func(c echo.Context) error {
		return s.tree(c, c.Param("ref"), "")
	})
	e.GET("/tree/:ref/*", func(c echo.Context) error {
		return s.tree(c, c.Param("ref"), c.Param("*"))
	})
	e.GET("/blob/:ref/*", func(c echo.Context) error {
		return s.blob(c, c.Param("ref"), c.Param("*"))
	})
	e.GET("/raw/:ref/*", func(c echo.Context) error {
		return s.raw(c, c.Param("ref"), c.Param("*"))
	})
	e.GET("/branches", s.branches)
	e.GET("/tags", s.tags)
	e.GET("/commits/:ref", func(c echo.Context) error {
		return s.commits(c, c.Param("ref"))
	})
	e.GET("/commit/:hash", func(c echo.Context) error {
		return s.commit(c, c.Param("hash"))
	})

	e.Static("/static", "public/node_modules")

	return nil
}
