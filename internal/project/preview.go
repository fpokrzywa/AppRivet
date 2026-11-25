package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// PreviewFlutterWeb serves a completed Web build on a separate loopback
// origin until the caller closes its input or cancels the operation.
func PreviewFlutterWeb(ctx context.Context, project string, trusted bool, input io.Reader, ready io.Writer) error {
	if !trusted {
		return errors.New("confirm you trust this project before opening its web build")
	}
	info, err := Detect(project)
	if err != nil || info.Kind != "flutter-project" || !slices.Contains(info.Platforms, "web") {
		return errors.New("choose a Flutter project that includes Web")
	}
	root, err := os.OpenRoot(info.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	webPath := filepath.Join("build", "web")
	for _, dir := range []string{"build", webPath} {
		entry, err := root.Lstat(dir)
		if err != nil || !entry.IsDir() {
			return errors.New("build the Web target before opening the preview")
		}
	}
	web, err := root.OpenRoot(webPath)
	if err != nil {
		return err
	}
	defer web.Close()
	index, err := web.Lstat("index.html")
	if err != nil || !index.Mode().IsRegular() {
		return errors.New("the Web build does not have a regular index.html. Rebuild the Web target")
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	host := listener.Addr().String()
	files := web.FS()
	serveFile := http.FileServerFS(files)
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != host {
				http.Error(w, "Invalid preview host", http.StatusMisdirectedRequest)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
			if name == "" {
				name = "index.html"
			}
			entry, err := fs.Stat(files, name)
			if err != nil || !entry.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if path.Ext(name) == ".wasm" {
				w.Header().Set("Content-Type", "application/wasm")
			}
			serveFile.ServeHTTP(w, r)
		}),
	}
	defer server.Close()
	if _, err := fmt.Fprintf(ready, "http://%s/\n", host); err != nil {
		return err
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	closed := make(chan struct{})
	go func() {
		if input != nil {
			_, _ = io.Copy(io.Discard, input)
		}
		close(closed)
	}()
	select {
	case <-ctx.Done():
		return nil
	case <-closed:
		return nil
	case err := <-finished:
		return err
	}
}
