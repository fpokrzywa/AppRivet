// Package storebuild prepares local release artifacts from trusted app projects.
package storebuild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/project"
	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type Request struct {
	Project                 string `json:"project"`
	Platform                string `json:"platform"`
	TeamID                  string `json:"team_id"`
	UploadCertificateSHA256 string `json:"upload_certificate_sha256"`
	Trusted                 bool   `json:"trusted"`
}

type Result struct {
	Platform string `json:"platform"`
	Artifact string `json:"artifact"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
}

type builder struct {
	host        string
	releaseRoot string
	run         runner.Runner
	flutter     func(ctx context.Context, root, platform string) (string, error)
	java        func() string
}

var teamPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)
var digestPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
var colonDigestPattern = regexp.MustCompile(`^(?:[a-fA-F0-9]{2}:){31}[a-fA-F0-9]{2}$`)

func Build(ctx context.Context, req Request) (Result, error) {
	base, err := defaultReleaseRoot()
	if err != nil {
		return Result{}, err
	}
	b := builder{host: runtime.GOOS, releaseRoot: base, run: runner.Exec{}, flutter: project.RunFlutterStore, java: findJava}
	return b.build(ctx, req)
}

func (b builder) build(ctx context.Context, req Request) (Result, error) {
	if !req.Trusted {
		return Result{}, errors.New("review the project and confirm you trust its build scripts")
	}
	if strings.TrimSpace(req.Project) == "" {
		return Result{}, errors.New("select a project to build for the store")
	}
	var java string
	switch req.Platform {
	case "ios":
		if b.host != "darwin" {
			return Result{}, errors.New("iOS store builds require macOS with full Xcode")
		}
		req.TeamID = strings.ToUpper(strings.TrimSpace(req.TeamID))
		if !teamPattern.MatchString(req.TeamID) {
			return Result{}, errors.New("enter the 10-character Apple Developer Team ID")
		}
	case "android":
		pin := strings.TrimSpace(req.UploadCertificateSHA256)
		if !digestPattern.MatchString(pin) && !colonDigestPattern.MatchString(pin) {
			return Result{}, errors.New("enter the SHA-256 fingerprint of the Android upload certificate")
		}
		req.UploadCertificateSHA256 = strings.ToLower(strings.ReplaceAll(pin, ":", ""))
		java = b.java()
		if java == "" {
			return Result{}, errors.New("install a Java 17 or later JDK and set JAVA_HOME to verify the Android upload signature")
		}
	default:
		return Result{}, errors.New("choose the iOS or Android release platform")
	}
	info, err := project.Detect(req.Project)
	if err != nil {
		return Result{}, err
	}
	if info.Kind == "flutter-project" {
		if !slices.Contains(info.Platforms, req.Platform) {
			return Result{}, errors.New("this Flutter project does not include the requested platform")
		}
	} else if req.Platform == "android" || (info.Kind != "xcode-project" && info.Kind != "xcode-workspace") {
		return Result{}, errors.New("store builds require a Flutter app or an iOS Xcode app project")
	}

	bucket, err := releaseBucket(b.releaseRoot, info.Root, true)
	if err != nil {
		return Result{}, err
	}
	stage, err := os.MkdirTemp(bucket, ".pending-")
	if err != nil {
		return Result{}, errors.New("could not create a private release build directory")
	}
	defer os.RemoveAll(stage)
	work := filepath.Join(stage, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	name := artifactName(req.Platform)
	destination := filepath.Join(stage, name)
	var result Result
	if req.Platform == "ios" {
		result, err = b.archiveIOS(ctx, info, req.TeamID, work, destination)
	} else {
		result, err = b.buildAndroid(ctx, info.Root, java, req.UploadCertificateSHA256, work, destination)
	}
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := os.RemoveAll(work); err != nil {
		return Result{}, errors.New("could not remove temporary release build files")
	}
	result.Platform = req.Platform
	dir, err := publish(stage, result)
	if err != nil {
		return Result{}, err
	}
	result.Artifact = filepath.Join(dir, name)
	return result, nil
}

func List(projectPath string) ([]Result, error) {
	base, err := defaultReleaseRoot()
	if err != nil {
		return nil, err
	}
	return (builder{releaseRoot: base}).list(projectPath)
}
