// Package studio serves AppRivet's local browser interface. It does not expose
// an execution service or accept arbitrary filesystem paths from the browser.
package studio

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fpokrzywa/AppRivet/internal/project"
)

//go:embed ui/index.html ui/studio.css ui/studio.js
var assets embed.FS

type Config struct {
	Host       string
	Session    string
	Workspace  string
	OpenFolder func(string) error
	Quit       func()
}

type state struct {
	Workspace string   `json:"workspace"`
	Projects  []string `json:"projects"`
}

type createRequest struct {
	Name     string `json:"name"`
	BundleID string `json:"bundle_id"`
}

func Handler(config Config) http.Handler {
	mux := http.NewServeMux()
	asset := func(pattern, name, contentType string) {
		body, err := assets.ReadFile("ui/" + name)
		if err != nil {
			panic(err) // The compiler embeds the exact files named above.
		}
		mux.HandleFunc("GET "+pattern, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			_, _ = w.Write(body)
		})
	}
	asset("/{$}", "index.html", "text/html; charset=utf-8")
	asset("/studio.css", "studio.css", "text/css; charset=utf-8")
	asset("/studio.js", "studio.js", "text/javascript; charset=utf-8")
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, _ *http.Request) {
		projects := []string{}
		if err := checkWorkspace(config.Workspace); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, "Could not read your AppRivet Projects folder.")
			return
		}
		if entries, err := os.ReadDir(config.Workspace); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				marker := filepath.Join(config.Workspace, entry.Name(), entry.Name()+".xcodeproj")
				if info, err := os.Lstat(marker); err == nil && info.IsDir() {
					projects = append(projects, entry.Name())
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusInternalServerError, "Could not read your AppRivet Projects folder.")
			return
		}
		writeJSON(w, http.StatusOK, state{Workspace: config.Workspace, Projects: projects})
	})
	mux.HandleFunc("POST /api/create", func(w http.ResponseWriter, r *http.Request) {
		var req createRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF {
			writeError(w, http.StatusBadRequest, "Enter an app name and optional bundle ID.")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.BundleID = strings.TrimSpace(req.BundleID)
		if req.BundleID == "" {
			req.BundleID = "com.example." + strings.ToLower(req.Name)
		}
		if err := ensureWorkspace(config.Workspace); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create your AppRivet Projects folder.")
			return
		}
		info, err := project.Create(config.Workspace, req.Name, req.BundleID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, info)
	})
	mux.HandleFunc("POST /api/open-folder", func(w http.ResponseWriter, _ *http.Request) {
		if ensureWorkspace(config.Workspace) != nil || config.OpenFolder == nil || config.OpenFolder(config.Workspace) != nil {
			writeError(w, http.StatusInternalServerError, "Could not open your AppRivet Projects folder.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/quit", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		if config.Quit != nil {
			config.Quit()
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		if r.Host != config.Host {
			http.Error(w, "Use the local AppRivet window.", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			origin := r.Header.Get("Origin")
			if (origin != "" && origin != "http://"+config.Host) || config.Session == "" ||
				subtle.ConstantTimeCompare([]byte(r.Header.Get("X-AppRivet-Session")), []byte(config.Session)) != 1 {
				writeError(w, http.StatusForbidden, "Open AppRivet from your desktop to start a local session.")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func ensureWorkspace(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return checkWorkspace(path)
}

func checkWorkspace(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("workspace must be a real directory")
	}
	return nil
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
