package fsutil

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var ErrTooLarge = errors.New("input exceeds size limit")

// ReadLimited reads at most max bytes, rejecting input that exceeds the limit.
func ReadLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, ErrTooLarge
	}
	return data, nil
}

// AtomicWrite refuses existing symlinks and renames a private sibling over the entry.
func AtomicWrite(path string, data []byte) error {
	check := func() error {
		st, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %s: writing could clobber another file", path)
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", path)
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".agent-profiles-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = check(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// AtomicWriteRoot uses an open Root for every filesystem operation.
// Root has no CreateTemp method, so exclusive random siblings serve the same purpose.
func AtomicWriteRoot(root *os.Root, name string, data []byte) error {
	check := func() error {
		st, err := root.Lstat(name)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %s: writing could clobber another file", name)
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", name)
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	var f *os.File
	var tmp string
	for {
		tmp = filepath.Join(filepath.Dir(name), ".agent-profiles-"+rand.Text())
		var err error
		f, err = root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		break
	}
	defer func() { _ = root.Remove(tmp) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	return root.Rename(tmp, name)
}
