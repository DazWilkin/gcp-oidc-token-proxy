package main

import (
	"flag"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

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

func init() {
	log = stdr.NewWithOptions(stdlog.New(os.Stderr, "", stdlog.LstdFlags), stdr.Options{LogCaller: stdr.All})
}
func handler(w http.ResponseWriter, r *http.Request) {
	log.Info("Request",
		"Host", r.URL.Host,
		"Path", r.URL.Path,
		"Query", r.URL.RawQuery,
	)
}
func metrics(port uint16) {
	log := log.WithValues("func", "metrics")

	endpoint := fmt.Sprintf("0.0.0.0:%d", port)
	log = log.WithValues("endpoint", endpoint)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	s := &http.Server{
		Addr:    endpoint,
		Handler: mux,
	}
	listen, err := net.Listen("tcp", endpoint)
	if err != nil {
		log.Error(err, "unable to create listener",
			"endpoint", endpoint,
		)
	}
	log.Info("Starting Prometheus metrics exporter",
		"endpoint", fmt.Sprintf("%s/metrics", endpoint),
	)
	log.Error(s.Serve(listen), "unable to serve")
}
func main() {
	http.HandleFunc("/", handler)
	log.Error(http.ListenAndServe(
		fmt.Sprintf(":%d", *port),
		nil,
	), "failed to start server")
}
