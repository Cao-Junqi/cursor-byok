package mitm

import "testing"

func TestIsTunnelOnlyHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"api3.cursor.sh", true},
		{"api3.cursor.sh:443", true},
		{"metrics.cursor.sh", true},
		{"metrics.cursor.sh:443", true},
		{"api2.cursor.sh", false},
		{"api2.cursor.sh:443", false},
		{"other.cursor.sh", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isTunnelOnlyHost(c.host); got != c.want {
			t.Errorf("isTunnelOnlyHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}
