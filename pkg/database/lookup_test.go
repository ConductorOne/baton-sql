package database

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func mapLookup(m map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestLookupFunc_NoCrossContamination(t *testing.T) {
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n*2)

	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			host := fmt.Sprintf("host-a-%d", i)
			opts := ConnectOptions{
				Scheme:   "postgres",
				Host:     "${DB_HOST}",
				Database: "db",
				User:     "u",
				Password: "p",
				Lookup:   mapLookup(map[string]string{"DB_HOST": host}),
			}
			u, err := buildConnectionURL(opts)
			if err != nil {
				errs <- err
				return
			}
			if u.Hostname() != host {
				errs <- fmt.Errorf("expected host %q, got %q", host, u.Hostname())
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			host := fmt.Sprintf("host-b-%d", i)
			opts := ConnectOptions{
				Scheme:   "postgres",
				Host:     "${DB_HOST}",
				Database: "db",
				User:     "u",
				Password: "p",
				Lookup:   mapLookup(map[string]string{"DB_HOST": host}),
			}
			u, err := buildConnectionURL(opts)
			if err != nil {
				errs <- err
				return
			}
			if u.Hostname() != host {
				errs <- fmt.Errorf("expected host %q, got %q", host, u.Hostname())
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestLookupFunc_MissingKeyNamesKeyNotValue(t *testing.T) {
	opts := ConnectOptions{
		Scheme:   "postgres",
		Host:     "${SECRET_HOST}",
		Database: "db",
		Lookup: mapLookup(map[string]string{
			// intentionally missing SECRET_HOST
		}),
	}
	_, err := buildConnectionURL(opts)
	if err == nil {
		t.Fatal("expected missing key error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "SECRET_HOST") {
		t.Errorf("error should name the missing key: %v", err)
	}
	// Ensure no accidental secret material from other keys could appear
	if strings.Contains(msg, "super-secret-password") {
		t.Errorf("error leaked secret value: %v", err)
	}
}

func TestLookupFunc_ResolvesWithoutProcessEnv(t *testing.T) {
	// Ensure process env does not supply the value.
	t.Setenv("DB_HOST", "from-env")
	opts := ConnectOptions{
		Scheme:   "postgres",
		Host:     "${DB_HOST}",
		Database: "db",
		Lookup:   mapLookup(map[string]string{"DB_HOST": "from-lookup"}),
	}
	u, err := buildConnectionURL(opts)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "from-lookup" {
		t.Fatalf("expected from-lookup, got %q", u.Hostname())
	}
}
