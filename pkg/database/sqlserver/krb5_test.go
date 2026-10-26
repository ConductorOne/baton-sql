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

	authenticator, err := integratedauth.GetIntegratedAuthenticator(config)
	if err != nil && strings.Contains(err.Error(), "not found") {
		t.Fatalf("krb5 provider not registered (lookup missed the registry): %v", err)
	}
	if err != nil {
		// A config-load or keytab-validation error is expected here (the paths
		// above do not exist); it proves the krb5 provider was found and ran.
		return
	}
	if authenticator == nil {
		t.Fatalf("expected an error or a non-nil authenticator, got nil authenticator without error")
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