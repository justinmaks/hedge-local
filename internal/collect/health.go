package collect

import (
	"fmt"
	"net/http"
	"time"
)

// HealthCheck reports whether an OTLP collector is answering on the local
// port. It detects any collector: the background daemon, a service-managed
// process, or an embedded receiver in another hcli. This, not PID files, is
// the source of truth for "is telemetry being collected right now".
func HealthCheck(port int, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
