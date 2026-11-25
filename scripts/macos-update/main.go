// Command macos-update signs release archives with a Sparkle-compatible Ed25519 seed.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "keygen" {
		info, err := os.Stdout.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice != 0 {
			return errors.New("capture keygen output in a secure store")
		}
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		fmt.Println(base64.StdEncoding.EncodeToString(key.Seed()))
		return nil
	}

	publicOnly := len(args) == 1 && args[0] == "public-key"
	if !publicOnly && (len(args) != 3 || args[0] != "sign") {
		return errors.New("use keygen, public-key, or sign <public-key> <archive>; public-key and sign read the seed from stdin")
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 256))
	if err != nil {
		return err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(input)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("expected a base64-encoded Ed25519 seed on stdin")
	}
	key := ed25519.NewKeyFromSeed(seed)
	publicKey := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	if publicOnly {
		fmt.Println(publicKey)
		return nil
	}
	if args[1] != publicKey {
		return errors.New("signing key does not match the application's public update key")
	}
	archive, err := os.ReadFile(args[2])
	if err != nil {
		return err
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, archive))
	fmt.Printf(`sparkle:edSignature="%s" length="%d"`+"\n", signature, len(archive))
	return nil
}
