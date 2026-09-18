# Merlin Exporter

A Prometheus exporter for RCN/Astound "Merlin" cable modem portals. This project polls the Merlin portal's JSON API endpoints to extract upstream and downstream DOCSIS channel metrics and exposes them in Prometheus format for monitoring.

## Supported Devices

This exporter works with any modem accessible via the RCN/Astound Merlin portal infrastructure, including but not limited to:

- Arris DG1670A
- Arris DG2470A  
- Hitron CODA-4589
- CableLabs reference modems
- Any modem behind an RCN/Astound "pa." Merlin endpoint

## How It Works

The Merlin portal is a JavaScript-heavy single-page application that fetches modem data from JSON API endpoints. This exporter queries the same endpoints directly:

| Endpoint | Purpose |
|----------|---------|
| `/lookup_mip_merlin-new.cgi` | Discovers the modem's IP address and MAC from the portal |
| `/merlin/rfmodem_ds.cgi?ip={IP}` | Retrieves downstream channel data (SNR, power, errors) |
| `/merlin/rfmodem_us_Ver2.cgi?ip={IP}` | Retrieves upstream channel data (power, symbol rate, width) |

No authentication is required — the Merlin portal endpoints serve data to any connected client.

## Installation

```bash
# Clone this repository
git clone https://github.com/jeneverllc/merlin_exporter.git
cd merlin_exporter

# Download dependencies and build
go mod tidy
go build -o merlin_exporter .

# Run with default config
./merlin_exporter -config.file=merlin_exporter.yml
```

Alternatively use Go to pull the repository, build and run using

```sh
go install github.com/jeneverllc/merlin_exporter@latest
${GOPATH:-~/go}/bin/merlin_exporter
```

## Usage

```
Usage of ./merlin_exporter:
  -config.file string
        Path to configuration file. (default "merlin_exporter.yml")
  -version
        Print version information.
```

### Example configuration (`merlin_exporter.yml`):

```yaml
merlin:
  url: "https://pa.speedtest.rcn.net/merlin"

telemetry:
  listen_address: ":9527"
  metrics_path: "/metrics"
```

Change the `merlin.url` to match your regional Merlin portal (e.g., `ma.speedtest.rcn.net`, `ny.speedtest.rcn.net`, etc.).

## Prometheus Metrics

### Upstream Channel Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `merlin_upstream_channel_power_dbmv` | Gauge | channel, lock_status, modulation, frequency | Upstream channel power in dBmV |
| `merlin_downstream_channel_snr_db` | Gauge | channel, lock_status, modulation, frequency | Upstream SNR in dB |
| `merlin_downstream_channel_correctable_errors_total` | Counter | channel, lock_status, modulation, frequency | Upstream correctable error count |
| `merlin_downstream_channel_uncorrectable_errors_total` | Counter | channel, lock_status, modulation, frequency | Upstream uncorrectable error count |
| `merlin_upstream_channel_frequency_mhz` | Gauge | channel, lock_status, modulation, frequency | Upstream channel frequency in MHz |

### Downstream Channel Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `merlin_downstream_channel_power_dbmv` | Gauge | channel, lock_status, modulation, frequency | Downstream channel power in dBmV |
| `merlin_downstream_channel_snr_db` | Gauge | channel, lock_status, modulation, frequency | Downstream SNR in dB |
| `merlin_downstream_channel_correctable_errors_total` | Counter | channel, lock_status, modulation, frequency | Downstream correctable error count |
| `merlin_downstream_channel_uncorrectable_errors_total` | Counter | channel, lock_status, modulation, frequency | Downstream uncorrectable error count |
| `merlin_downstream_channel_frequency_mhz` | Gauge | channel, lock_status, modulation, frequency | Downstream channel frequency in MHz |

### Exporter Internal Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `merlin_status_scrapes_total` | Counter | Total successful scrapes |
| `merlin_status_scrape_errors_total` | Counter | Total failed scrapes |

### Example output:

```
# HELP merlin_upstream_channel_power_dbmv Upstream channel power in dBmV.
# TYPE merlin_upstream_channel_power_dbmv gauge
merlin_upstream_channel_power_dbmv{channel="1",lock_status="Locked",modulation="QAM64",frequency="23600000"} 41.5
merlin_upstream_channel_power_dbmv{channel="2",lock_status="Locked",modulation="QAM64",frequency="23900000"} 41.8
# HELP merlin_downstream_channel_snr_db Downstream channel signal-to-noise ratio in dB.
# TYPE merlin_downstream_channel_snr_db gauge
merlin_downstream_channel_snr_db{channel="1",lock_status="Locked",modulation="QAM256",frequency="73400000"} 38.2
```

## Grafana Dashboard

Import the included `merlin_dashboard.json` into your Grafana instance to get a quick visualization of modem channel health, including:

- Downstream/SN power vs. SNR scatter plot
- Upstream power bar chart per channel
- Error rate trends (correctable vs. uncorrectable)
- Channel bonding overview

## License

Apache Public License 2.0
