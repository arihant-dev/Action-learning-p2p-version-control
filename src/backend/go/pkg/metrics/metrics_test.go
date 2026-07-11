package metrics

import (
	"testing"
)

func TestRegisterPanicsOnDoubleRegister(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on double registration")
		}
	}()
	Register()
	Register()
}

func TestMetricValues(t *testing.T) {
	ConnectedPeers.Set(5)
	ActiveTransfers.Set(3)
	QueueDepth.Set(10)

	IPCMessagesTotal.WithLabelValues("test_type", "in").Inc()
	P2PMessagesTotal.WithLabelValues("test_type").Inc()
	SyncErrorsTotal.WithLabelValues("test_error").Inc()
	DaemonHealthChecks.WithLabelValues("test_repo", "ok").Inc()
	DaemonRestartsTotal.WithLabelValues("test_repo").Inc()

	TransferDuration.Observe(1.5)
}
