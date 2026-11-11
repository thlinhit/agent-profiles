package shell

import (
	"fmt"
	"regexp"
	"strings"
)

var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// Quote is the only shell value encoder. Shell variables cannot contain NUL.
func Quote(value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("shell value contains NUL")
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'", nil
}
func Export(key, value string) (string, error) {
	if !envName.MatchString(key) {
		return "", fmt.Errorf("invalid environment variable %q", key)
	}
	quoted, err := Quote(value)
	if err != nil {
		return "", err
	}
	return "export " + key + "=" + quoted + ";\n", nil
}
func Unset(key string) (string, error) {
	if !envName.MatchString(key) {
		return "", fmt.Errorf("invalid environment variable %q", key)
	}
	return "unset " + key + ";\n", nil
}
func Init(name string) (string, error) {
	if name != "bash" && name != "zsh" {
		return "", fmt.Errorf("unsupported shell %q", name)
	}
	return `ap() {
  local _ap_code
  _ap_code=$(command agent-profiles "$@" --emit-shell) || return $?
  eval "$_ap_code"
}
`, nil
}
