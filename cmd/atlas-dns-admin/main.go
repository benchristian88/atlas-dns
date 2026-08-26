package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/benchristian88/atlas-dns/internal/auth"
	"github.com/benchristian88/atlas-dns/internal/config"
	"github.com/benchristian88/atlas-dns/internal/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 || os.Args[1] != "reset-mfa" {
		return fmt.Errorf("usage: atlas-dns-admin reset-mfa --email <local-login>")
	}
	flags := flag.NewFlagSet("reset-mfa", flag.ContinueOnError)
	email := flags.String("email", "", "local Atlas login email")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := database.Open(ctx, configuration.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	tokens, err := auth.NewTokenManager(configuration.SessionSecret)
	if err != nil {
		return err
	}
	cipher, err := auth.NewCredentialCipher(configuration.CredentialEncryptionKey)
	if err != nil {
		return err
	}
	service, err := auth.NewService(store, tokens, configuration.SessionDuration, cipher)
	if err != nil {
		return err
	}
	user, revoked, err := service.ResetMFAFromHost(ctx, *email)
	if err != nil {
		return err
	}
	fmt.Printf("MFA reset for %s; %d active session(s) revoked.\n", user.Email, revoked)
	return nil
}
