package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All Prometheus metrics for VectorCore DRA.
// Access via the package-level variables after calling Init().

var (
	// --- Message counters ---

	// MessagesTotal counts all Diameter messages.
	// Labels: peer, direction (in|out), app_id, command_code, result
	MessagesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_messages_total",
		Help: "Total Diameter messages processed.",
	}, []string{"peer", "direction", "app_id", "command_code", "result"})

	// --- Peer state ---

	// PeerState is a gauge: 0=closed, 1=connecting, 2=open, 3=draining
	PeerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dra_peer_state",
		Help: "Current FSM state of each Diameter peer (0=closed,1=connecting,2=open,3=draining).",
	}, []string{"peer", "transport", "mode"})

	// --- Peer lifecycle ---

	PeerConnectsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_peer_connects_total",
		Help: "Total successful Diameter peer connections (CER/CEA complete).",
	}, []string{"peer", "mode"})

	PeerDisconnectsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_peer_disconnects_total",
		Help: "Total Diameter peer disconnections.",
		// reason: dpr_sent | dpr_received | watchdog_failure | error | admin
	}, []string{"peer", "reason"})

	// --- Routing ---

	RouteHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_route_hits_total",
		Help: "Total route rule matches.",
	}, []string{"peer_group", "app_id"})

	RouteMissesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_route_misses_total",
		Help: "Total routing failures.",
		// reason: no_rule | no_open_peer | loop_detected | rejected | dropped
	}, []string{"reason"})

	IMSIRouteHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_imsi_route_hits_total",
		Help: "Total IMSI prefix routing matches.",
	}, []string{"prefix", "peer_group"})

	ForwardedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_forwarded_total",
		Help: "Total Diameter requests forwarded.",
	}, []string{"from_peer", "to_peer", "command_code"})

	// --- Latency ---

	AnswerLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dra_answer_latency_seconds",
		Help:    "Request-to-answer latency.",
		Buckets: []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.0, 2.5, 5.0},
	}, []string{"peer", "command_code", "app_id"})

	// --- Watchdog ---

	WatchdogSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_watchdog_sent_total",
		Help: "Total DWR messages sent.",
	}, []string{"peer"})

	WatchdogTimeoutTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_watchdog_timeout_total",
		Help: "Total DWR timeouts (no DWA received).",
	}, []string{"peer"})

	// --- Reconnect (active peers) ---

	ReconnectAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_reconnect_attempts_total",
		Help: "Total peer reconnect attempts.",
	}, []string{"peer"})

	ReconnectBackoffSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dra_reconnect_backoff_seconds",
		Help: "Current reconnect backoff duration in seconds.",
	}, []string{"peer"})

	// --- In-flight ---

	InFlightRequests = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dra_inflight_requests",
		Help: "Current number of in-flight Diameter requests per peer.",
	}, []string{"peer"})

	// --- Security ---

	RejectedConnectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_rejected_connections_total",
		Help: "Total inbound connections rejected (source IP not in peer list).",
	}, []string{"source_ip"})

	// --- Transport errors ---

	TransportErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dra_transport_errors_total",
		Help: "Total transport-level errors.",
	}, []string{"peer", "transport", "error_type"})
)
