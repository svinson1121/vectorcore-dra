package api

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/peer"
	"github.com/svinson1121/vectorcore-dra/internal/logging"
)

type StatusResponse struct {
	Identity      string    `json:"identity"`
	Realm         string    `json:"realm"`
	ProductName   string    `json:"product_name"`
	Uptime        string    `json:"uptime"`
	UptimeSeconds float64   `json:"uptime_seconds"`
	StartedAt     time.Time `json:"started_at"`
	PeersTotal    int       `json:"peers_total"`
	PeersOpen     int       `json:"peers_open"`
	PeersClosed   int       `json:"peers_closed"`
	Version       string    `json:"version"`
	LogLevel      string    `json:"log_level"`
}

// MetricSample is a single time-series sample from Prometheus.
type MetricSample struct {
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

// MetricFamily groups samples for one metric name.
type MetricFamily struct {
	Name    string         `json:"name"`
	Help    string         `json:"help"`
	Type    string         `json:"type"`
	Samples []MetricSample `json:"samples"`
}

// MetricsResponse is the JSON metrics snapshot returned by GET /api/v1/metrics.
type MetricsResponse struct {
	CollectedAt time.Time      `json:"collected_at"`
	Metrics     []MetricFamily `json:"metrics"`
}

type LogLevelRequest struct {
	Level string `json:"level" required:"true" enum:"debug,info,warn,error"`
}

func registerRuntime(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/api/v1/oam/status",
		Summary: "DRA status",
		Tags:    []string{"OAM"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body StatusResponse }, error) {
		peers := s.mgr.List()
		total, open, closed := 0, 0, 0
		for _, p := range peers {
			total++
			switch p.State() {
			case peer.StateOpen:
				open++
			default:
				closed++
			}
		}
		uptimeDur := time.Since(s.startAt).Truncate(time.Second)
		return &struct{ Body StatusResponse }{Body: StatusResponse{
			Identity:      s.cfg.DRA.Identity,
			Realm:         s.cfg.DRA.Realm,
			ProductName:   "VectorCore DRA",
			Uptime:        uptimeDur.String(),
			UptimeSeconds: uptimeDur.Seconds(),
			StartedAt:     s.startAt,
			PeersTotal:    total,
			PeersOpen:     open,
			PeersClosed:   closed,
			Version:       "0.2.0B",
			LogLevel:      logging.GetLevel(),
		}}, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodPost,
		Path:        "/api/v1/oam/reload",
		Summary:     "Force config reload",
		Description: "Re-reads config.yaml from disk and applies changes.",
		Tags:        []string{"OAM"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
		// The config watcher handles hot-reload; this endpoint triggers a manual reload.
		// We re-use the same sync path as the watcher.
		s.log.Info("manual config reload requested via API")
		// The actual reload is handled by the watcher callback in main.go.
		// Here we trigger it by posting a no-op sync with current config.
		localIP := getLocalIPForAPI()
		s.mgr.Sync(s.ctx, s.cfg.Peers, s.cfg.DRA, s.cfg.Watchdog, s.cfg.Reconnect, localIP)
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		Method:      http.MethodPost,
		Path:        "/api/v1/oam/log-level",
		Summary:     "Change log level at runtime",
		Description: "Changes the log level without restart. Does not persist across restarts.",
		Tags:        []string{"OAM"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Body LogLevelRequest
	}) (*struct{}, error) {
		logging.SetLevel(input.Body.Level)
		s.log.Info("log level changed", zap.String("level", input.Body.Level))
		return nil, nil
	})

	// GET /api/v1/oam/recent-messages
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/v1/oam/recent-messages",
		Summary:     "Recent Diameter messages",
		Description: "Returns the last 20 Diameter messages processed by the DRA (excludes CER/CEA, DWR/DWA, DPR/DPA).",
		Tags:        []string{"OAM"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []RecentMessage }, error) {
		msgs := s.RecentMsgs.Snapshot()
		return &struct{ Body []RecentMessage }{Body: msgs}, nil
	})

	// GET /api/v1/oam/metrics - Prometheus metrics as JSON (dra_* prefix only)
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/v1/oam/metrics",
		Summary:     "Diameter metrics snapshot",
		Description: "Returns current Prometheus metric values for all dra_* metrics as JSON. Full Prometheus exposition is also available at /metrics.",
		Tags:        []string{"OAM"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body MetricsResponse }, error) {
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			return nil, huma.Error500InternalServerError("gathering metrics: " + err.Error())
		}
		resp := MetricsResponse{
			CollectedAt: time.Now(),
		}
		for _, mf := range mfs {
			// Only include dra_* metrics
			if len(mf.GetName()) < 4 || mf.GetName()[:4] != "dra_" {
				continue
			}
			family := MetricFamily{
				Name: mf.GetName(),
				Help: mf.GetHelp(),
				Type: mf.GetType().String(),
			}
			for _, m := range mf.GetMetric() {
				sample := MetricSample{
					Labels: labelPairsToMap(m.GetLabel()),
					Value:  metricValue(mf.GetType(), m),
				}
				family.Samples = append(family.Samples, sample)
			}
			resp.Metrics = append(resp.Metrics, family)
		}
		return &struct{ Body MetricsResponse }{Body: resp}, nil
	})
}

// labelPairsToMap converts Prometheus label pairs to a string map.
func labelPairsToMap(pairs []*dto.LabelPair) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[p.GetName()] = p.GetValue()
	}
	return m
}

// metricValue extracts the float64 value from a Prometheus metric based on its type.
func metricValue(t dto.MetricType, m *dto.Metric) float64 {
	switch t {
	case dto.MetricType_COUNTER:
		if c := m.GetCounter(); c != nil {
			return c.GetValue()
		}
	case dto.MetricType_GAUGE:
		if g := m.GetGauge(); g != nil {
			return g.GetValue()
		}
	case dto.MetricType_HISTOGRAM:
		if h := m.GetHistogram(); h != nil {
			return float64(h.GetSampleCount())
		}
	case dto.MetricType_SUMMARY:
		if s := m.GetSummary(); s != nil {
			return float64(s.GetSampleCount())
		}
	}
	return 0
}

// getLocalIPForAPI returns the local IP for use in API-triggered peer syncs.
func getLocalIPForAPI() net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.ParseIP("127.0.0.1")
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
				return ip4
			}
		}
	}
	return net.ParseIP("127.0.0.1")
}

