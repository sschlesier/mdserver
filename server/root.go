package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// root is one served directory and the live reload that watches it. Server
// finds files, lists directories and runs live reload through a root, so each
// root's reloads stay with its own clients.
type root struct {
	// name is the first URL segment in multi-root mode; empty in single-root mode.
	name string
	dir  string
	// prefix is "/"+name in multi-root mode and empty in single-root mode. It
	// goes in front of every URL the root hands out.
	prefix     string
	liveReload *LiveReload
	// gone is set once the root's directory was seen missing in multi-root
	// mode. The root then stays unavailable until restart.
	gone atomic.Bool
}

// resolveKind says what a URL path resolved to under a root.
type resolveKind int

const (
	kindNotFound resolveKind = iota
	kindPage
	kindDirectory
	kindImage
)

// resolve maps a URL path (already decoded, as in r.URL.Path) to a file under
// the root. It returns the filesystem path, what it is and the HTTP status the
// request should get:
//
//	200 serve it as the kind says
//	301 a directory requested without a trailing slash; redirect to urlPath+"/"
//	403 the path lies outside the root
//	404 hidden, missing, or not a page, directory or allowlisted image
//
// The path is empty unless the status is 200 or 301.
func (rt *root) resolve(urlPath string) (string, resolveKind, int) {
	if urlPath == "/" {
		return rt.dir, kindDirectory, http.StatusOK
	}

	requestPath := urlPath
	if len(requestPath) > 0 && requestPath[0] == '/' {
		requestPath = requestPath[1:]
	}

	filePath := filepath.Join(rt.dir, requestPath)

	if rt.isHiddenPath(filePath) {
		return "", kindNotFound, http.StatusNotFound
	}

	if info, err := os.Stat(filePath); err == nil && info.IsDir() {
		if !rt.contains(filePath) {
			return "", kindNotFound, http.StatusForbidden
		}
		if !strings.HasSuffix(requestPath, "/") && !strings.HasSuffix(urlPath, "/") {
			return filePath, kindDirectory, http.StatusMovedPermanently
		}
		return filePath, kindDirectory, http.StatusOK
	}

	// Markdown: a .md file, or a name without an extension that has a .md file.
	if isPageFile(filePath) {
		if rt.isValidPath(filePath) {
			return filePath, kindPage, http.StatusOK
		}
	} else if filepath.Ext(filePath) == "" {
		if withExt := filePath + ".md"; rt.isValidPath(withExt) {
			return withExt, kindPage, http.StatusOK
		}
	}

	// Anything else is served only if it is an allowlisted image.
	if !rt.isValidPath(filePath) {
		return "", kindNotFound, http.StatusForbidden
	}
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return "", kindNotFound, http.StatusNotFound
	}
	if _, ok := imageContentType(filepath.Ext(filePath)); !ok {
		return "", kindNotFound, http.StatusNotFound
	}
	return filePath, kindImage, http.StatusOK
}

// unavailable reports whether the root's directory has been seen missing.
func (rt *root) unavailable() bool {
	return rt.gone.Load()
}

// exists reports whether the root directory is still present. Errors other
// than "not exist" (e.g. permissions) are treated as present.
func (rt *root) exists() bool {
	info, err := os.Stat(rt.dir)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return info.IsDir()
}

// contains reports whether path is the root directory or lies under it.
func (rt *root) contains(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(rt.dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// isValidPath checks if a file path is within the root directory (security)
func (rt *root) isValidPath(filePath string) bool {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	absRoot, err := filepath.Abs(rt.dir)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}

	// Prevent directory traversal
	return rel != ".." && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !hasDotSegment(rel)
}

// isHiddenPath reports whether filePath is inside the root and has a dot segment
// relative to the root. Paths outside the root are not hidden; isValidPath rejects them.
func (rt *root) isHiddenPath(filePath string) bool {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(rt.dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return hasDotSegment(rel)
}

// relPath returns a path relative to the root directory, or the original path if it's outside the root
func (rt *root) relPath(path string) string {
	rel, err := filepath.Rel(rt.dir, path)
	if err != nil {
		return path
	}
	// If path is outside root directory, return original
	if strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
