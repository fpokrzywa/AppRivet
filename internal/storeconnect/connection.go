package storeconnect

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/fpokrzywa/AppRivet/internal/credentials"
)

const (
	appleApps   = "https://api.appstoreconnect.apple.com/v1/apps?limit=1"
	googleToken = "https://oauth2.googleapis.com/token"
)

var connectionClient = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

type ConnectionRequest struct {
	Store                string `json:"store"`
	ApplePrivateKey      string `json:"apple_private_key,omitempty"`
	GoogleServiceAccount string `json:"google_service_account,omitempty"`
}

type ConnectionResult struct {
	Store   string `json:"store"`
	Message string `json:"message"`
}

type googleAccount struct {
	Type       string `json:"type"`
	Email      string `json:"client_email"`
	KeyID      string `json:"private_key_id"`
	PrivateKey string `json:"private_key"`
}

func parseAppleKey(value string) (*ecdsa.PrivateKey, error) {
	invalid := errors.New("choose an App Store Connect .p8 private key")
	if len(value) > 8*1024 {
		return nil, invalid
	}
	key, err := parsePKCS8(value)
	if err != nil {
		return nil, invalid
	}
	private, ok := key.(*ecdsa.PrivateKey)
	if !ok || private.Curve.Params().Name != "P-256" {
		return nil, invalid
	}
	return private, nil
}

func parseGoogleAccount(value string) (googleAccount, *rsa.PrivateKey, error) {
	invalid := errors.New("choose a Google Cloud service account .json key")
	var account googleAccount
	if len(value) > 24*1024 || json.Unmarshal([]byte(value), &account) != nil || account.Type != "service_account" || len(account.Email) > 254 {
		return googleAccount{}, nil, invalid
	}
	email, err := mail.ParseAddress(account.Email)
	if err != nil || email.Address != account.Email || !strings.HasSuffix(account.Email, ".gserviceaccount.com") {
		return googleAccount{}, nil, invalid
	}
	key, err := parsePKCS8(account.PrivateKey)
	if err != nil {
		return googleAccount{}, nil, invalid
	}
	private, ok := key.(*rsa.PrivateKey)
	if !ok || private.N.BitLen() < 2048 || private.N.BitLen() > 4096 || len(account.KeyID) > 128 {
		return googleAccount{}, nil, invalid
	}
	return account, private, nil
}

func parsePKCS8(value string) (any, error) {
	block, rest := pem.Decode([]byte(strings.TrimSpace(value)))
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("invalid private key file")
	}
	return x509.ParsePKCS8PrivateKey(block.Bytes)
}

func (s Store) Test(ctx context.Context, input ConnectionRequest) (ConnectionResult, error) {
	settings, err := s.Load()
	if err != nil {
		return ConnectionResult{}, err
	}
	switch input.Store {
	case "apple":
		if input.ApplePrivateKey == "" && s.Vault != nil {
			saved, err := s.Vault.Get(credentials.AppStoreConnect)
			if err != nil || (saved != "" && json.Unmarshal([]byte(saved), &input.ApplePrivateKey) != nil) {
				return ConnectionResult{}, errors.New("could not load the App Store Connect key from the system credential store")
			}
		}
		return checkApple(ctx, connectionClient, settings, input.ApplePrivateKey, time.Now())
	case "google":
		if input.GoogleServiceAccount == "" && s.Vault != nil {
			input.GoogleServiceAccount, err = s.Vault.Get(credentials.GooglePlay)
			if err != nil {
				return ConnectionResult{}, errors.New("could not load the Google Play key from the system credential store")
			}
		}
		return checkGoogle(ctx, connectionClient, input.GoogleServiceAccount, time.Now())
	default:
		return ConnectionResult{}, errors.New("choose App Store Connect or Google Play")
	}
}

func checkApple(ctx context.Context, client *http.Client, settings Settings, value string, now time.Time) (ConnectionResult, error) {
	if !appleID.MatchString(settings.AppleKeyID) || value == "" {
		return ConnectionResult{}, errors.New("save the App Store Connect Key ID and .p8 key before checking the connection")
	}
	key, err := parseAppleKey(value)
	if err != nil {
		return ConnectionResult{}, err
	}
	claims := map[string]any{
		"aud": "appstoreconnect-v1", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}
	if settings.AppleIssuerID == "" {
		claims["sub"] = "user"
	} else {
		claims["iss"] = settings.AppleIssuerID
	}
	token, err := signedJWT("ES256", settings.AppleKeyID, claims, func(hash []byte) ([]byte, error) {
		r, s, err := ecdsa.Sign(rand.Reader, key, hash)
		if err != nil {
			return nil, err
		}
		var signature [64]byte
		r.FillBytes(signature[:32])
		s.FillBytes(signature[32:])
		return signature[:], nil
	})
	if err != nil {
		return ConnectionResult{}, errors.New("could not sign an App Store Connect token")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, appleApps, nil)
	if err != nil {
		return ConnectionResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return ConnectionResult{}, errors.New("could not reach App Store Connect")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ConnectionResult{}, fmt.Errorf("App Store Connect returned HTTP %d. Check the key, issuer and app access", response.StatusCode)
	}
	return ConnectionResult{Store: "apple", Message: "App Store Connect accepted the key and allowed access to the app list."}, nil
}

func checkGoogle(ctx context.Context, client *http.Client, value string, now time.Time) (ConnectionResult, error) {
	if value == "" {
		return ConnectionResult{}, errors.New("save the Google Cloud service account JSON before checking the connection")
	}
	account, key, err := parseGoogleAccount(value)
	if err != nil {
		return ConnectionResult{}, err
	}
	token, err := signedJWT("RS256", account.KeyID, map[string]any{
		"iss": account.Email, "aud": googleToken, "scope": "https://www.googleapis.com/auth/androidpublisher",
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}, func(hash []byte) ([]byte, error) {
		return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash)
	})
	if err != nil {
		return ConnectionResult{}, errors.New("could not sign the Google service account assertion")
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {token}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, googleToken, strings.NewReader(form.Encode()))
	if err != nil {
		return ConnectionResult{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return ConnectionResult{}, errors.New("could not reach Google authorization")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ConnectionResult{}, fmt.Errorf("Google authorization returned HTTP %d. Check the service account and key", response.StatusCode)
	}
	var reply struct {
		AccessToken string `json:"access_token"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 32*1024)).Decode(&reply) != nil || reply.AccessToken == "" {
		return ConnectionResult{}, errors.New("Google did not return an access token")
	}
	return ConnectionResult{Store: "google", Message: "Google issued a service account token. App and testing-track permissions still need to be verified in Play Console."}, nil
}

func signedJWT(algorithm, keyID string, claims map[string]any, sign func([]byte) ([]byte, error)) (string, error) {
	header := map[string]string{"alg": algorithm, "typ": "JWT"}
	if keyID != "" {
		header["kid"] = keyID
	}
	head, _ := json.Marshal(header)
	body, _ := json.Marshal(claims)
	encoding := base64.RawURLEncoding
	unsigned := encoding.EncodeToString(head) + "." + encoding.EncodeToString(body)
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := sign(hash[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + encoding.EncodeToString(signature), nil
}
