package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Root is one named directory served in multi-root mode at /<name>/.
type Root struct {
	Name string
	Dir  string
}

// RootsConfig is a parsed multi-root config file.
type RootsConfig struct {
	Roots []Root
	// Port is the config's port line, or 0 when it has none.
	Port int
}

var rootNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// reservedRootNames are first path segments mdserver routes itself, so a root
// cannot use them ("port" is the config's port keyword).
var reservedRootNames = map[string]bool{
	"port":        true,
	"settings":    true,
	"livereload":  true,
	"favicon.ico": true,
	"favicon.svg": true,
	"_mdserver":   true,
}

// ParseRootsConfig parses the contents of a roots config file read from file
// (used in error messages). home is the user's home directory, which "~/"
// paths expand to. Any invalid line, or a file with no roots, is an error of
// the form "<file>:<line>: <reason>".
//
// Grammar: blank lines and lines starting with # are ignored; "port <n>" sets
// the port; any other line is "<name> <path>", where the path is the rest of
// the line.
func ParseRootsConfig(data []byte, file, home string) (RootsConfig, error) {
	var cfg RootsConfig
	seen := map[string]bool{}
	havePort := false

	for i, line := range strings.Split(string(data), "\n") {
		lineNo := i + 1
		fail := func(format string, args ...any) (RootsConfig, error) {
			return RootsConfig{}, fmt.Errorf("%s:%d: %s", file, lineNo, fmt.Sprintf(format, args...))
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, rest := line, ""
		if idx := strings.IndexAny(line, " \t"); idx >= 0 {
			name, rest = line[:idx], strings.TrimSpace(line[idx:])
		}

		if name == "port" {
			if havePort {
				return fail("second port line")
			}
			n, err := strconv.Atoi(rest)
			if err != nil || n < 1 || n > 65535 {
				return fail("bad port %q (want 1-65535)", rest)
			}
			havePort = true
			cfg.Port = n
			continue
		}

		if !rootNamePattern.MatchString(name) {
			return fail("invalid name %q (want lowercase letters, digits, - and _, starting with a letter or digit)", name)
		}
		if reservedRootNames[name] {
			return fail("reserved name %q", name)
		}
		if seen[name] {
			return fail("duplicate name %q", name)
		}
		if rest == "" {
			return fail("name %q has no path", name)
		}

		dir, err := expandRootPath(rest, home)
		if err != nil {
			return fail("%v", err)
		}
		if err := checkRootDir(dir, home); err != nil {
			return fail("%v", err)
		}

		seen[name] = true
		cfg.Roots = append(cfg.Roots, Root{Name: name, Dir: dir})
	}

	if len(cfg.Roots) == 0 {
		return RootsConfig{}, fmt.Errorf("%s:0: no roots defined", file)
	}
	return cfg, nil
}

// expandRootPath accepts an absolute path or one starting with "~/" and
// expands the latter. Nothing else is expanded.
func expandRootPath(path, home string) (string, error) {
	switch {
	case strings.HasPrefix(path, "~/"):
		if home == "" {
			return "", fmt.Errorf("cannot expand ~/: home directory unknown")
		}
		return filepath.Join(home, path[2:]), nil
	case strings.HasPrefix(path, "/"):
		return filepath.Clean(path), nil
	default:
		return "", fmt.Errorf("path must start with / or ~/")
	}
}

// checkRootDir requires dir to exist and be a directory.
func checkRootDir(dir, home string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dir)
	}
	return nil
}
