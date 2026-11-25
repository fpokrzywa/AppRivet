package credentials

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os/exec"
	"strings"
	"time"
)

const secretServiceName = "com.github.fpokrzywa.AppRivet.ProviderKey"

var (
	errSecretService = errors.New("could not access Secret Service")
	errNoItem        = errors.New("no matching Secret Service item")
)

type secretTool string

// NewVault returns the available operating-system credential store, or nil.
func NewVault() Vault {
	path, err := exec.LookPath("secret-tool")
	if err != nil {
		return nil
	}
	v := secretTool(path)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// A nonexistent, random item checks the service without accessing saved keys.
	if _, err := v.run(ctx, "probe-"+rand.Text(), "", "lookup"); err != nil && err != errNoItem {
		return nil
	}
	return v
}

func (v secretTool) Get(provider string) (string, error) {
	if !validSlot(provider) {
		return "", errProvider
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	key, err := v.run(ctx, provider, "", "lookup")
	if err == errNoItem {
		return "", nil
	}
	return strings.TrimSuffix(key, "\n"), err
}

func (v secretTool) Save(provider, key string) error {
	if !validSlot(provider) {
		return errProvider
	}
	if !validStoredValue(provider, key) {
		return errKey
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := v.run(ctx, provider, key, "store", "--label=AppRivet API key ("+provider+")")
	return err
}

func (v secretTool) Remove(provider string) error {
	if !validSlot(provider) {
		return errProvider
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Lookup can unlock the item. Clear can only delete unlocked items.
	if _, err := v.run(ctx, provider, "", "lookup"); err == errNoItem {
		return nil
	} else if err != nil {
		return err
	}
	_, err := v.run(ctx, provider, "", "clear")
	return err
}

func (v secretTool) run(ctx context.Context, provider, key string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, string(v), append(args, "service", secretServiceName, "provider", provider)...)
	cmd.WaitDelay = time.Second
	if key != "" {
		cmd.Stdin = strings.NewReader(key)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	defer clear(output)
	if err != nil {
		var status *exec.ExitError
		if args[0] == "lookup" && errors.As(err, &status) && status.ExitCode() == 1 && stderr.Len() == 0 {
			return "", errNoItem
		}
		return "", errSecretService
	}
	if len(bytes.TrimSuffix(output, []byte("\n"))) > slotSize(provider) {
		return "", errKey
	}
	return string(output), nil
}
