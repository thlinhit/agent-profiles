package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/thlinhit/agent-profiles/internal/fsutil"
)

func absolutePool(path string) (string, error) {
	if path == "~" {
		path = "~/"
	}
	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}

// UpdatePool adds or removes a pool while preserving unrelated TOML values.
// Comments and formatting may be normalized when a change is written.
func UpdatePool(input string, add bool) error {
	absolute, err := absolutePool(input)
	if err != nil {
		return err
	}
	if add {
		st, err := os.Stat(absolute)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return fmt.Errorf("pool is not a directory: %s", absolute)
		}
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	values := make(map[string]any)
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular config file: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := toml.Unmarshal(data, &values); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	pools := []string{}
	if value, exists := values["skill_pools"]; exists {
		entries, ok := value.([]any)
		if !ok {
			return fmt.Errorf("skill_pools must be an array of strings")
		}
		for _, entry := range entries {
			pool, ok := entry.(string)
			if !ok {
				return fmt.Errorf("skill_pools must be an array of strings")
			}
			pools = append(pools, pool)
		}
	}
	found := false
	next := make([]string, 0, len(pools)+1)
	for _, pool := range pools {
		normalized, err := absolutePool(pool)
		if err != nil {
			return err
		}
		match := pool == input || normalized == absolute
		if match {
			found = true
		}
		if add || !match {
			next = append(next, pool)
		}
	}
	if add {
		if found {
			return nil
		}
		next = append(next, absolute)
	} else if !found {
		return fmt.Errorf("pool %q not found", input)
	}
	values["skill_pools"] = next
	data, err := toml.Marshal(values)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return fsutil.AtomicWrite(path, data)
}
