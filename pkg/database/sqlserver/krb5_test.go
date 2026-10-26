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

	// Normative wording (R-3): assert err != nil AND the error message does not
	// contain "not found". Do NOT assert a specific upstream error label: any
	// non-"not found" error from GetIntegratedAuthenticator proves the registry
	// lookup reached the krb5 provider rather than the miss
	// `provider %v not found` (getProvider in integratedauth/auth.go); upstream
	// may reword its config-load or keytab error messages.
	_, err := integratedauth.GetIntegratedAuthenticator(config)
	if err == nil {
		t.Fatalf("expected an error from the krb5 provider for a nonexistent krb5 config, got nil")
	}
	if strings.Contains(err.Error(), "not found") {
		t.Fatalf("krb5 provider not registered (lookup missed the registry): %v", err)
	}
}

// Test_DefaultProviderNameUnchanged guards that registering the krb5 provider
// does not change the platform default authenticator, which stays "ntlm" on
// unix (per auth_unix.go init() being its only unix writer).
func Test_DefaultProviderNameUnchanged(t *testing.T) {
	if integratedauth.DefaultProviderName != "ntlm" {
		t.Fatalf("DefaultProviderName = %q, want %q", integratedauth.DefaultProviderName, "ntlm")
	}
}
