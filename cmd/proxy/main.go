package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/brabantcourt/ackal-cli/google/token_service"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
		log.Error(nil, "Unable to find `API_KEY` in the environment")
		os.Exit(1)
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

	// decoder := json.NewDecoder(r.Body)
	// var j interface{}
	// err := decoder.Decode(&j)
	// if err != nil {
	// 	log.Error(err, "unable to decode body",
	// 		"Body", r.Body,
	// 	)
	// }
	// s, err := json.Marshal(j)
	// if err != nil {
	// 	fmt.Fprint(w, err)
	// }
	// fmt.Fprintf(w, "%s", s)

	client, err := token_service.NewClient(apiKey, log)
	if err != nil {
		log.Error(err, "Unable to connect to Token Service")
		fmt.Fprint(w, err)
		return
	}
	resp, err := client.Token("XXX")
	if err != nil {
		log.Error(err, "Error retrieving token from Token Service")
		fmt.Fprint(w, err)
		return
	}
	j, err := json.Marshal(resp)
	if err != nil {
		log.Error(err, "Unable to marshal JSON response")
		fmt.Fprint(w, err)
		return
	}
	fmt.Fprint(w, j)
}
func main() {
	http.HandleFunc("/", handler)
	http.Handle("/metrics", promhttp.Handler())

	log.Error(http.ListenAndServe(
		fmt.Sprintf(":%d", *port),
		nil,
	), "failed to start server")
}
