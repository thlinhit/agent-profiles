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
	shim := `ap() {
  local _ap_code
  if [ "$1" = exec ]; then
    command agent-profiles "$@"
    return $?
  fi
  _ap_code=$(command agent-profiles "$@" --emit-shell) || return $?
`
	if name == "bash" {
		shim += `  case "$1" in
    local|trust|untrust) unset _ap_last_pwd ;;
  esac
`
	}
	shim += `  eval "$_ap_code"
}
_ap_auto() {
  local _ap_status=$? _ap_code
`
	if name == "bash" {
		shim += `  if [[ ${_ap_last_pwd-} == "$PWD" ]]; then
    return "$_ap_status"
  fi
  _ap_last_pwd=$PWD
`
	}
	shim += `  if _ap_code=$(command agent-profiles auto --emit-shell); then
    eval "$_ap_code"
  fi
  return "$_ap_status"
}
`
	if name == "zsh" {
		shim += `typeset -ga chpwd_functions
if (( ! ${chpwd_functions[(Ie)_ap_auto]} )); then
  chpwd_functions+=(_ap_auto)
fi
`
	} else {
		shim += `if [[ $(declare -p PROMPT_COMMAND 2>/dev/null) == 'declare -a'* ]]; then
  _ap_found=0
  for _ap_item in "${PROMPT_COMMAND[@]}"; do
    [[ $_ap_item == _ap_auto ]] && _ap_found=1
  done
  if [[ $_ap_found == 0 ]]; then PROMPT_COMMAND=(_ap_auto "${PROMPT_COMMAND[@]}"); fi
  unset _ap_found _ap_item
else
  case "${PROMPT_COMMAND-}" in
    _ap_auto|_ap_auto\;*) ;;
    *) PROMPT_COMMAND="_ap_auto${PROMPT_COMMAND:+; $PROMPT_COMMAND}" ;;
  esac
fi
unset _ap_last_pwd
`
	}
	return shim + "_ap_auto\n", nil
}
