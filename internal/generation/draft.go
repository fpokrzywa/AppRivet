package generation

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DraftStore keeps approved generation checkpoints in local user data. A
// zero value chooses the per-user directory. Directory can isolate a test.
type DraftStore struct {
	Directory string
}

type checkpoint struct {
	Approval       string         `json:"approval"`
	Plan           *ComponentPlan `json:"plan"`
	Clarifications []string       `json:"clarifications,omitempty"`
	Sources        []string       `json:"sources"`
}

func (s DraftStore) location(parent string, draft Draft) (string, string, error) {
	abs, err := filepath.Abs(parent)
	if err != nil {
		return "", "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", errors.New("could not resolve the selected project location")
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	base := s.Directory
	if base == "" {
		base, err = os.UserConfigDir()
		if runtime.GOOS == "windows" {
			base, err = os.UserCacheDir()
		}
		if err != nil || !filepath.IsAbs(base) {
			return "", "", errors.New("could not locate local AppRivet draft storage")
		}
		base = filepath.Join(base, "AppRivet", "generation-drafts")
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte("generation-draft-v1\n"+draft.Approval+"\n"+abs)))
	return base, filepath.Join(base, id+".json"), nil
}

// Review opens a saved plan without sending anything to a provider.
func (s DraftStore) Review(parent string, input PlanRequest) (PlanReview, error) {
	draft, err := input.draft()
	if err != nil {
		return PlanReview{}, err
	}
	base, path, err := s.location(parent, draft)
	if err != nil {
		return PlanReview{}, err
	}
	saved, err := loadCheckpoint(base, path)
	if err != nil {
		return PlanReview{}, err
	}
	if saved == nil {
		return reviewPlanning(draft, input.Clarifications), nil
	}
	input.Clarifications = saved.Clarifications
	if _, err := input.draft(); err != nil {
		return PlanReview{}, errors.New("the saved draft contains invalid planning clarifications")
	}
	review, err := reviewComponents(draft, saved.Plan, saved.Clarifications)
	if err != nil || saved.Approval != review.Approval || len(saved.Sources) > len(review.Prompts) {
		return PlanReview{}, errors.New("the saved draft no longer matches the approved plan")
	}
	review.Resuming, review.Completed = true, len(saved.Sources)
	return review, nil
}

func checkDraftDirectory(base string, create bool) error {
	if create {
		if err := os.MkdirAll(base, 0700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(base)
	if err != nil {
		return err
	}
	if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return errors.New("AppRivet draft storage must be a private directory")
	}
	return nil
}

func loadCheckpoint(base, path string) (*checkpoint, error) {
	if err := checkDraftDirectory(base, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	const maxBytes = 8 * 1024 * 1024
	entry, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !entry.Mode().IsRegular() || entry.Size() > maxBytes {
		return nil, errors.New("could not read the saved AppRivet draft")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if opened, err := file.Stat(); err != nil || !os.SameFile(entry, opened) {
		return nil, errors.New("the saved AppRivet draft changed while opening")
	}
	var saved checkpoint
	decoder := json.NewDecoder(io.LimitReader(file, maxBytes+1))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&saved) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("the saved AppRivet draft is unreadable")
	}
	return &saved, nil
}

func saveCheckpoint(base, path string, saved *checkpoint) error {
	file, err := os.CreateTemp(base, ".draft-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(saved)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
