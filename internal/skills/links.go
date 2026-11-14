package skills

import (
	"iter"
	"os"
	"path/filepath"
)

// LinkKind classifies a Profile's Skill links. NonLink is never managed by ap.
type LinkKind uint8

const (
	NonLink LinkKind = iota
	Pool
	Alias
	External
	Broken
)

// Link retains the raw target for display and change detection, and the
// resolved source for matching against Skill pools.
type Link struct {
	Name, Target, Source, Pool string
	Kind                       LinkKind
}

// Links reads a Profile's skills directory in name order. pools maps canonical
// Skill sources to pool paths. With nil pools it only inspects raw targets,
// retaining the listing's stat errors rather than resolving canonical sources.
// Results are yielded as read so a later error preserves earlier printed lines.
func Links(dir string, pools map[string]string) iter.Seq2[Link, error] {
	return func(yield func(Link, error) bool) {
		// Listing checks the directory first; the Picker historically reads it first.
		if pools == nil {
			if err := checkOptionalDir(dir); err != nil {
				yield(Link{}, err)
				return
			}
		}
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			yield(Link{}, err)
			return
		}
		if pools != nil {
			if err := checkOptionalDir(dir); err != nil {
				yield(Link{}, err)
				return
			}
		}
		for _, entry := range entries {
			link := Link{Name: entry.Name()}
			if entry.Type()&os.ModeSymlink == 0 {
				if !yield(link, nil) {
					return
				}
				continue
			}
			path := filepath.Join(dir, entry.Name())
			link.Target, err = os.Readlink(path)
			if err != nil {
				yield(Link{}, err)
				return
			}
			if pools == nil {
				_, err = os.Stat(path)
			} else {
				link.Source, err = filepath.EvalSymlinks(path)
			}
			switch {
			case os.IsNotExist(err):
				link.Kind = Broken
				link.Source = ""
			case err != nil:
				yield(Link{}, err)
				return
			default:
				if pool, ok := pools[link.Source]; ok {
					link.Pool = pool
					link.Kind = Pool
					if link.Name != filepath.Base(link.Source) {
						link.Kind = Alias
					}
				} else {
					link.Kind = External
				}
			}
			if !yield(link, nil) {
				return
			}
		}
	}
}
