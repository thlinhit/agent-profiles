package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

type Store struct{}

const maxDatabase = 1 << 20

func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func read(root *os.Root) (map[string]string, error) {
	entries := map[string]string{}
	f, err := root.OpenFile("trust.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("trust.json must be a regular private file (0600)")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxDatabase+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDatabase {
		return nil, fmt.Errorf("trust database exceeds 1 MiB")
	}
	if err = json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = map[string]string{}
	}
	return entries, nil
}
func (Store) Trusted(doc *dotfile.Document) (bool, error) {
	hash := digest(doc.Content)
	dir, err := profile.DataDir()
	if err != nil {
		return false, err
	}
	if err = profile.CheckDir(dir); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	entries, err := read(root)
	if err != nil {
		return false, err
	}
	return entries[doc.Dir] == hash, nil
}
func update(change func(map[string]string)) error {
	dir, err := profile.DataDir()
	if err != nil {
		return err
	}
	if err = profile.PrivateDir(dir); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	lock, err := root.OpenFile("trust.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	st, err := lock.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 {
		return fmt.Errorf("trust lock must be a private regular file")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	entries, err := read(root)
	if err != nil {
		return err
	}
	change(entries)
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxDatabase {
		return fmt.Errorf("trust database exceeds 1 MiB")
	}
	return dotfile.AtomicWrite(filepath.Join(dir, "trust.json"), append(data, '\n'))
}
func (Store) Grant(doc *dotfile.Document) error {
	return update(func(entries map[string]string) { entries[doc.Dir] = digest(doc.Content) })
}
func (Store) Revoke(dir string) error {
	canonical, err := dotfile.Canonical(dir)
	if err != nil {
		return err
	}
	return update(func(entries map[string]string) { delete(entries, canonical) })
}
