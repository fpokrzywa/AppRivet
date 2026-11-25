package storeconnect

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func pemKey(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func unpackJWT(t *testing.T, token string) (map[string]any, map[string]any, []byte, [32]byte) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("invalid token format")
	}
	bytes := func(part string) []byte {
		decoded, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	var header, claims map[string]any
	if json.Unmarshal(bytes(parts[0]), &header) != nil || json.Unmarshal(bytes(parts[1]), &claims) != nil {
		t.Fatal("invalid token JSON")
	}
	return header, claims, bytes(parts[2]), sha256.Sum256([]byte(parts[0] + "." + parts[1]))
}

func TestAppleConnectionUsesReadOnlySignedRequest(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	for _, issuer := range []string{"", "11111111-1111-1111-1111-111111111111"} {
		client := &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet || request.URL.String() != appleApps {
				t.Fatal("did not use the read-only Apple endpoint")
			}
			header, claims, signature, hash := unpackJWT(t, strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
			if header["alg"] != "ES256" || header["kid"] != "A123456789" || claims["aud"] != "appstoreconnect-v1" ||
				claims["exp"] != float64(now.Unix()+300) || len(signature) != 64 ||
				!ecdsa.Verify(&key.PublicKey, hash[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
				t.Fatal("incorrect App Store Connect token")
			}
			if issuer == "" && claims["sub"] != "user" || issuer != "" && claims["iss"] != issuer {
				t.Fatal("incorrect individual or team token identity")
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		})}
		if result, err := checkApple(t.Context(), client, Settings{AppleKeyID: "A123456789", AppleIssuerID: issuer}, pemKey(t, key), now); err != nil || result.Store != "apple" {
			t.Fatalf("connection failed: %v", err)
		}
	}
}

func TestGoogleConnectionUsesFixedTokenHostAndHidesToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "fixture@example.iam.gserviceaccount.com",
		"private_key": pemKey(t, key), "private_key_id": "fixture-key", "token_uri": "https://foreign.example/token",
	})
	client := &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != googleToken || request.ParseForm() != nil ||
			request.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Fatal("did not use the fixed Google token endpoint")
		}
		header, claims, signature, hash := unpackJWT(t, request.Form.Get("assertion"))
		if header["alg"] != "RS256" || claims["aud"] != googleToken || claims["scope"] != "https://www.googleapis.com/auth/androidpublisher" ||
			rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], signature) != nil {
			t.Fatal("incorrect Google OAuth assertion")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"fixture-access-token"}`))}, nil
	})}
	result, err := checkGoogle(context.Background(), client, string(input), time.Now())
	if err != nil || result.Store != "google" || strings.Contains(result.Message, "fixture-access-token") {
		t.Fatalf("Google connection failed or returned its token: %v", err)
	}
}

type fixtureVault map[string]string

func (v fixtureVault) Get(slot string) (string, error) { return v[slot], nil }
func (v fixtureVault) Save(slot, value string) error   { v[slot] = value; return nil }
func (v fixtureVault) Remove(slot string) error        { delete(v, slot); return nil }

func TestStoreSettingsPersistOnlyMetadata(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := Store{Directory: t.TempDir(), Vault: fixtureVault{}}
	input := Update{Settings: Settings{AppleTeamID: "a123456789", AppleKeyID: "B123456789",
		AndroidUploadCertificateSHA256: strings.Repeat("ab:", 31) + "ab"}, ApplePrivateKey: pemKey(t, key)}
	status, err := s.Update(input)
	if err != nil || !status.HasAppleKey || status.HasGoogleKey || status.AppleTeamID != "A123456789" ||
		status.AndroidUploadCertificateSHA256 != strings.Repeat("AB", 32) {
		t.Fatalf("unexpected saved status: %+v %v", status, err)
	}
	body, err := os.ReadFile(filepath.Join(s.Directory, "publishing-settings.json"))
	if err != nil || strings.Contains(string(body), "PRIVATE KEY") {
		t.Fatal("credential material reached the configuration file")
	}
	status, err = s.Update(Update{Settings: status.Settings, RemoveAppleKey: true})
	if err != nil || status.HasAppleKey {
		t.Fatal("the credential was not removed")
	}
	if _, err := s.Test(t.Context(), ConnectionRequest{Store: "apple"}); err == nil {
		t.Fatal("attempted a connection without a credential")
	}
}

func TestStoreRollsBackCredentialsIfSettingsCannotBeSaved(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "publishing-settings.json"), 0700); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	vault := fixtureVault{}
	s := Store{Directory: dir, Vault: vault}
	if _, err := s.Update(Update{Settings: Settings{AppleKeyID: "B123456789"}, ApplePrivateKey: pemKey(t, key)}); err == nil || len(vault) != 0 {
		t.Fatal("partial update left a credential behind")
	}
}
