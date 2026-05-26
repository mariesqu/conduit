package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileService is the upload/download/list backend rooted at a single
// directory. All path inputs from clients are joined to Root and then
// re-validated to ensure the result is still under Root.
type FileService struct {
	Root           string // absolute; created on first use
	MaxUploadBytes int64
}

// NewFileService constructs the service. If root is empty, defaults to
// $USERPROFILE/Conduit-Files (Windows) or $HOME/Conduit-Files. The dir
// is created if missing.
func NewFileService(root string, maxUploadMB int) (*FileService, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("locate home dir: %w", err)
		}
		root = filepath.Join(home, "Conduit-Files")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve files root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create files root: %w", err)
	}
	if maxUploadMB <= 0 {
		maxUploadMB = DefaultMaxUploadMB
	}
	return &FileService{
		Root:           abs,
		MaxUploadBytes: int64(maxUploadMB) * 1024 * 1024,
	}, nil
}

// SafePath joins rel to Root and verifies the result is still under
// Root after cleaning (no traversal via .. segments or absolute paths).
// The returned path may or may not exist.
func (f *FileService) SafePath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	// Normalize backslashes so Windows-style inputs work on any OS.
	rel = strings.ReplaceAll(rel, "\\", "/")
	// Reject absolute inputs explicitly — Join would silently drop the
	// existing base, which is a path-traversal surface.
	if strings.HasPrefix(rel, "/") || (len(rel) >= 2 && rel[1] == ':') {
		return "", errors.New("absolute paths not allowed")
	}
	joined := filepath.Join(f.Root, filepath.FromSlash(rel))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(f.Root)
	if err != nil {
		return "", err
	}
	relCheck, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", err
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", errors.New("path outside files root")
	}
	return abs, nil
}

// RegisterFileRoutes adds the file REST endpoints to mux. All require
// the main auth token.
//
//   POST /api/files                  (multipart) → [{name, path, size}]
//   GET  /api/files/download?path=R   → file bytes
//   GET  /api/files/list?dir=R        → [{name, size, mod, dir}]
func RegisterFileRoutes(mux *http.ServeMux, cfg *Config, fs *FileService) {
	mux.HandleFunc("POST /api/files", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Cap whole request to avoid DoS; per-file cap enforced below.
		r.Body = http.MaxBytesReader(w, r.Body, fs.MaxUploadBytes+1024*1024)
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, "expected multipart/form-data", http.StatusBadRequest)
			return
		}
		dir := strings.TrimSpace(r.URL.Query().Get("dir"))
		destDir := fs.Root
		if dir != "" {
			d, err := fs.SafePath(dir)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			destDir = d
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				http.Error(w, "create dir: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		var saved []map[string]any
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "multipart read: "+err.Error(), http.StatusBadRequest)
				return
			}
			if part.FileName() == "" {
				_ = part.Close()
				continue
			}
			name := safeBaseName(part.FileName())
			if name == "" {
				_ = part.Close()
				continue
			}
			fullPath := filepath.Join(destDir, name)
			// Re-check after join: name should have no separators after sanitize,
			// but be defensive.
			if _, err := fs.SafePath(rel(fs.Root, fullPath)); err != nil {
				_ = part.Close()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			out, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				_ = part.Close()
				http.Error(w, "open file: "+err.Error(), http.StatusInternalServerError)
				return
			}
			n, copyErr := io.CopyN(out, part, fs.MaxUploadBytes+1)
			_ = part.Close()
			if copyErr != nil && copyErr != io.EOF {
				_ = out.Close()
				_ = os.Remove(fullPath)
				http.Error(w, "write file: "+copyErr.Error(), http.StatusInternalServerError)
				return
			}
			if n > fs.MaxUploadBytes {
				_ = out.Close()
				_ = os.Remove(fullPath)
				http.Error(w, fmt.Sprintf("file %q exceeds max upload size", name), http.StatusRequestEntityTooLarge)
				return
			}
			_ = out.Close()
			saved = append(saved, map[string]any{
				"name": name,
				"path": rel(fs.Root, fullPath),
				"size": n,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(saved)
	})

	mux.HandleFunc("GET /api/files/download", func(w http.ResponseWriter, r *http.Request) {
		// Download accepts ?token= so plain <a download> links work.
		if !authorizeWithQuery(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p := r.URL.Query().Get("path")
		if p == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		abs, err := fs.SafePath(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(abs)))
		http.ServeFile(w, r, abs)
	})

	mux.HandleFunc("GET /api/files/list", func(w http.ResponseWriter, r *http.Request) {
		if !authorize(cfg, r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		dir := r.URL.Query().Get("dir")
		base := fs.Root
		if dir != "" {
			d, err := fs.SafePath(dir)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			base = d
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		type item struct {
			Name string    `json:"name"`
			Path string    `json:"path"`
			Size int64     `json:"size"`
			Dir  bool      `json:"dir"`
			Mod  time.Time `json:"modified"`
		}
		var out []item
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, item{
				Name: e.Name(),
				Path: rel(fs.Root, filepath.Join(base, e.Name())),
				Size: info.Size(),
				Dir:  e.IsDir(),
				Mod:  info.ModTime(),
			})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Dir != out[j].Dir {
				return out[i].Dir
			}
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"root":    rel(fs.Root, base),
			"entries": out,
		})
	})
}

// safeBaseName strips any directory components from a filename and
// disallows hidden/system names. Returns "" if the result would be
// unsafe (empty, dot, traversal).
func safeBaseName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if strings.ContainsAny(name, `/\:`) {
		return ""
	}
	return name
}

// rel returns the cleaned forward-slash path of full relative to root,
// or full itself if it can't be made relative.
func rel(root, full string) string {
	r, err := filepath.Rel(root, full)
	if err != nil {
		return filepath.ToSlash(full)
	}
	return filepath.ToSlash(r)
}

// logFile is a tiny helper used by tests to confirm setup.
func (f *FileService) logRoot() {
	log.Printf("file service: root=%s max=%dB", f.Root, f.MaxUploadBytes)
}
