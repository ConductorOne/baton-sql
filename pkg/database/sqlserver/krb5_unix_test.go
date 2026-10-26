//go:build !windows

package sqlserver

import (
	"testing"

	"github.com/microsoft/go-mssqldb/integratedauth"
)

// Test_DefaultProviderNameUnchanged guards that registering the krb5 provider
// does not change the platform default authenticator, which stays "ntlm" on
// unix (per auth_unix.go init() being its only unix writer).
func Test_DefaultProviderNameUnchanged(t *testing.T) {
	if integratedauth.DefaultProviderName != "ntlm" {
		t.Fatalf("DefaultProviderName = %q, want %q", integratedauth.DefaultProviderName, "ntlm")
	}
}
