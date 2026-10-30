//go:build !windows

package sqlserver

import (
	"testing"

	"github.com/microsoft/go-mssqldb/integratedauth"
)

// Default provider stays ntlm on unix.
func Test_DefaultProviderNameUnchanged(t *testing.T) {
	if integratedauth.DefaultProviderName != "ntlm" {
		t.Fatalf("DefaultProviderName = %q, want %q", integratedauth.DefaultProviderName, "ntlm")
	}
}
