// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package endpoint

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestResult_CertificateExpirationJSON(t *testing.T) {
	withCertificate, err := json.Marshal(&Result{Success: true, Timestamp: time.Now(), CertificateExpiration: 73 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	var decoded Result
	if err := json.Unmarshal(withCertificate, &decoded); err != nil || decoded.CertificateExpiration != 73*24*time.Hour {
		t.Errorf("expected the certificate expiration in the JSON of the result, got %s (err=%v)", withCertificate, err)
	}
	withoutCertificate, err := json.Marshal(&Result{Success: true, Timestamp: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(withoutCertificate), "certificateExpiration") {
		t.Errorf("expected no certificate expiration without certificate, got %s", withoutCertificate)
	}
}
