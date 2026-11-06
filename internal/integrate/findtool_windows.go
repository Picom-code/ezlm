package integrate

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// userPath is the Machine and then the User PATH as the registry holds them
// now, expanded. Installers (Hermes, npm, scoop) change these and broadcast
// the change, but a running EZLM keeps the PATH it started with.
func userPath() []string {
	var dirs []string
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{registry.CURRENT_USER, `Environment`},
	} {
		dirs = append(dirs, splitWinPath(readRegPath(k.root, k.path))...)
	}
	return dirs
}

func readRegPath(root registry.Key, path string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, typ, err := k.GetStringValue("Path")
	if err != nil {
		return ""
	}
	if typ == registry.EXPAND_SZ {
		if x, err := registry.ExpandString(v); err == nil {
			v = x
		}
	}
	return v
}

// splitWinPath splits a registry PATH value, dropping quotes and empty
// entries and expanding %VARS% the registry left unexpanded.
func splitWinPath(v string) []string {
	var out []string
	for _, d := range strings.Split(v, ";") {
		d = strings.Trim(strings.TrimSpace(d), `"`)
		if strings.Contains(d, "%") {
			d = os.ExpandEnv(winVars(d))
		}
		if d != "" {
			out = append(out, filepath.Clean(d))
		}
	}
	return out
}

// winVars turns %NAME% into ${NAME} for os.ExpandEnv.
func winVars(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			break
		}
		b.WriteString(s[:i] + "${" + s[i+1:i+1+j] + "}")
		s = s[i+2+j:]
	}
	b.WriteString(s)
	return b.String()
}

// LoginEnv is a variable from the user's registry environment ("" if
// unset), for settings an installer saved there after EZLM started.
func LoginEnv(key string) string {
	if _, ok := os.LookupEnv("EZLM_TOOL_PATH"); ok {
		return ""
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, typ, err := k.GetStringValue(key)
	if err != nil {
		return ""
	}
	if typ == registry.EXPAND_SZ {
		if x, err := registry.ExpandString(v); err == nil {
			v = x
		}
	}
	return v
}

// RefreshToolSearch: the registry is read on every search, so there is
// nothing to drop.
func RefreshToolSearch() {}
