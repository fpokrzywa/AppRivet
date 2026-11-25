package storebuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/project"
)

type releaseRecord struct {
	Platform  string    `json:"platform"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func defaultReleaseRoot() (string, error) {
	baseDir := os.UserConfigDir
	if runtime.GOOS == "windows" {
		baseDir = os.UserCacheDir // LocalAppData stays on this machine.
	}
	base, err := baseDir()
	if err != nil || !filepath.IsAbs(base) {
		return "", errors.New("could not locate the local AppRivet data directory")
	}
	return filepath.Join(base, "AppRivet", "releases"), nil
}

func releaseBucket(base, projectRoot string, create bool) (string, error) {
	projectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", err
	}
	parent, err := resolveProspectivePath(filepath.Dir(base))
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(parent, filepath.Base(base))
	if relative, err := filepath.Rel(projectRoot, resolved); err == nil && filepath.IsLocal(relative) {
		return "", errors.New("the AppRivet release directory must be outside the project")
	}
	if create {
		if err := os.MkdirAll(resolved, 0700); err != nil {
			return "", errors.New("could not create the local AppRivet release directory")
		}
	}
	if err := privateDirectory(resolved); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		projectRoot = strings.ToLower(projectRoot)
	}
	key := sha256.Sum256([]byte(projectRoot))
	bucket := filepath.Join(resolved, hex.EncodeToString(key[:]))
	if create {
		if err := os.Mkdir(bucket, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", errors.New("could not create the project's private release directory")
		}
	}
	if err := privateDirectory(bucket); err != nil {
		return "", err
	}
	return bucket, nil
}

func privateDirectory(path string) error {
	entry, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !entry.IsDir() || (runtime.GOOS != "windows" && entry.Mode().Perm()&0077 != 0) {
		return errors.New("the AppRivet release directory must be a private directory")
	}
	return nil
}

// Resolve existing parents before creating directories so aliases cannot place
// release output inside the selected project.
func resolveProspectivePath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return resolved, err
	}
	parent := filepath.Dir(path)
	if parent == path || !filepath.IsAbs(path) {
		return "", err
	}
	resolved, err = resolveProspectivePath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

func artifactName(platform string) string {
	switch platform {
	case "ios":
		return "app.ipa"
	case "android":
		return "app.aab"
	}
	return ""
}

func publish(stage string, result Result) (string, error) {
	record := releaseRecord{Platform: result.Platform, SHA256: result.SHA256, Size: result.Size, CreatedAt: time.Now().UTC()}
	body, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, "release.json"), body, 0600); err != nil {
		return "", err
	}
	final := filepath.Join(filepath.Dir(stage), "release-"+strings.TrimPrefix(filepath.Base(stage), ".pending-"))
	if err := os.Rename(stage, final); err != nil {
		return "", errors.New("could not publish the local release artifact")
	}
	return final, nil
}

func (b builder) list(path string) ([]Result, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("select a project to list its release builds")
	}
	info, err := project.Detect(path)
	if err != nil {
		return nil, err
	}
	results := []Result{}
	bucket, err := releaseBucket(b.releaseRoot, info.Root, false)
	if errors.Is(err, os.ErrNotExist) {
		return results, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(bucket)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := readDirectory(root, ".")
	if err != nil {
		return nil, err
	}
	type entry struct {
		Result
		createdAt time.Time
	}
	var releases []entry
	for _, dir := range entries {
		if !dir.IsDir() || !strings.HasPrefix(dir.Name(), "release-") {
			continue
		}
		record, ok := readRecord(root, filepath.Join(dir.Name(), "release.json"))
		if !ok {
			continue
		}
		artifact := filepath.Join(dir.Name(), artifactName(record.Platform))
		stat, err := root.Lstat(artifact)
		if err != nil || !stat.Mode().IsRegular() || stat.Size() != record.Size {
			continue
		}
		releases = append(releases, entry{Result: Result{Platform: record.Platform, Artifact: filepath.Join(bucket, artifact),
			SHA256: record.SHA256, Size: record.Size}, createdAt: record.CreatedAt})
	}
	slices.SortFunc(releases, func(a, b entry) int { return b.createdAt.Compare(a.createdAt) })
	for _, release := range releases {
		results = append(results, release.Result)
	}
	return results, nil
}

func readRecord(root *os.Root, path string) (releaseRecord, bool) {
	var record releaseRecord
	stat, err := root.Lstat(path)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 4096 {
		return record, false
	}
	file, err := root.Open(path)
	if err != nil {
		return record, false
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 4097))
	ok := err == nil && len(body) <= 4096 && json.Unmarshal(body, &record) == nil &&
		artifactName(record.Platform) != "" && digestPattern.MatchString(record.SHA256) && record.Size > 0 && !record.CreatedAt.IsZero()
	return record, ok
}

func copyArtifact(source *os.Root, name, destination string) (Result, error) {
	info, err := source.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return Result{}, errors.New("the tool did not produce a regular release artifact")
	}
	input, err := source.Open(name)
	if err != nil {
		return Result{}, err
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return Result{}, errors.New("the release artifact changed while it was being opened")
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Result{}, err
	}
	digest := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(output, digest), input)
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return Result{}, err
	}
	if n != opened.Size() {
		return Result{}, errors.New("the release artifact changed while it was being copied")
	}
	return Result{Size: n, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func readDirectory(root *os.Root, name string) ([]os.DirEntry, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.ReadDir(-1)
}
