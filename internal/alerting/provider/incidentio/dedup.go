package incidentio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jniltinho/go-uptime/v7/internal/alerting/alert"
	"github.com/jniltinho/go-uptime/v7/internal/config/endpoint"
)

// generateDeduplicationKey generates a unique deduplication_key for incident.io
func generateDeduplicationKey(ep *endpoint.Endpoint, alert *alert.Alert) string {
	data := fmt.Sprintf("%s|%s|%s|%d", ep.Key(), alert.Type, alert.GetDescription(), time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
