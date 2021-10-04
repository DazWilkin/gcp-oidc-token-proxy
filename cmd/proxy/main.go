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

const (
	method string = "https://securetoken.googleapis.com/v1/token"
)
const (
	scopeCloudPlatform string = "https://www.googleapis.com/auth/cloud-platform"
)

// GrantType represents the implicit GrantType enum in Google's Token Service Request type
type GrantType int

func (g GrantType) String() string {
	switch g {
	case AuthorizationCode:
		return "authorization_code"
	case RefreshToken:
		return "refresh_token"
	default:
		panic(fmt.Sprintf("Unknown GrantType (%d)", int(g)))
	}
}

const (
	AuthorizationCode = iota
	RefreshToken      = iota
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
	port = flag.Uint("port", 7777, "The endpoint of the HTTP server")
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
	log.Info("Request",
		"Host", r.URL.Host,
		"Path", r.URL.Path,
		"Query", r.URL.RawQuery,
	)
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error(err, "Unable to read body")
	}

	log.Info("Body",
		"Body", string(b),
	)

	// Returns Access Tokens ya29...
	// ts, err := google.DefaultTokenSource(context.Background(), scopeCloudPlatform)

	// Returns ID Token
	// But need to create a dummy response (see handler)
	aud := "https://ackal-healthcheck-server-2eynp5ydga-wl.a.run.app"
	// aud := "https://oauth2.googleapis.com/token"
	ts, err := idtoken.NewTokenSource(context.Background(), aud)
	if err != nil {
		log.Error(err, "Unable to get default token source")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// tok, err := creds.Token()
	tok, err := ts.Token()
	if err != nil {
		log.Error(err, "Unable to get token from token source")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Info("Token",
		"token", tok,
	)

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
		TokenType:    "Bearer",
		Scope:        "https://www.googleapis.com/auth/cloud-platform",
	}

	log.Info("Response",
		"response", resp,
	)

	j, err := json.Marshal(resp)
	if err != nil {
		log.Error(err, "Unable to marshal JSON response")
		fmt.Fprint(w, err)
		return
	}
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
