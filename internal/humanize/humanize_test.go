package humanize

import "testing"

func TestBytes(t *testing.T) {
	cases := map[int64]string{
		0:              "0B",
		512:            "512B",
		1024:           "1.0K",
		1536:           "1.5K",
		1 << 20:        "1.0M",
		1 << 30:        "1.0G",
		5 * (1 << 40):  "5.0T",
	}
	for in, want := range cases {
		if got := Bytes(in); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	cases := map[string]int64{
		"100M":  100 << 20,
		"1.5G":  int64(1.5 * float64(1<<30)),
		"500K":  500 << 10,
		"2048":  2048,
		"1GiB":  1 << 30,
		"10m":   10 << 20,
	}
	for in, want := range cases {
		got, err := ParseBytes(in)
		if err != nil {
			t.Errorf("ParseBytes(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseBytes(%q) = %d, want %d", in, got, want)
		}
	}
	if _, err := ParseBytes("nonsense"); err == nil {
		t.Error("expected error for nonsense input")
	}
}
