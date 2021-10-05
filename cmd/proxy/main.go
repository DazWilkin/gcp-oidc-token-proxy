package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"golang.org/x/oauth2"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"google.golang.org/api/idtoken"
)

var (
	// BuildTime is the time that this binary was built represented as a UNIX epoch
	BuildTime string
	// GitCommit is the git commit value and is expected to be set during build
	GitCommit string
	// GoVersion is the Golang runtime version
	GoVersion = runtime.Version()
	// OSVersion is the OS version (uname --kernel-release) and is expected to be set during build
	OSVersion string
	// StartTime is the start time of the exporter represented as a UNIX epoch
	StartTime = time.Now().Unix()
)

var (
	target_url = flag.String("target_url", "", "The URL of the target service")
	port       = flag.Uint("port", 7777, "The endpoint of the proxy's HTTP server")
)
var (
	log logr.Logger
)

func init() {
	log = stdr.NewWithOptions(stdlog.New(os.Stderr, "", stdlog.LstdFlags), stdr.Options{LogCaller: stdr.All})

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		log.Error(nil, "Unable to find GOOGLE_APPLICATION_CREDENTIALS in the environment")
	}
}

// Refactor this
var (
	ts  oauth2.TokenSource
	tok *oauth2.Token
)

func getToken() (err error) {
	tok, err = ts.Token()
	return
}
func handler(w http.ResponseWriter, r *http.Request) {
	log = log.WithName("handler")

	// Debugging: read the request body
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error(err, "Unable to read body")
	}

	// Debugging: log the request's details
	log.Info("Body",
		"Host", r.URL.Host,
		"Path", r.URL.Path,
		"Query", r.URL.RawQuery,
		"Body", string(b),
	)

	// Check whether token has already been initialized
	if tok == nil {
		log.Info("Token being initialized")
		if err := getToken(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Check whether token has expired
	// If just initialized, probably redundant to check its expiry
	if int(time.Until(tok.Expiry).Seconds()) < 5 {
		log.Info("Refreshing Token as nearing expiry or expired")
		if err := getToken(); err != nil {
			log.Error(err, "Unable to get token from token source")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Debugging: log the token
	// TODO(dazwilkin) Don't do this in production
	log.Info("Token",
		"token", tok,
	)

	// The response isn't quite oauth2.Token
	// https://pkg.go.dev/golang.org/x/oauth2#Token
	// Uses expires_in instead of expiry !??
	// By the time of its calculation, it's likely <3600
	// Cloud Run accepts e.g. --header="Authorization: Bearer $(gcloud auth print-identity-token)"
	resp := struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}{
		AccessToken:  tok.AccessToken,
		ExpiresIn:    int(time.Until(tok.Expiry).Seconds()),
		RefreshToken: tok.AccessToken,
		TokenType:    "bearer",
		Scope:        "https://www.googleapis.com/auth/cloud-platform",
	}

	// Debugging: log the response
	log.Info("Response",
		"response", resp,
	)

	j, err := json.Marshal(resp)
	if err != nil {
		log.Error(err, "Unable to marshal JSON response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Done
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(j))
}
func main() {
	flag.Parse()

	if *target_url == "" {
		log.Error(nil, "Flag `target_url` is required as it is used as the audience value when constructing a TokenSource")
		os.Exit(1)
	}

	log = log.WithValues("aud", *target_url)

	// Initialize TokenSource using Application Default Credentials
	var err error
	ts, err = idtoken.NewTokenSource(context.Background(), *target_url)
	if err != nil {
		log.Error(err, "Unable to get default token source")
		os.Exit(1)
	}

	http.HandleFunc("/", handler)
	http.Handle("/metrics", promhttp.Handler())

	// Avoid serving /favico.ico
	// Doing so triggers the root handler and this results in tokens being minted
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {})

	log.Info("Starting token server",
		"port", *port,
	)
	log.Error(http.ListenAndServe(
		fmt.Sprintf(":%d", *port),
		nil,
	), "failed to start token server")
}
