package project

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFlutterWebPreviewServesOnlyBuiltFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"lib", "web", "build/web/assets"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(name)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for name, contents := range map[string]string{
		"pubspec.yaml": "name: preview", "lib/main.dart": "private project source",
		"build/web/index.html": "<html>Web preview</html>", "build/web/assets/app.wasm": "wasm",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := PreviewFlutterWeb(t.Context(), root, false, nil, io.Discard); err == nil {
		t.Fatal("opened a preview without trust")
	}
	linked := os.Symlink(filepath.Join(root, "lib", "main.dart"), filepath.Join(root, "build", "web", "linked.txt")) == nil
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	input, lifetime := io.Pipe()
	defer lifetime.Close()
	readURL, writeURL := io.Pipe()
	defer readURL.Close()
	finished := make(chan error, 1)
	go func() {
		err := PreviewFlutterWeb(ctx, root, true, input, writeURL)
		_ = writeURL.CloseWithError(err)
		finished <- err
	}()
	stopReading := context.AfterFunc(ctx, func() { _ = readURL.CloseWithError(ctx.Err()) })
	defer stopReading()
	line, err := bufio.NewReader(readURL).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "http://127.0.0.1:") {
		t.Fatalf("expected a ready loopback preview: %q, %v", line, err)
	}
	address := strings.TrimSpace(line)
	client := &http.Client{Timeout: 2 * time.Second}
	for _, check := range []struct {
		path, method, host, body, contentType string
		status                                int
	}{
		{"", "GET", "", "<html>Web preview</html>", "text/html", 200},
		{"assets/app.wasm", "GET", "", "wasm", "application/wasm", 200},
		{"assets/", "GET", "", "", "", 404},
		{"../../lib/main.dart", "GET", "", "", "", 404},
		{"", "POST", "", "", "", 405},
		{"", "GET", "wrong-host.example", "", "", 421},
		{"linked.txt", "GET", "", "", "", 404},
	} {
		if check.path == "linked.txt" && !linked {
			continue
		}
		request, err := http.NewRequestWithContext(ctx, check.method, address+check.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if check.host != "" {
			request.Host = check.host
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != check.status || (check.body != "" && string(body) != check.body) ||
			!strings.HasPrefix(response.Header.Get("Content-Type"), check.contentType) {
			t.Fatalf("%s %s returned %d, %q, %v", check.method, check.path, response.StatusCode, body, err)
		}
	}
	lifetime.Close()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("preview did not stop when its owner closed the input")
	}
}
