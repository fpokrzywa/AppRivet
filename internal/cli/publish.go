package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/fpokrzywa/AppRivet/internal/runner"
	"github.com/fpokrzywa/AppRivet/internal/storebuild"
	"github.com/fpokrzywa/AppRivet/internal/storeconnect"
)

func (a App) runPublish(ctx context.Context, args []string, jsonMode bool) int {
	response := Envelope{SchemaVersion: 1, Command: "publish", OK: true}
	fail := func(message string, exit int) int {
		response.OK = false
		response.Error = &Error{Code: "publish_failed", Message: runner.Redact(message)}
		return a.emit(response, jsonMode, exit)
	}
	if len(args) != 1 || !jsonMode || a.In == nil {
		return fail("use apprivet publish with an action, --json, and a JSON request on stdin", 2)
	}
	settings, err := storeconnect.Local(nil)
	if err != nil {
		return fail(err.Error(), 4)
	}
	decoder := json.NewDecoder(io.LimitReader(a.In, 40*1024))
	decoder.DisallowUnknownFields()
	decode := func(value any) error {
		if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
			return errors.New("provide exactly one valid publishing request")
		}
		return nil
	}
	switch args[0] {
	case "settings":
		if err := decode(new(struct{})); err != nil {
			return fail(err.Error(), 2)
		}
		response.Data, err = settings.Load()
	case "save-settings":
		var input storeconnect.Settings
		if err := decode(&input); err != nil {
			return fail(err.Error(), 2)
		}
		response.Data, err = settings.Save(input)
	case "connection":
		var input storeconnect.ConnectionRequest
		if err := decode(&input); err != nil {
			return fail(err.Error(), 2)
		}
		response.Data, err = settings.Test(ctx, input)
	case "build":
		var input struct {
			Project  string `json:"project"`
			Platform string `json:"platform"`
			Trusted  bool   `json:"trusted"`
		}
		if err := decode(&input); err != nil {
			return fail(err.Error(), 2)
		}
		var saved storeconnect.Settings
		saved, err = settings.Load()
		if err == nil {
			response.Data, err = storebuild.Build(ctx, storebuild.Request{
				Project: input.Project, Platform: input.Platform, Trusted: input.Trusted,
				TeamID: saved.AppleTeamID, UploadCertificateSHA256: saved.AndroidUploadCertificateSHA256,
			})
		}
	case "history":
		var input struct {
			Project string `json:"project"`
		}
		if err := decode(&input); err != nil {
			return fail(err.Error(), 2)
		}
		response.Data, err = storebuild.List(input.Project)
	default:
		return fail("choose a publishing action: settings, save-settings, connection, build or history", 2)
	}
	if err != nil {
		response.Data = nil
		return fail(err.Error(), 4)
	}
	return a.emit(response, jsonMode, 0)
}
