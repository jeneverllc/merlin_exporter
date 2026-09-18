package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version   = "0.0.0"
	revision  = "unknown"
	branch    = "unknown"
	buildUser = "unknown"
	buildDate = "unknown"
)

// Exporter represents an instance of the Merlin cable modem exporter.
type Exporter struct {
	api *MerlinAPI

	// Exporter metrics.
	totalScrapes prometheus.Counter
	scrapeErrors prometheus.Counter
}

// NewExporter returns an instance of Exporter configured with the Merlin portal URL.
func NewExporter(baseURL string) *Exporter {
	return &Exporter{
		api: NewMerlinExporter(baseURL),

		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "merlin",
			Name:      "status_scrapes_total",
			Help:      "Total number of scrapes of the Merlin portal.",
		}),
		scrapeErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "merlin",
			Name:      "status_scrape_errors_total",
			Help:      "Total number of failed scrapes of the Merlin portal.",
		}),
	}
}

// Describe returns Prometheus metric descriptions for the exporter metrics.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.totalScrapes.Desc()
	ch <- e.scrapeErrors.Desc()
	// Pass through the MerlinAPI descriptions.
	e.api.Describe(ch)
}

// Collect runs our scrape returning each Prometheus metric.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.totalScrapes.Inc()

	// Collect the MerlinAPI metrics directly.
	e.api.Collect(ch)
}

func main() {
	var (
		configFile  = flag.String("config.file", "merlin_exporter.yml", "Path to configuration file.")
		showVersion = flag.Bool("version", false, "Print version information.")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("merlin_exporter version=%s revision=%s branch=%s buildUser=%s buildDate=%s\n",
			version, revision, branch, buildUser, buildDate)
		os.Exit(0)
	}

	config, err := NewConfigFromFile(*configFile)
	if err != nil {
		log.Fatal(err)
	}

	exporter := NewExporter(config.Merlin.URL)
	prometheus.MustRegister(exporter)

	http.Handle(config.Telemetry.MetricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, config.Telemetry.MetricsPath, http.StatusMovedPermanently)
	})

	log.Printf("merlin_exporter listening on %s", config.Telemetry.ListenAddress)
	if err := http.ListenAndServe(config.Telemetry.ListenAddress, nil); err != nil {
		log.Fatalf("failed to start merlin exporter: %s", err)
	}
}
