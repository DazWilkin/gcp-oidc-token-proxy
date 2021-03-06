package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"golang.org/x/oauth2"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"google.golang.org/api/idtoken"
)

const (
	namespace string = "gcp"
	subsystem string = "oidc_token_proxy"
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
	port = flag.Uint("port", 7777, "The endpoint of the proxy's HTTP server")
)

var (
	counterBuildTime = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "build_info",
			Namespace: namespace,
			Subsystem: subsystem,
			Help:      "A metric with a constant '1' value labels by build time, git commit, OS and Go versions",
		}, []string{
			"build_time",
			"git_commit",
			"os_version",
			"go_version",
		},
	)
	tokensourcesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "tokensources_total",
			Namespace: namespace,
			Subsystem: subsystem,
			Help:      "The total number of TokenSources created",
		}, []string{
			"audience",
		},
	)
	tokensourcesError = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "tokensources_error",
			Namespace: namespace,
			Subsystem: subsystem,
			Help:      "The total number of TokenSources failed",
		}, []string{
			"audience",
		},
	)
	tokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "tokens_total",
			Namespace: namespace,
			Subsystem: subsystem,
			Help:      "The total number of Tokens created",
		}, []string{
			"audience",
		},
	)
	tokensError = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "tokens_error",
			Namespace: namespace,
			Subsystem: subsystem,
			Help:      "The total number of Tokens failed",
		}, []string{
			"audience",
		},
	)
)

var (
	log logr.Logger
)
var (
	tokenSources = map[string]oauth2.TokenSource{}
	tokens       = map[string]*oauth2.Token{}
)

func handler(w http.ResponseWriter, r *http.Request) {
	log := log.WithName("handler")

	log.Info("Request",
		"Host", r.URL.Host,
		"Path", r.URL.Path,
		"Query", r.URL.RawQuery,
	)

	// Expect POST
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Expect Content-Type
	if r.Header.Get("content-type") != "application/x-www-form-urlencoded" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	r.ParseForm()

	// Expect FORM property: audience
	audiences, ok := r.PostForm["audience"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(audiences) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	audience := audiences[0]
	log = log.WithValues("audience", audience)

	// Is a TokenSource for this audience cached?
	ts, ok := tokenSources[audience]
	if !ok {
		// Initialize TokenSource using Application Default Credentials
		log.Info("Creating and caching TokenSource")
		tokensourcesTotal.With(prometheus.Labels{
			"audience": audience,
		}).Inc()
		var err error
		ts, err = idtoken.NewTokenSource(context.Background(), audience)
		if err != nil {
			log.Error(err, "Unable to get default TokenSource")
			tokensourcesError.With(prometheus.Labels{
				"audience": audience,
			}).Inc()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Cache TokenSource
		tokenSources[audience] = ts
	}

	// Is a token for this audience cached?
	tok, ok := tokens[audience]

	// if ok but the token has expired, then it's really not ok
	if ok {
		if tok.Expiry.Before(time.Now()) {
			ok = false
		}
	}

	// If not ok, get a new Token
	if !ok {
		log.Info("Creating and caching Token")
		tokensTotal.With(prometheus.Labels{"audience": audience}).Inc()
		var err error
		tok, err = ts.Token()
		if err != nil {
			log.Error(err, "Unable create Token")
			tokensError.With(prometheus.Labels{"audience": audience}).Inc()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Cache Token
		tokens[audience] = tok
	}

	// The response isn't quite oauth2.Token
	// https://pkg.go.dev/golang.org/x/oauth2#Token
	// Cloud Run accepts e.g. --header="Authorization: Bearer $(gcloud auth print-identity-token)"
	log.Info("Creating response")
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

	log.Info("Marshaling response")
	j, err := json.Marshal(resp)
	if err != nil {
		log.Error(err, "Unable to marshal JSON response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Done
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(j))
	log.Info("Done")
}

func main() {
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
	log := log.WithName("main")

	flag.Parse()

	// Create Prometheus 'static' counter for build config
	log.Info("Build config",
		"build_time", BuildTime,
		"git_commit", GitCommit,
		"os_version", OSVersion,
		"go_version", GoVersion,
	)
	counterBuildTime.With(prometheus.Labels{
		"build_time": BuildTime,
		"git_commit": GitCommit,
		"os_version": OSVersion,
		"go_version": GoVersion,
	}).Inc()

	log.Info("Configuring handlers")
	http.HandleFunc("/", handler)
	http.Handle("/metrics", promhttp.Handler())

	// Avoid serving /favico.ico
	// Doing so triggers the root handler and this results in tokens being minted
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {})

	log.Info("Starting OIDC Token Proxy",
		"port", *port,
	)
	log.Error(http.ListenAndServe(
		fmt.Sprintf(":%d", *port),
		nil,
	), "Failed to start OIDC Token Proxy")
}
