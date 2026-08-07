package controllers

import (
	"time"

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

	reconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "minato_reconcile_duration_seconds",
		Help:    "Duration of controller reconciliations.",
		Buckets: prometheus.DefBuckets,
	}, []string{"controller"})

	reconcileErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "minato_reconcile_errors_total",
		Help: "Total reconciliations that returned an error.",
	}, []string{"controller"})

	provisioningDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "minato_gameserver_provisioning_duration_seconds",
		Help:    "Time from GameServer creation to first transition to the Running state.",
		Buckets: []float64{5, 15, 30, 60, 120, 300, 600, 1200},
	}, []string{"namespace", "profile"})

	fleetReplicasGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "minato_fleet_replicas",
		Help: "GameServerFleet replica counts by kind (desired, ready, updated).",
	}, []string{"namespace", "fleet", "kind"})

	agentUnreachableTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "minato_agent_unreachable_total",
		Help: "Total failed agent health checks (agent unreachable).",
	}, []string{"namespace", "server"})
)

func init() {
	crmetrics.Registry.MustRegister(
		gameServersGauge,
		playersOnlineGauge,
		playerCapacityGauge,
		actionExecutionsTotal,
		reconcileDuration,
		reconcileErrorsTotal,
		provisioningDuration,
		fleetReplicasGauge,
		agentUnreachableTotal,
	)
}

// observeReconcile records the duration of a reconcile and counts errors.
// Intended to be deferred at the top of each controller's Reconcile.
func observeReconcile(controller string, start time.Time, err error) {
	reconcileDuration.WithLabelValues(controller).Observe(time.Since(start).Seconds())
	if err != nil {
		reconcileErrorsTotal.WithLabelValues(controller).Inc()
	}
}

// observeProvisioningDuration records how long a GameServer took to reach
// the Running state, measured from its creation timestamp.
func observeProvisioningDuration(server *operatorv1.GameServer, d time.Duration) {
	provisioningDuration.WithLabelValues(server.Namespace, server.Spec.Profile).Observe(d.Seconds())
}

// recordFleetMetrics exports the fleet's replica counts. desired comes from
// spec; ready/updated from status.
func recordFleetMetrics(fleet *operatorv1.GameServerFleet) {
	fleetReplicasGauge.WithLabelValues(fleet.Namespace, fleet.Name, "desired").Set(float64(fleet.Spec.Replicas))
	fleetReplicasGauge.WithLabelValues(fleet.Namespace, fleet.Name, "ready").Set(float64(fleet.Status.ReadyReplicas))
	fleetReplicasGauge.WithLabelValues(fleet.Namespace, fleet.Name, "updated").Set(float64(fleet.Status.UpdatedReplicas))
}

// deleteFleetMetrics removes all series for a GameServerFleet. Called from
// the finalizer path so deleted fleets do not leave stale series behind.
func deleteFleetMetrics(namespace, name string) {
	fleetReplicasGauge.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "fleet": name})
}

// recordAgentUnreachable counts a failed agent health check for a server.
func recordAgentUnreachable(namespace, server string) {
	agentUnreachableTotal.WithLabelValues(namespace, server).Inc()
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
