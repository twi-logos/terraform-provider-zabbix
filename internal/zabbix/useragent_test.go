package zabbix

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUserAgent pins that the caller's identification reaches the wire. The
// provider sets this to carry its own version, so a Zabbix access log records
// which provider build made a call — the first thing worth knowing when a
// report says "it broke after upgrading".
func TestUserAgent(t *testing.T) {
	for _, tc := range []struct{ name, configured, want string }{
		{"caller supplies one", "terraform-provider-zabbix/2.0.0", "terraform-provider-zabbix/2.0.0"},
		{"caller supplies none", "", "github.com/twi-logos/terraform-provider-zabbix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("User-Agent")
				fmt.Fprint(w, `{"jsonrpc":"2.0","result":"7.4.13","id":1}`)
			}))
			defer srv.Close()

			if _, err := NewAPI(Config{Url: srv.URL, UserAgent: tc.configured}); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("User-Agent = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewAPIMinimumVersion(t *testing.T) {
	for _, tc := range []struct {
		version string
		wantErr string
	}{
		{version: "7.2.99", wantErr: "Zabbix 7.4 or newer is required; server reports 7.2.99"},
		{version: "7.4.0"},
		{version: "8.0.0"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"jsonrpc":"2.0","result":%q,"id":1}`, tc.version)
			}))
			defer srv.Close()

			_, err := NewAPI(Config{Url: srv.URL})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("NewAPI() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
