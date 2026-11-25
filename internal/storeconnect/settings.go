// Package storeconnect manages local publishing profiles and checks store authentication.
package storeconnect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fpokrzywa/AppRivet/internal/credentials"
)

type Settings struct {
	AppleTeamID                    string `json:"apple_team_id"`
	AppleIssuerID                  string `json:"apple_issuer_id"`
	AppleKeyID                     string `json:"apple_key_id"`
	AndroidUploadCertificateSHA256 string `json:"android_upload_certificate_sha256"`
}

type Update struct {
	Settings
	ApplePrivateKey      string `json:"apple_private_key,omitempty"`
	GoogleServiceAccount string `json:"google_service_account,omitempty"`
	RemoveAppleKey       bool   `json:"remove_apple_key,omitempty"`
	RemoveGoogleKey      bool   `json:"remove_google_key,omitempty"`
}

type Status struct {
	Settings
	HasAppleKey       bool `json:"has_apple_key"`
	HasGoogleKey      bool `json:"has_google_key"`
	CredentialStorage bool `json:"credential_storage"`
}

type Store struct {
	Directory string
	Vault     credentials.Vault
}

var (
	appleID     = regexp.MustCompile(`^[A-Z0-9]{10}$`)
	issuerID    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	fingerprint = regexp.MustCompile(`^[A-F0-9]{64}$`)
)

func Local(vault credentials.Vault) (Store, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return Store{}, errors.New("could not locate the local AppRivet settings folder")
	}
	return Store{Directory: filepath.Join(config, "AppRivet"), Vault: vault}, nil
}

func normalize(settings Settings) (Settings, error) {
	settings.AppleTeamID = strings.ToUpper(strings.TrimSpace(settings.AppleTeamID))
	settings.AppleKeyID = strings.ToUpper(strings.TrimSpace(settings.AppleKeyID))
	settings.AppleIssuerID = strings.ToLower(strings.TrimSpace(settings.AppleIssuerID))
	settings.AndroidUploadCertificateSHA256 = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(settings.AndroidUploadCertificateSHA256), ":", ""))
	if settings.AppleTeamID != "" && !appleID.MatchString(settings.AppleTeamID) {
		return Settings{}, errors.New("enter the ten-character Apple Developer Team ID")
	}
	if settings.AppleKeyID != "" && !appleID.MatchString(settings.AppleKeyID) {
		return Settings{}, errors.New("enter the ten-character App Store Connect Key ID")
	}
	if settings.AppleIssuerID != "" && !issuerID.MatchString(settings.AppleIssuerID) {
		return Settings{}, errors.New("enter the App Store Connect Issuer ID or leave it blank for an individual key")
	}
	if settings.AndroidUploadCertificateSHA256 != "" && !fingerprint.MatchString(settings.AndroidUploadCertificateSHA256) {
		return Settings{}, errors.New("enter the SHA-256 fingerprint of the Android upload certificate")
	}
	return settings, nil
}

func (s Store) Load() (Settings, error) {
	path := filepath.Join(s.Directory, "publishing-settings.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return Settings{}, errors.New("could not read the local publishing settings")
	}
	file, err := os.Open(path)
	if err != nil {
		return Settings{}, errors.New("could not open the local publishing settings")
	}
	defer file.Close()
	var settings Settings
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&settings) != nil || decoder.Decode(new(any)) != io.EOF {
		return Settings{}, errors.New("the local publishing settings are invalid")
	}
	return normalize(settings)
}

func (s Store) Save(settings Settings) (Settings, error) {
	settings, err := normalize(settings)
	if err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return Settings{}, errors.New("could not create the local publishing settings folder")
	}
	if info, err := os.Lstat(s.Directory); err != nil || !info.IsDir() {
		return Settings{}, errors.New("the publishing settings folder must be a real directory")
	}
	file, err := os.CreateTemp(s.Directory, "publishing-*.tmp")
	if err != nil {
		return Settings{}, errors.New("could not stage the local publishing settings")
	}
	defer os.Remove(file.Name())
	writeErr := json.NewEncoder(file).Encode(settings)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return Settings{}, errors.New("could not write the local publishing settings")
	}
	if err := os.Rename(file.Name(), filepath.Join(s.Directory, "publishing-settings.json")); err != nil {
		return Settings{}, errors.New("could not replace the local publishing settings")
	}
	return settings, nil
}

func (s Store) Status() (Status, error) {
	settings, err := s.Load()
	if err != nil {
		return Status{}, err
	}
	status := Status{Settings: settings, CredentialStorage: s.Vault != nil}
	if s.Vault != nil {
		apple, _ := s.Vault.Get(credentials.AppStoreConnect)
		google, _ := s.Vault.Get(credentials.GooglePlay)
		status.HasAppleKey, status.HasGoogleKey = apple != "", google != ""
	}
	return status, nil
}

func (s Store) Update(update Update) (Status, error) {
	settings, err := normalize(update.Settings)
	if err != nil {
		return Status{}, err
	}
	if (update.RemoveAppleKey && update.ApplePrivateKey != "") || (update.RemoveGoogleKey && update.GoogleServiceAccount != "") {
		return Status{}, errors.New("choose either replace or remove for each store credential")
	}
	appleValue, googleValue := "", ""
	if update.ApplePrivateKey != "" {
		if _, err := parseAppleKey(update.ApplePrivateKey); err != nil {
			return Status{}, err
		}
		if settings.AppleKeyID == "" {
			return Status{}, errors.New("enter the Key ID for the Apple API private key")
		}
		value, _ := json.Marshal(strings.TrimSpace(update.ApplePrivateKey))
		appleValue = string(value)
	}
	if update.GoogleServiceAccount != "" {
		account, _, err := parseGoogleAccount(update.GoogleServiceAccount)
		if err != nil {
			return Status{}, err
		}
		value, _ := json.Marshal(account)
		googleValue = string(value)
	}
	changes := []struct {
		slot, value, previous string
		remove                bool
	}{
		{slot: credentials.AppStoreConnect, value: appleValue, remove: update.RemoveAppleKey},
		{slot: credentials.GooglePlay, value: googleValue, remove: update.RemoveGoogleKey},
	}
	for i := range changes {
		change := &changes[i]
		if change.value == "" && !change.remove {
			continue
		}
		if s.Vault == nil {
			return Status{}, errors.New("system credential storage is unavailable")
		}
		change.previous, err = s.Vault.Get(change.slot)
		if err != nil {
			return Status{}, errors.New("could not read the system credential store")
		}
	}
	for i, change := range changes {
		if change.remove {
			err = s.Vault.Remove(change.slot)
		} else if change.value != "" {
			err = s.Vault.Save(change.slot, change.value)
		}
		if err != nil {
			if undoErr := s.restoreCredentials(changes[:i+1]); undoErr != nil {
				return Status{}, errors.New("the credential update failed and could not be restored. Reopen Settings to check the current entries")
			}
			return Status{}, fmt.Errorf("could not update %s credentials in the system credential store", change.slot)
		}
	}
	if _, err := s.Save(settings); err != nil {
		if undoErr := s.restoreCredentials(changes); undoErr != nil {
			return Status{}, errors.New("settings could not be saved and the credential store could not be restored. Reopen Settings to check the current entries")
		}
		return Status{}, err
	}
	return s.Status()
}

func (s Store) restoreCredentials(changes []struct {
	slot, value, previous string
	remove                bool
}) error {
	var result error
	for _, change := range changes {
		if change.value == "" && !change.remove {
			continue
		}
		if change.previous == "" {
			result = errors.Join(result, s.Vault.Remove(change.slot))
		} else {
			result = errors.Join(result, s.Vault.Save(change.slot, change.previous))
		}
	}
	return result
}
