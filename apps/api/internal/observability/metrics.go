package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// DefaultRegistry is the singleton metrics registry for the SMARTREVERSAL platform.
var DefaultRegistry = NewRegistry()

// Registry manages and renders Prometheus-compatible metrics in-memory.
type Registry struct {
	mu           sync.RWMutex
	httpRequests map[string]*uint64
	txnsIngested map[string]*uint64
	exceptions   map[string]*uint64
	reversals    map[string]*uint64
	cbTrips      map[string]*uint64
	slaBreaches  uint64
	auditEvents  uint64
}

// NewRegistry initializes an empty metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		httpRequests: make(map[string]*uint64),
		txnsIngested: make(map[string]*uint64),
		exceptions:   make(map[string]*uint64),
		reversals:    make(map[string]*uint64),
		cbTrips:      make(map[string]*uint64),
	}
}

func (r *Registry) getOrInitCounter(m map[string]*uint64, key string) *uint64 {
	r.mu.RLock()
	if ptr, ok := m[key]; ok {
		r.mu.RUnlock()
		return ptr
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if ptr, ok := m[key]; ok {
		return ptr
	}
	var val uint64
	m[key] = &val
	return &val
}

// IncHTTPRequest increments the count of processed HTTP requests with status code.
func (r *Registry) IncHTTPRequest(method, path string, status int) {
	key := fmt.Sprintf(`method="%s",path="%s",status="%d"`, method, path, status)
	counter := r.getOrInitCounter(r.httpRequests, key)
	atomic.AddUint64(counter, 1)
}

// IncTransaction increments the transaction ingestion counter for a rail.
func (r *Registry) IncTransaction(rail string) {
	if rail == "" {
		rail = "UNKNOWN"
	}
	key := fmt.Sprintf(`rail="%s"`, rail)
	counter := r.getOrInitCounter(r.txnsIngested, key)
	atomic.AddUint64(counter, 1)
}

// IncException increments the exception detection counter for an exception code.
func (r *Registry) IncException(code string) {
	if code == "" {
		code = "UNKNOWN"
	}
	key := fmt.Sprintf(`code="%s"`, code)
	counter := r.getOrInitCounter(r.exceptions, key)
	atomic.AddUint64(counter, 1)
}

// IncReversal increments the reversal execution counter by final status.
func (r *Registry) IncReversal(status string) {
	if status == "" {
		status = "UNKNOWN"
	}
	key := fmt.Sprintf(`status="%s"`, status)
	counter := r.getOrInitCounter(r.reversals, key)
	atomic.AddUint64(counter, 1)
}

// IncSLABreach increments the regulatory SLA breaches counter.
func (r *Registry) IncSLABreach() {
	atomic.AddUint64(&r.slaBreaches, 1)
}

// IncCircuitBreakerTrip increments the circuit breaker trip counter.
func (r *Registry) IncCircuitBreakerTrip(name string) {
	key := fmt.Sprintf(`name="%s"`, name)
	counter := r.getOrInitCounter(r.cbTrips, key)
	atomic.AddUint64(counter, 1)
}

// IncAuditEvent increments the audit trail append counter.
func (r *Registry) IncAuditEvent() {
	atomic.AddUint64(&r.auditEvents, 1)
}

// Render formats all registered metrics into Prometheus text exposition format.
func (r *Registry) Render() string {
	var sb strings.Builder

	sb.WriteString("# HELP smartreversal_http_requests_total Total HTTP requests processed.\n")
	sb.WriteString("# TYPE smartreversal_http_requests_total counter\n")
	r.renderMap(&sb, "smartreversal_http_requests_total", r.httpRequests)

	sb.WriteString("\n# HELP smartreversal_transactions_ingested_total Ingested transactions by payment rail.\n")
	sb.WriteString("# TYPE smartreversal_transactions_ingested_total counter\n")
	r.renderMap(&sb, "smartreversal_transactions_ingested_total", r.txnsIngested)

	sb.WriteString("\n# HELP smartreversal_exceptions_detected_total Detected transaction exceptions by rule code.\n")
	sb.WriteString("# TYPE smartreversal_exceptions_detected_total counter\n")
	r.renderMap(&sb, "smartreversal_exceptions_detected_total", r.exceptions)

	sb.WriteString("\n# HELP smartreversal_reversals_executed_total Reversal attempts by execution outcome.\n")
	sb.WriteString("# TYPE smartreversal_reversals_executed_total counter\n")
	r.renderMap(&sb, "smartreversal_reversals_executed_total", r.reversals)

	sb.WriteString("\n# HELP smartreversal_sla_breaches_total Total regulatory SLA deadline breaches.\n")
	sb.WriteString("# TYPE smartreversal_sla_breaches_total counter\n")
	sb.WriteString(fmt.Sprintf("smartreversal_sla_breaches_total %d\n", atomic.LoadUint64(&r.slaBreaches)))

	sb.WriteString("\n# HELP smartreversal_circuit_breaker_trips_total Total times a downstream circuit breaker opened.\n")
	sb.WriteString("# TYPE smartreversal_circuit_breaker_trips_total counter\n")
	r.renderMap(&sb, "smartreversal_circuit_breaker_trips_total", r.cbTrips)

	sb.WriteString("\n# HELP smartreversal_audit_records_total Cryptographically linked audit trail records written.\n")
	sb.WriteString("# TYPE smartreversal_audit_records_total counter\n")
	sb.WriteString(fmt.Sprintf("smartreversal_audit_records_total %d\n", atomic.LoadUint64(&r.auditEvents)))

	return sb.String()
}

func (r *Registry) renderMap(sb *strings.Builder, metric string, m map[string]*uint64) {
	r.mu.RLock()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := atomic.LoadUint64(m[k])
		sb.WriteString(fmt.Sprintf("%s{%s} %d\n", metric, k, val))
	}
	r.mu.RUnlock()
}

// Handler returns an HTTP handler serving Prometheus text exposition format.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Render()))
	}
}

// MetricsMiddleware tracks status and duration of HTTP requests on a Registry.
func MetricsMiddleware(reg *Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			reg.IncHTTPRequest(r.Method, r.URL.Path, rec.status)
		})
	}
}
