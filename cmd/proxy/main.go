package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"google.golang.org/grpc/credentials/oauth"
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
	creds  oauth.TokenSource
	ts     oauth2.TokenSource
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
	// DefaultTokenSource uses GOOGLE_APPLICATION_CREDENTIALS
	var err error
	ts, err = google.DefaultTokenSource(context.Background(), scopeCloudPlatform)
	// aud := "https://ackal-healthcheck-server-2eynp5ydga-wl.a.run.app"
	// aud := "https://oauth2.googleapis.com/token"
	// ts, err = idtoken.NewTokenSource(context.Background(), aud)
	if err != nil {
		log.Error(err, "Unable to get default token source")
		os.Exit(1)
	}

	creds = oauth.TokenSource{ts}
}

// Client is a type that is not yet used
type Client struct {
	client http.Client
	creds  oauth.TokenSource
}

// Request is a type that corresponds to Google's Token Service Request
// https://developers.google.com/identity/toolkit/reference/securetoken/rest/v1/token
type Request struct {
	GrantType    string `json:"grant_type" description:"The type of token being sent"`
	Code         string `json:"code" description:"Identity token to exchange for an access token"`
	RefreshToken string `json:"refresh_token" description:"Refresh token to exchange for an access token"`
}

// Response is a type that corresponds to Google's Token Service Response
// Values is a method that converts a TokenRequest into URL-encoded values
type Response struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    string `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
}

func (t *Request) Values() url.Values {
	data := url.Values{}
	data.Set("grant_type", t.GrantType)

	switch t.GrantType {
	case "authorization_code":
		data.Set("code", t.Code)
	case "refresh_token":
		data.Set("refresh_token", t.RefreshToken)
	}

	return data
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

	// tok, err := creds.Token()
	tok, err := ts.Token()
	if err != nil {
		log.Error(err, "Unable to get token from token source")
		os.Exit(1)
	}

	log.Info("Token",
		"token", tok,
	)

	// rqst := Request{
	// 	GrantType: "authorization_code",
	// 	Code:      access_token,
	// }

	// client := *http.DefaultClient

	// resp, err := client.PostForm(fmt.Sprintf("%s?key=%s", method, apiKey), rqst.Values())
	// if err != nil {
	// 	log.Error(err, "Unable to POST against Google Secure Token endpoint")
	// 	return
	// }

	// resp := struct {
	// 	AccessToken  string  `json:"access_token"`
	// 	ExpiresIn    float64 `json:"expires_in"`
	// 	RefreshToken string  `json:"refresh_token"`
	// 	Scope        string  `json:"scope"`
	// 	TokenType    string  `json:"token_type"`
	// }{
	// 	AccessToken: strings.TrimRight(tok.AccessToken, "."),
	// 	ExpiresIn:   time.Until(tok.Expiry).Seconds(),
	// 	TokenType:   tok.TokenType,
	// 	Scope:       "https://www.googleapis.com/auth/cloud-platform",
	// }

	j, err := json.Marshal(tok)
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
