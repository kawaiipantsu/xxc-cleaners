// Package humanize converts byte counts to/from human-readable strings.
package humanize

import (
	"fmt"
	"strconv"
	"strings"
)

var units = []string{"B", "K", "M", "G", "T", "P"}

// Bytes formats a byte count like "1.4G".
func Bytes(n int64) string {
	if n < 0 {
		return "?"
	}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d%s", n, units[i])
	}
	return fmt.Sprintf("%.1f%s", f, units[i])
}

// ParseBytes parses "100M", "1.5G", "500K", "2048" (bytes) into a byte count.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	// Accept optional trailing "B" and "iB".
	s = strings.TrimSuffix(s, "IB")
	s = strings.TrimSuffix(s, "B")
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'K':
		mult = 1 << 10
	case 'M':
		mult = 1 << 20
	case 'G':
		mult = 1 << 30
	case 'T':
		mult = 1 << 40
	case 'P':
		mult = 1 << 50
	}
	if mult != 1 || last < '0' || last > '9' {
		s = strings.TrimRight(s, "KMGTP")
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return int64(val * float64(mult)), nil
}

// Percent returns a 0..100 float as a short string.
func Percent(used, total int64) string {
	if total <= 0 {
		return "  -"
	}
	return fmt.Sprintf("%3.0f%%", float64(used)/float64(total)*100)
}
