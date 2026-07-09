package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	ConnectedPeers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_connected_peers",
		Help: "Number of currently connected P2P peers",
	})

	ActiveTransfers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_active_transfers",
		Help: "Number of active file transfers",
	})

	QueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_queue_depth",
		Help: "Current number of pending sync tasks in the queue",
	})

	IPCMessagesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_ipc_messages_total",
		Help: "Total number of IPC messages processed by type",
	}, []string{"type", "direction"})

	P2PMessagesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_network_messages_total",
		Help: "Total number of P2P network messages processed by type",
	}, []string{"type"})

	TransferDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "p2p_transfer_duration_seconds",
		Help:    "Histogram of file transfer durations in seconds",
		Buckets: prometheus.DefBuckets,
	})

	SyncErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_sync_errors_total",
		Help: "Total number of sync errors by type",
	}, []string{"error_type"})

	DaemonHealthChecks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_daemon_health_checks_total",
		Help: "Total number of C++ daemon health checks by result",
	}, []string{"repo_id", "result"})

	DaemonRestartsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "p2p_daemon_restarts_total",
		Help: "Total number of C++ daemon restarts by repo",
	}, []string{"repo_id"})
)

func Register() {
	prometheus.MustRegister(
		ConnectedPeers,
		ActiveTransfers,
		QueueDepth,
		IPCMessagesTotal,
		P2PMessagesTotal,
		TransferDuration,
		SyncErrorsTotal,
		DaemonHealthChecks,
		DaemonRestartsTotal,
	)
}