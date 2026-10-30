package sqlserver

import (
	"strings"
	"testing"

	"github.com/microsoft/go-mssqldb/integratedauth"
	"github.com/microsoft/go-mssqldb/msdsn"
)

// A non-"not found" error proves the lookup reached the krb5 provider
// registered by the blank import in sqlserver.go.
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
