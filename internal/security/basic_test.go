package security

import (
	"testing"
	"time"
)

func TestBasicConfig_IsValidUsingBcrypt(t *testing.T) {
	basicConfig := &BasicConfig{
		Username:                        "admin",
		PasswordBcryptHashBase64Encoded: "JDJhJDA4JDFoRnpPY1hnaFl1OC9ISlFsa21VS09wOGlPU1ZOTDlHZG1qeTFvb3dIckRBUnlHUmNIRWlT",
	}
	if !basicConfig.isValid() {
		t.Error("basicConfig should've been valid")
	}
}

func TestBasicConfig_IsValidWhenPasswordIsInvalidUsingBcrypt(t *testing.T) {
	basicConfig := &BasicConfig{
		Username:                        "admin",
		PasswordBcryptHashBase64Encoded: "",
	}
	if basicConfig.isValid() {
		t.Error("basicConfig shouldn't have been valid")
	}
}

func TestBasicConfig_ValidateAndSetDefaultsWithSessionTTL(t *testing.T) {
	scenarios := []struct {
		sessionTTL  time.Duration
		expectValid bool
		expectedTTL time.Duration
	}{
		{sessionTTL: 0, expectValid: true, expectedTTL: DefaultBasicSessionTTL},
		{sessionTTL: MinimumBasicSessionTTL, expectValid: true, expectedTTL: MinimumBasicSessionTTL},
		{sessionTTL: MaximumBasicSessionTTL, expectValid: true, expectedTTL: MaximumBasicSessionTTL},
		{sessionTTL: MinimumBasicSessionTTL - time.Second},
		{sessionTTL: MaximumBasicSessionTTL + time.Second},
		{sessionTTL: -time.Hour},
	}
	for _, scenario := range scenarios {
		basicConfig := &BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: "JDJhJDA4JDFoRnpPY1hnaFl1OC9ISlFsa21VS09wOGlPU1ZOTDlHZG1qeTFvb3dIckRBUnlHUmNIRWlT", SessionTTL: scenario.sessionTTL}
		if valid := basicConfig.validateAndSetDefaults(); valid != scenario.expectValid {
			t.Errorf("session-ttl %s: expected valid=%v, got %v", scenario.sessionTTL, scenario.expectValid, valid)
		} else if valid && basicConfig.SessionTTL != scenario.expectedTTL {
			t.Errorf("session-ttl %s: expected %s, got %s", scenario.sessionTTL, scenario.expectedTTL, basicConfig.SessionTTL)
		}
	}
}

// TestBasicConfig_isValid_PasswordHash refuses, when the configuration is validated, a password that is not the base64
// of a bcrypt hash. It used to be accepted by go-uptime config validate and to panic when the server started.
func TestBasicConfig_isValid_PasswordHash(t *testing.T) {
	const valid = "JDJhJDA4JDFoRnpPY1hnaFl1OC9ISlFsa21VS09wOGlPU1ZOTDlHZG1qeTFvb3dIckRBUnlHUmNIRWlT"
	if !(&BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: valid}).isValid() {
		t.Error("expected a bcrypt hash in base64 to be valid")
	}
	for name, hash := range map[string]string{
		"placeholder of the example": "PASTE-THE-HASH-HERE",
		"not base64":                 "not base64!",
		"base64 of something else":   "aGVsbG8gd29ybGQ=",
		"bcrypt hash without base64": "$2a$08$1hFzOcXghYu8/HJQlkmUKOp8iOSVNL9Gdmjy1oowHrDARyGRcHEiS",
		"plain password":             "hunter2",
		"empty":                      "",
	} {
		if (&BasicConfig{Username: "admin", PasswordBcryptHashBase64Encoded: hash}).isValid() {
			t.Errorf("%s: expected %q to be refused", name, hash)
		}
	}
}
