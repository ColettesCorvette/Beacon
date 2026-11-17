package gather

import (
	"net"
	"sort"
	"testing"

	"github.com/grandcat/zeroconf"
)

func TestEntryURL(t *testing.T) {
	tests := []struct {
		name string
		e    *zeroconf.ServiceEntry
		want string
	}{
		{
			name: "ipv4 + port",
			e: &zeroconf.ServiceEntry{
				AddrIPv4:      []net.IP{net.ParseIP("192.168.40.42")},
				ServiceRecord: zeroconf.ServiceRecord{},
				Port:          8193,
			},
			want: "http://192.168.40.42:8193/metrics",
		},
		{
			name: "pas d'ipv4",
			e:    &zeroconf.ServiceEntry{Port: 8193},
			want: "",
		},
		{
			name: "port nul",
			e:    &zeroconf.ServiceEntry{AddrIPv4: []net.IP{net.ParseIP("10.30.0.1")}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entryURL(tt.e); got != tt.want {
				t.Errorf("entryURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeURLs(t *testing.T) {
	static := []string{"http://a/metrics", "http://b/metrics"}
	discovered := []string{"http://b/metrics", "http://c/metrics"}
	got := mergeURLs(static, discovered)
	sort.Strings(got)
	want := []string{"http://a/metrics", "http://b/metrics", "http://c/metrics"}
	if len(got) != len(want) {
		t.Fatalf("mergeURLs len = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mergeURLs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
