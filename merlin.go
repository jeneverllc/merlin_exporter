package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MerlinAPI represents the RCN/Astound Merlin portal.
type MerlinAPI struct {
	BaseURL string

	client *http.Client
	mu     sync.Mutex

	// Cache discovered modem info.
	modemIP  string
	modemMAC string

	// Exporter metrics.
	totalScrapes prometheus.Counter
	scrapeErrors prometheus.Counter

	// Downstream metrics.
	dsChannelPower   *prometheus.Desc
	dsChannelSNR     *prometheus.Desc
	dsChannelFreq    *prometheus.Desc
	dsChannelCorrect *prometheus.Desc
	dsChannelUncorr  *prometheus.Desc

	// Upstream metrics.
	usChannelPower   *prometheus.Desc
	usChannelSNR     *prometheus.Desc
	usChannelFreq    *prometheus.Desc
	usChannelCorrect *prometheus.Desc
	usChannelUncorr  *prometheus.Desc

	// Modem info.
	modemInfoDesc *prometheus.Desc
}

// NewMerlinExporter creates a new Prometheus exporter for the Merlin portal.
func NewMerlinExporter(baseURL string) *MerlinAPI {
	dsLabelNames := []string{"frequency"}
	usLabelNames := []string{"frequency"}

	return &MerlinAPI{
		BaseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},

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

		dsChannelPower: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "downstream_channel", "power_dbmv"),
			"Downstream channel power in dBmV.",
			dsLabelNames, nil,
		),
		dsChannelSNR: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "downstream_channel", "snr_db"),
			"Downstream channel signal-to-noise ratio in dB.",
			dsLabelNames, nil,
		),
		dsChannelFreq: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "downstream_channel", "frequency_mhz"),
			"Downstream channel frequency in MHz.",
			dsLabelNames, nil,
		),
		dsChannelCorrect: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "downstream_channel", "correctable_errors_total"),
			"Downstream channel correctable error count.",
			dsLabelNames, nil,
		),
		dsChannelUncorr: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "downstream_channel", "uncorrectable_errors_total"),
			"Downstream channel uncorrectable error count.",
			dsLabelNames, nil,
		),

		usChannelPower: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "upstream_channel", "power_dbmv"),
			"Upstream channel power in dBmV.",
			usLabelNames, nil,
		),
		usChannelFreq: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "upstream_channel", "frequency_mhz"),
			"Upstream channel frequency in MHz.",
			usLabelNames, nil,
		),
		usChannelSNR: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "upstream_channel", "snr_db"),
			"Upstream channel signal-to-noise ratio in dB.",
			usLabelNames, nil,
		),
		usChannelCorrect: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "upstream_channel", "correctable_errors_total"),
			"Upstream channel correctable error count.",
			usLabelNames, nil,
		),
		usChannelUncorr: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "upstream_channel", "uncorrectable_errors_total"),
			"Upstream channel uncorrectable error count.",
			usLabelNames, nil,
		),

		modemInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName("merlin", "modem", "info"),
			"Static information about the modem (model, firmware version).",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (m *MerlinAPI) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.totalScrapes.Desc()
	ch <- m.scrapeErrors.Desc()
	ch <- m.dsChannelPower
	ch <- m.dsChannelSNR
	ch <- m.dsChannelFreq
	ch <- m.dsChannelCorrect
	ch <- m.dsChannelUncorr
	ch <- m.usChannelPower
	ch <- m.usChannelSNR
	ch <- m.usChannelFreq
	ch <- m.usChannelCorrect
	ch <- m.usChannelUncorr
	ch <- m.modemInfoDesc
}

// Collect implements prometheus.Collector.
func (m *MerlinAPI) Collect(ch chan<- prometheus.Metric) {
	m.totalScrapes.Inc()
	m.mu.Lock()

	err := m.scrapeAndUpdate(ch)
	if err != nil {
		log.Printf("merlin: scrape error: %v", err)
		m.scrapeErrors.Inc()
	}

	m.mu.Unlock()
}

func (m *MerlinAPI) scrapeAndUpdate(ch chan<- prometheus.Metric) error {
	// Step 1: Discover modem IP via lookup endpoint.
	modemIP, mac, err := m.discoverModem()
	if err != nil {
		return fmt.Errorf("discover modem: %w", err)
	}

	if m.modemIP != modemIP || m.modemMAC != mac {
		log.Printf("merlin: discovered modem IP=%s MAC=%s", modemIP, mac)
		m.modemIP = modemIP
		m.modemMAC = mac
	}

	// Step 2: Fetch downstream data.
	downStreams, err := m.fetchDownstreams(modemIP)
	if err != nil {
		log.Printf("merlin: downstream fetch warning: %v", err)
	} else {
		m.emitDownstreamMetrics(ch, downStreams)
	}

	// Step 3: Fetch upstream data.
	upStreams, err := m.fetchUpstreams(modemIP, m.modemMAC)
	if err != nil {
		log.Printf("merlin: upstream fetch warning: %v", err)
	} else {
		m.emitUpstreamMetrics(ch, upStreams)
	}

	return nil
}

// discoverModem queries the Merlin lookup endpoint to find the modem's IP.
func (m *MerlinAPI) discoverModem() (string, string, error) {
	// strip /merlin/ from the baseURL if present, since the lookup endpoint is at the root.
	url := m.BaseURL
	if strings.HasSuffix(m.BaseURL, "/merlin") {
		url = strings.TrimSuffix(url, "/merlin")
	}
	url = url + "/lookup_mip_merlin-new.cgi"

	resp, err := m.client.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read body from %s: %w", url, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("parse JSON from %s: %w", url, err)
	}

	ip, _ := result["modemIP"].(string)
	mac, _ := result["modem"].(string)

	if ip == "" {
		return "", "", fmt.Errorf("no modemIP returned from %s", url)
	}

	return ip, mac, nil
}

// downstreamData represents a single downstream channel row from the API.
type downstreamData map[string]interface{}

// fetchDownstreams queries the Merlin RF modem status endpoint.
func (m *MerlinAPI) fetchDownstreams(modemIP string) ([]downstreamData, error) {
	url := fmt.Sprintf("%s/rfmodem_ds.cgi?ip=%s", m.BaseURL, modemIP)
	// log.Println("merlin: fetching downstream data from", url)

	resp, err := m.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body from %s: %w", url, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse JSON from %s: %w", url, err)
	}

	// The API returns DownStreams as an array of objects.
	dsRaw, ok := result["DownStreams"]
	if !ok {
		return nil, fmt.Errorf("no DownStreams key in response from %s", url)
	}

	dsArr, ok := dsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("DownStreams is not an array in response from %s", url)
	}

	downStreams := make([]downstreamData, 0, len(dsArr))
	for _, item := range dsArr {
		if obj, ok := item.(map[string]interface{}); ok {
			downStreams = append(downStreams, obj)
		}
	}

	return downStreams, nil
}

// upstreamData represents a single upstream channel row from the API.
type upstreamData map[string]interface{}

// fetchUpstreams queries the Merlin RF modem upstream endpoint.
func (m *MerlinAPI) fetchUpstreams(modemIP, mac string) ([]upstreamData, error) {
	url := fmt.Sprintf("%s/rfmodem_us_Ver2.cgi?ip=%s&modem=%s", m.BaseURL, modemIP, mac)
	// log.Println("merlin: fetching upstream data from", url)

	resp, err := m.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body from %s: %w", url, err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse JSON from %s: %w", url, err)
	}

	// The API returns upstreamData as an array of objects.
	usRaw, ok := result["upstreamData"]
	if !ok {
		return nil, fmt.Errorf("no upstreamData key in response from %s", url)
	}

	usArr, ok := usRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("upstreamData is not an array in response from %s", url)
	}

	upStreams := make([]upstreamData, 0, len(usArr))
	for _, item := range usArr {
		if obj, ok := item.(map[string]interface{}); ok {
			upStreams = append(upStreams, obj)
		}
	}

	return upStreams, nil
}

// safeFloat64 extracts a float64 value from an interface{} value.
// Handles string numeric values (common in JSON from these APIs).
func safeFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

// safeString extracts a string value from an interface{} value.
func safeString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.2f", val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// emitDownstreamMetrics converts downstream channel data to Prometheus metrics.
func (m *MerlinAPI) emitDownstreamMetrics(ch chan<- prometheus.Metric, streams []downstreamData) {
	for _, row := range streams {
		freqStr := safeString(row["Channel Frequency"])
		freq := freqStr // default to raw string if parsing fails
		freqVal, err := extractNumericValue(freqStr)
		if err != nil {
			log.Printf("Error extracting numeric value from frequency '%s': %v", freqStr, err)
		} else {
			freq = fmt.Sprintf("%.2f", freqVal)
		}

		labels := []string{freq}

		powerStr := safeString(row["DownStream Pwr"])
		powerVal, err := unescapeAndExtractNumericValue(powerStr)
		if err != nil {
			log.Fatal("Error extracting numeric value:", err)
		} else {
			ch <- prometheus.MustNewConstMetric(
				m.dsChannelPower, prometheus.GaugeValue, powerVal, labels...,
			)

		}

		snrStr := safeString(row["DownStream SNR"])
		snrVal, err := unescapeAndExtractNumericValue(snrStr)
		if err != nil {
			log.Fatal("Error extracting numeric value:", err)
		} else {
			ch <- prometheus.MustNewConstMetric(
				m.dsChannelSNR, prometheus.GaugeValue, snrVal, labels...,
			)
		}

		correctable := safeFloat64(row["zCorr"])
		if correctable == 0 {
			correctable = safeFloat64(row["zCorr"])
		}
		ch <- prometheus.MustNewConstMetric(
			m.dsChannelCorrect, prometheus.CounterValue, correctable, labels...,
		)

		uncorrectable := safeFloat64(row["Uncorr"])
		if uncorrectable == 0 {
			uncorrectable = safeFloat64(row["Uncorr"])
		}
		ch <- prometheus.MustNewConstMetric(
			m.dsChannelUncorr, prometheus.CounterValue, uncorrectable, labels...,
		)
	}
}

// Sample value "\u003Cspan class=inspec\u003E2.7 dBmV\u003C/span\u003E"
func unescapeAndExtractNumericValue(str string) (float64, error) {
	// First, convert the UTF-8 escaped string to a normal string, then extract the numeric part.
	// Wrap in double quotes so Unquote recognizes it as a quoted string literal
	unquoted, err := strconv.Unquote(`"` + str + `"`)
	if err != nil {
		return 0.0, err
	}

	return extractHtmlNumericValue(unquoted)
}

func extractHtmlNumericValue(str string) (float64, error) {
	// Matches digits, optionally followed by a dot and more digits, between > and <
	re := regexp.MustCompile(`>(-?[0-9]+(?:\.[0-9]+)?)[^<]*<`)
	matches := re.FindStringSubmatch(str)

	if len(matches) < 2 {
		return 0, fmt.Errorf("no numeric value found between tags in: %s", str)
	}

	// Convert extracted string match to float64
	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse float: %w", err)
	}

	return val, nil
}

func extractNumericValue(str string) (float64, error) {
	// Matches digits, optionally followed by a dot and more digits, between > and <
	re := regexp.MustCompile(`(-?[0-9]+(?:\.[0-9]+)?).*`)
	matches := re.FindStringSubmatch(str)

	if len(matches) < 2 {
		return 0, fmt.Errorf("no numeric value found between tags in: %s", str)
	}

	// Convert extracted string match to float64
	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse float: %w", err)
	}

	return val, nil
}

// emitUpstreamMetrics converts upstream channel data to Prometheus metrics.
func (m *MerlinAPI) emitUpstreamMetrics(ch chan<- prometheus.Metric, streams []upstreamData) {
	for _, row := range streams {
		freqStr := safeString(row["Channel Frequency"])
		freq := freqStr // default to raw string if parsing fails
		freqVal, err := extractNumericValue(freqStr)
		if err != nil {
			log.Printf("Error extracting numeric value from frequency '%s': %v", freqStr, err)
		} else {
			freq = fmt.Sprintf("%.2f", freqVal)
		}

		labels := []string{freq}

		powerStr := safeString(row["Upstream Pwr"])
		powerVal, err := unescapeAndExtractNumericValue(powerStr)
		if err != nil {
			log.Fatal("Error extracting numeric value:", err)
		} else {
			ch <- prometheus.MustNewConstMetric(
				m.usChannelPower, prometheus.GaugeValue, powerVal, labels...,
			)

		}

		snrStr := safeString(row["SNR"])
		snrVal, err := unescapeAndExtractNumericValue(snrStr)
		if err != nil {
			log.Fatal("Error extracting numeric value:", err)
		} else {
			ch <- prometheus.MustNewConstMetric(
				m.usChannelSNR, prometheus.GaugeValue, snrVal, labels...,
			)
		}

		correctable := safeFloat64(row["zCorr"])
		if correctable == 0 {
			correctable = safeFloat64(row["zCorr"])
		}
		ch <- prometheus.MustNewConstMetric(
			m.usChannelCorrect, prometheus.CounterValue, correctable, labels...,
		)

		uncorrectable := safeFloat64(row["yUncorr"])
		if uncorrectable == 0 {
			uncorrectable = safeFloat64(row["yUncorr"])
		}
		ch <- prometheus.MustNewConstMetric(
			m.usChannelUncorr, prometheus.CounterValue, uncorrectable, labels...,
		)
	}
}
