package studio

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/runner"
	"github.com/fpokrzywa/AppRivet/internal/storebuild"
	"github.com/fpokrzywa/AppRivet/internal/storeconnect"
)

func registerPublishHandlers(mux *http.ServeMux, config Config, projectPath func(string) (string, error)) {
	store, locateErr := storeconnect.Local(config.Vault)
	if config.PublishStore != nil {
		store, locateErr = *config.PublishStore, nil
	}
	var settingsLock sync.Mutex
	settings := func(w http.ResponseWriter, r *http.Request) {
		if locateErr != nil {
			writeError(w, http.StatusServiceUnavailable, locateErr.Error())
			return
		}
		settingsLock.Lock()
		defer settingsLock.Unlock()
		var value storeconnect.Status
		var err error
		if r.Method == http.MethodGet {
			value, err = store.Status()
		} else {
			var update storeconnect.Update
			if !decodePublish(w, r, &update, 40*1024) {
				return
			}
			value, err = store.Update(update)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
	mux.HandleFunc("GET /api/publish/settings", settings)
	mux.HandleFunc("POST /api/publish/settings", settings)
	mux.HandleFunc("POST /api/publish/connection", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Store string `json:"store"`
		}
		if !decodePublish(w, r, &input, 1024) {
			return
		}
		if locateErr != nil {
			writeError(w, http.StatusServiceUnavailable, locateErr.Error())
			return
		}
		settingsLock.Lock()
		defer settingsLock.Unlock()
		result, err := store.Test(r.Context(), storeconnect.ConnectionRequest{Store: input.Store})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/publish/build", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name     string `json:"name"`
			Platform string `json:"platform"`
			Trusted  bool   `json:"trusted"`
		}
		if !decodePublish(w, r, &input, 2048) {
			return
		}
		path, err := projectPath(input.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if locateErr != nil {
			writeError(w, http.StatusServiceUnavailable, locateErr.Error())
			return
		}
		settingsLock.Lock()
		profile, err := store.Load()
		settingsLock.Unlock()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(31 * time.Minute))
		result, err := storebuild.Build(r.Context(), storebuild.Request{
			Project: path, Platform: input.Platform, Trusted: input.Trusted,
			TeamID: profile.AppleTeamID, UploadCertificateSHA256: profile.AndroidUploadCertificateSHA256,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, runner.Redact(err.Error()))
			return
		}
		writeJSON(w, http.StatusCreated, result)
	})
	mux.HandleFunc("POST /api/publish/history", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name string `json:"name"`
		}
		if !decodePublish(w, r, &input, 2048) {
			return
		}
		path, err := projectPath(input.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := storebuild.List(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/publish/open-artifact", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name   string `json:"name"`
			SHA256 string `json:"sha256"`
		}
		if !decodePublish(w, r, &input, 2048) {
			return
		}
		path, err := projectPath(input.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		results, err := storebuild.List(path)
		if err == nil && config.OpenFolder != nil {
			for _, result := range results {
				if result.SHA256 == input.SHA256 && config.OpenFolder(filepath.Dir(result.Artifact)) == nil {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		writeError(w, http.StatusBadRequest, "Could not open this project's release folder.")
	})
}

func decodePublish(w http.ResponseWriter, r *http.Request, value any, limit int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, http.StatusBadRequest, "Enter a valid publishing request.")
		return false
	}
	return true
}
