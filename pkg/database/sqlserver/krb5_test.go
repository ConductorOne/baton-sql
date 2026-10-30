package sqlserver

import (
	"strings"
	"testing"

	"github.com/microsoft/go-mssqldb/integratedauth"
	"github.com/microsoft/go-mssqldb/msdsn"
)

// A non-"not found" error proves the lookup reached the krb5 provider.
func Test_Krb5ProviderRegistered(t *testing.T) {
	config := msdsn.Config{
		Parameters: map[string]string{
			"authenticator":   "krb5",
			"krb5-realm":      "EXAMPLE.COM",
			"krb5-keytabfile": "/nonexistent",
			"krb5-configfile": "/nonexistent",
		},
	}

	// Both paths point at /nonexistent, so the error is deterministic without touching /etc/krb5.conf.
	_, err := integratedauth.GetIntegratedAuthenticator(config)
	if err == nil {
		t.Fatalf("expected an error from the krb5 provider for a nonexistent krb5 config, got nil")
	}
	if strings.Contains(err.Error(), "not found") {
		t.Fatalf("krb5 provider not registered (lookup missed the registry): %v", err)
	}
}

// Test_ProviderMissMessageControl is the negative control for
// Test_Krb5ProviderRegistered: it asserts the upstream registry-miss error
// still contains "not found". If upstream ever rewords that miss message, THIS
// test fails loudly and the guard's "not found" check in
// Test_Krb5ProviderRegistered must be updated to match.
func Test_ProviderMissMessageControl(t *testing.T) {
	config := msdsn.Config{
		Parameters: map[string]string{
			"authenticator": "krb5-no-such-provider",
		},
	}

	_, err := integratedauth.GetIntegratedAuthenticator(config)
	if err == nil {
		t.Fatalf("expected a registry-miss error for a bogus authenticator, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("upstream registry-miss message no longer contains \"not found\"; update the guard in Test_Krb5ProviderRegistered: %v", err)
	}
}
