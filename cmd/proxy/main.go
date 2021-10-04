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
var (
	apiKey string
)

func init() {
	log = stdr.NewWithOptions(stdlog.New(os.Stderr, "", stdlog.LstdFlags), stdr.Options{LogCaller: stdr.All})

	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		log.Error(nil, "Unable to find API_KEY in the environment")
		os.Exit(1)
	}

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		log.Error(nil, "Unable to find GOOGLE_APPLICATON_CREDENTIALS in the environment")
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	log := log.WithName("handler")

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

	ts, err := idtoken.NewTokenSource(context.Background(), *target_url)
	if err != nil {
		log.Error(err, "Unable to get default token source")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tok, err := ts.Token()
	if err != nil {
		log.Error(err, "Unable to get token from token source")
		w.WriteHeader(http.StatusInternalServerError)
		return
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
	http.HandleFunc("/", handler)
	http.Handle("/metrics", promhttp.Handler())

	log.Error(http.ListenAndServe(
		fmt.Sprintf(":%d", *port),
		nil,
	), "failed to start server")
}
