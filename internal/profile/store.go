package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var validName = regexp.MustCompile("^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$")

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid name %q: use 1–64 ASCII letters, digits, underscores or hyphens, starting with a letter or digit", name)
	}
	return nil
}

type Store struct{}

func DataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("XDG_DATA_HOME must be absolute")
	}
	return filepath.Join(base, "agent-profiles"), nil
}

// PrivateDir creates only application-owned directories; it never chmods ancestors.
func PrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return CheckDir(path)
}
func CheckDir(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("not a real directory: %s", path)
	}
	if st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("directory must be private (0700): %s", path)
	}
	return nil
}
func (Store) agentDir(agent string, create bool) (string, error) {
	if err := ValidateName(agent); err != nil {
		return "", err
	}
	root, err := DataDir()
	if err != nil {
		return "", err
	}
	check := CheckDir
	if create {
		check = PrivateDir
	}
	if err = check(root); err != nil {
		return "", err
	}
	dir := filepath.Join(root, agent)
	if err = check(dir); err != nil {
		return "", err
	}
	return dir, nil
}
func (s Store) Create(agent, name string, template []byte) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir, err := s.agentDir(agent, true)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err = root.Mkdir(name, 0o700); err != nil {
		return "", err
	}
	if len(template) > 0 {
		f, e := root.OpenFile(filepath.Join(name, "settings.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if e != nil {
			return "", e
		}
		_, e = f.Write(template)
		ce := f.Close()
		if e != nil {
			return "", e
		}
		if ce != nil {
			return "", ce
		}
	}
	return filepath.Join(dir, name), nil
}
func (s Store) Resolve(agent, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir, err := s.agentDir(agent, false)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if err = CheckDir(p); err != nil {
		return "", err
	}
	return p, nil
}
func (s Store) List(agent string) ([]string, error) {
	dir, err := s.agentDir(agent, false)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && ValidateName(e.Name()) == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
func (s Store) Delete(agent, name string) error {
	if _, err := s.Resolve(agent, name); err != nil {
		return err
	}
	dir, err := s.agentDir(agent, false)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(name)
}
