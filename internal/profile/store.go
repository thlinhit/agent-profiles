package profile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	defer func() { _ = root.Close() }()
	if err = root.Mkdir(name, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("profile %q already exists", agent+"/"+name)
		}
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
	if err == nil {
		err = CheckDir(filepath.Join(dir, name))
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("profile %q does not exist; run ap list", agent+"/"+name)
		}
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// maxSettingsSize caps how much of a source settings.json is copied.
const maxSettingsSize = 1 << 20

// ReadSettings returns the settings.json of an existing profile, or nil when
// the profile has none. Symlinks and non-regular files are refused so a copy
// never reads outside the profile directory.
func (s Store) ReadSettings(agent, name string) ([]byte, error) {
	dir, err := s.Resolve(agent, name)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	st, err := root.Lstat("settings.json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("settings.json of %q is not a regular file", agent+"/"+name)
	}
	if st.Size() > maxSettingsSize {
		return nil, fmt.Errorf("settings.json of %q is larger than %d bytes", agent+"/"+name, maxSettingsSize)
	}
	f, err := root.Open("settings.json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxSettingsSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSettingsSize {
		return nil, fmt.Errorf("settings.json of %q is larger than %d bytes", agent+"/"+name, maxSettingsSize)
	}
	return data, nil
}

// WriteSettings atomically replaces a profile's settings.json with mode 0600.
// It writes a temporary file in the profile directory and renames it over
// the target, so a failure never leaves a partial settings file.
func (s Store) WriteSettings(agent, name string, data []byte) error {
	dir, err := s.Resolve(agent, name)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if st, err := root.Lstat("settings.json"); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("settings.json of %q is not a regular file", agent+"/"+name)
	}
	const tmp = ".settings.json.tmp"
	_ = root.Remove(tmp)
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if ce := f.Close(); err == nil {
		err = ce
	}
	if err == nil {
		err = root.Rename(tmp, "settings.json")
	}
	if err != nil {
		_ = root.Remove(tmp)
	}
	return err
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
	defer func() { _ = root.Close() }()
	return root.RemoveAll(name)
}
