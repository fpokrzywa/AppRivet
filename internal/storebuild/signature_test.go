package storebuild

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

func TestVerifyBundleWithJDKSignatures(t *testing.T) {
	java := findJava()
	if java == "" {
		t.Skip("JDK unavailable")
	}
	keytool := filepath.Join(filepath.Dir(java), "keytool"+filepath.Ext(java))
	jarsigner := filepath.Join(filepath.Dir(java), "jarsigner"+filepath.Ext(java))
	if _, err := os.Stat(jarsigner); err != nil {
		t.Skip("JDK signing tools unavailable")
	}
	root := t.TempDir()
	verifier := filepath.Join(root, "VerifyBundle.java")
	writeFixture(t, verifier, string(verifyBundleSource))
	tool := func(path string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), path, args...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s fixture command failed: %v\n%s", filepath.Base(path), err, output)
		}
		return output
	}
	sign := func(label, subject string) (string, string) {
		t.Helper()
		const password = "fixturepass"
		keystore := filepath.Join(root, label+".p12")
		bundle := filepath.Join(root, label+".aab")
		cert := filepath.Join(root, label+".der")
		zipFixture(t, bundle, "BundleConfig.pb", "base/manifest/AndroidManifest.xml", "base/assets/test.txt")
		tool(keytool, "-genkeypair", "-keyalg", "RSA", "-storetype", "PKCS12", "-keystore", keystore,
			"-storepass", password, "-alias", "fixture", "-dname", subject, "-validity", "7")
		tool(jarsigner, "-keystore", keystore, "-storepass", password, bundle, "fixture")
		tool(keytool, "-exportcert", "-keystore", keystore, "-storepass", password, "-alias", "fixture", "-file", cert)
		der, err := os.ReadFile(cert)
		if err != nil {
			t.Fatal(err)
		}
		return bundle, fmt.Sprintf("%x", sha256.Sum256(der))
	}
	release, pin := sign("release", "CN=Fixture Upload")
	debug, debugPin := sign("debug", "CN=Android Debug,O=Android,C=US")
	unsigned := filepath.Join(root, "unsigned.aab")
	zipFixture(t, unsigned, "BundleConfig.pb", "base/manifest/AndroidManifest.xml")
	withUnsigned := rewriteBundle(t, release, "base/assets/extra.txt", false)
	tampered := rewriteBundle(t, release, "base/assets/test.txt", true)
	for _, test := range []struct {
		name, bundle, pin string
		accepted          bool
	}{
		{"pinned upload signer", release, pin, true},
		{"different pin", release, strings.Repeat("0", 64), false},
		{"debug even with matching pin", debug, debugPin, false},
		{"unsigned", unsigned, pin, false},
		{"unsigned entry in signed bundle", withUnsigned, pin, false},
		{"signed entry changed", tampered, pin, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := (runner.Exec{}).Run(t.Context(), runner.Spec{Path: java,
				Args: []string{verifier, test.bundle, test.pin}, Dir: root, Timeout: time.Minute})
			if (err == nil) != test.accepted {
				t.Fatalf("verification accepted=%t, want %t: %v\n%s", err == nil, test.accepted, err, result.Output)
			}
		})
	}
}

func rewriteBundle(t *testing.T, signed, entryName string, replace bool) string {
	t.Helper()
	input, err := zip.OpenReader(signed)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output := filepath.Join(t.TempDir(), "rewritten.aab")
	file, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, entry := range input.File {
		if replace && entry.Name == entryName {
			continue
		}
		if err := archive.Copy(entry); err != nil {
			t.Fatal(err)
		}
	}
	entry, err := archive.Create(entryName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, "different bytes"); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return output
}
