package dotfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/pelletier/go-toml/v2"

	"github.com/thlinhit/agent-profiles/internal/fsutil"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

const Filename = ".agent-profile"
const MaxSize = 4096

var errTooLarge = errors.New("dotfile exceeds 4 KiB")

// Document contains checked raw bytes, never parsed profile selections.
type Document struct {
	Dir     string
	Content []byte
}

func Parse(data []byte) (map[string]string, error) {
	if len(data) > MaxSize {
		return nil, errTooLarge
	}
	// Decode native TOML values first: typed-map decoding in go-toml can panic
	// when a date or time is assigned to a string. Enforce our schema explicitly.
	var values map[string]any
	if err := toml.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	p := make(map[string]string, len(values))
	for a, value := range values {
		n, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("profile for %q must be a string", a)
		}
		if err := profile.ValidateName(a); err != nil {
			return nil, err
		}
		if err := profile.ValidateName(n); err != nil {
			return nil, err
		}
		p[a] = n
	}
	return p, nil
}
func Canonical(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
func boundary(start string) string {
	home, _ := os.UserHomeDir()
	home, _ = Canonical(home)
	for dir := start; ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if dir == home || filepath.Dir(dir) == dir {
			return dir
		}
	}
}

// Read applies filesystem and size checks without invoking the TOML parser.
func Read(path, rootDir string) (*Document, error) {
	dir, err := Canonical(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	return readCanonical(path, rootDir, dir)
}

// readCanonical reuses the canonical directory established by the caller.
func readCanonical(path, rootDir, dir string) (*Document, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootDir, resolved)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	st, err := root.Stat(rel)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("dotfile is not a regular file")
	}
	if st.Size() > MaxSize {
		return nil, errTooLarge
	}
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("dotfile must stay inside repository root: %w", err)
	}
	defer func() { _ = f.Close() }()
	st, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("dotfile is not a regular file")
	}
	data, err := fsutil.ReadLimited(f, MaxSize)
	if errors.Is(err, fsutil.ErrTooLarge) {
		return nil, errTooLarge
	}
	if err != nil {
		return nil, err
	}
	return &Document{Dir: dir, Content: data}, nil
}
func Find(start string) (*Document, error) {
	dir, err := Canonical(start)
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	home, _ = Canonical(home)
	var path, docDir string
	for {
		if path == "" {
			candidate := filepath.Join(dir, Filename)
			if _, err := os.Lstat(candidate); err == nil {
				path, docDir = candidate, dir
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		_, gitErr := os.Lstat(filepath.Join(dir, ".git"))
		if gitErr == nil || dir == home || filepath.Dir(dir) == dir {
			if path == "" {
				return nil, nil
			}
			return readCanonical(path, dir, docDir)
		}
		dir = filepath.Dir(dir)
	}
}

func Write(dir, a, n string) error {
	if err := profile.ValidateName(a); err != nil {
		return err
	}
	if err := profile.ValidateName(n); err != nil {
		return err
	}
	dir, err := Canonical(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, Filename)
	p := map[string]string{}
	st, err := os.Lstat(path)
	if err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %s: writing could clobber another file", path)
		}
		doc, e := readCanonical(path, boundary(dir), dir)
		if e != nil {
			return e
		}
		// Explicit local editing parses existing entries for merging, not selection.
		p, e = Parse(doc.Content)
		if e != nil {
			return e
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	p[a] = n
	data, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > MaxSize {
		return errTooLarge
	}
	return fsutil.AtomicWrite(path, data)
}
