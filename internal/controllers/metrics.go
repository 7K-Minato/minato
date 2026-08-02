package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

// Operator metrics implementing docs/operations/metrics-schema.md. They are
// registered with controller-runtime's global registry and therefore exposed
// on the manager's metrics endpoint (--metrics-bind-address, :8080 in the
// Helm deployment) alongside the default controller-runtime metrics.
//
// Design: explicit gauge management instead of a custom prometheus.Collector
// that lists GameServers on scrape. The reconciler observes every transition
// (creates, status updates, and — via the finalizer — deletes), so setting
// the gauges during reconcile keeps them accurate without coupling metric
// collection to cache reads at scrape time. Stale label combinations are
// removed with DeletePartialMatch keyed on namespace+server, which is
// race-safe because the workqueue serializes reconciles for a given key.
// This is also directly testable with prometheus testutil against the vecs.
var (
	gameServersGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "minato_gameservers",
		Help: "Number of GameServers by state. One series per GameServer (value is always 1).",
	}, []string{"namespace", "profile", "server", "state"})

	playersOnlineGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "minato_players_online",
		Help: "Current player count reported in GameServer status.",
	}, []string{"namespace", "server"})

	playerCapacityGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "minato_player_capacity",
		Help: "Player capacity reported in GameServer status.",
	}, []string{"namespace", "server"})

	actionExecutionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "minato_action_executions_total",
		Help: "Total ActionExecutions reaching a terminal state (Succeeded, Failed, TimedOut, Rejected).",
	}, []string{"action", "result"})
)

func init() {
	crmetrics.Registry.MustRegister(
		gameServersGauge,
		playersOnlineGauge,
		playerCapacityGauge,
		actionExecutionsTotal,
	)
}

// recordGameServerMetrics exports the GameServer's current status as gauge
// series. Any previous series for this server (e.g. an older state label) are
// removed first so exactly one minato_gameservers series exists per server.
func recordGameServerMetrics(server *operatorv1.GameServer) {
	state := server.Status.State
	if state == "" {
		state = stateProvisioning
	}
	deleteGameServerMetrics(server.Namespace, server.Name)
	gameServersGauge.WithLabelValues(server.Namespace, server.Spec.Profile, server.Name, state).Set(1)
	playersOnlineGauge.WithLabelValues(server.Namespace, server.Name).Set(float64(server.Status.Players))
	playerCapacityGauge.WithLabelValues(server.Namespace, server.Name).Set(float64(server.Status.PlayerCapacity))
}

// deleteGameServerMetrics removes all series for a GameServer. Called from
// the finalizer path so deleted servers do not leave stale series behind.
func deleteGameServerMetrics(namespace, name string) {
	gameServersGauge.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "server": name})
	playersOnlineGauge.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "server": name})
	playerCapacityGauge.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "server": name})
}

// observeActionExecution counts a terminal ActionExecution. result is the
// terminal state (Succeeded, Failed, TimedOut, Rejected).
func observeActionExecution(action, result string) {
	actionExecutionsTotal.WithLabelValues(action, result).Inc()
}
