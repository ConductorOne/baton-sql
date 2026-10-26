package sqlserver

import (
	"strings"
	"testing"

	"github.com/microsoft/go-mssqldb/integratedauth"
	"github.com/microsoft/go-mssqldb/msdsn"
)

// Test_Krb5ProviderRegistered proves the blank import in sqlserver.go registers
// the "krb5" integrated-auth provider. Normative wording (no specific upstream
// error label): the assertion is err != nil AND the error message does not
// contain "not found". Any non-"not found" error from GetIntegratedAuthenticator
// proves the registry lookup reached the krb5 provider rather than the miss
// `provider %v not found`; upstream may reword its config-load or keytab error
// messages, so no specific upstream string is asserted here.
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
