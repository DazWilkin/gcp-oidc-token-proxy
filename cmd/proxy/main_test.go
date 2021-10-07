package main

import (
	"context"
	stdlog "log"
	"os"
	"testing"
	"time"

	"github.com/go-logr/stdr"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

const (
	delay time.Duration = 5 * time.Minute
)

func TestExpiry(t *testing.T) {
	log = stdr.NewWithOptions(
		stdlog.New(
			os.Stdout,
			"",
			stdlog.LstdFlags,
		),
		stdr.Options{
			LogCaller: stdr.All,
		},
	)
	log := log.WithName("test")

	// Proxy requires Application Default Credentials to authenticate with Google
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		t.Fatal("Unable to find GOOGLE_APPLICATION_CREDENTIALS in the environment")
	}

	// Test requires ENDPOINT to determine identity token audience
	audience := os.Getenv("ENDPOINT")
	if audience == "" {
		t.Fatal("Unable to find ENDPOINT in the environment")
	}

	log.Info("Initializing TokenSource")
	ts, err := idtoken.NewTokenSource(context.Background(), audience)
	if err != nil {
		t.Fatalf("Expected new TokenSource; got: %s", err)
	}

	log.Info("Initializing map of Tokens")
	tokens := map[string]*oauth2.Token{}

	// Simulate the proxy running and acquiring tokens upon expiry
	for {
		log.Info("Awake",
			"now", time.Now(),
		)

		// Get Token (if any) from map
		tok, ok := tokens[audience]
		log.Info("Token cached?",
			"ok", ok,
		)

		// If there's a token but it's expired, then not ok
		if ok {
			if tok.Expiry.Before(time.Now()) {
				ok = false
			}
		}
		if !ok {
			log.Info("Refreshing Token")
			// Cache retrieved Token in map
			tok, err = ts.Token()
			if err != nil {
				t.Fatalf("Expected new Token; got: %s", err)
			}

			// Cache Token
			tokens[audience] = tok

			log.Info("Token",
				"expiry", tok.Expiry,
			)

		}

		// Simulate handler being called periodically
		log.Info("Sleeping",
			"minutes", delay.Minutes(),
		)
		time.Sleep(delay)
	}
}
