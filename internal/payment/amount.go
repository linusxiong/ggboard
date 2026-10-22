package payment

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FormatAmount renders cents as a decimal string with two places ("1234" -> "12.34", "-5" -> "-0.05").
func FormatAmount(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		if cents == math.MinInt64 {
			return "-92233720368547758.08"
		}
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// ParseAmount parses a decimal amount ("12.3", "12.34", "12") into cents. More than two decimals is an error.
func ParseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("payment: amount %q has more than two decimals", s)
	}
	frac += strings.Repeat("0", 2-len(frac))
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("payment: invalid amount %q", s)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("payment: invalid amount %q", s)
	}
	v := w*100 + f
	if neg {
		v = -v
	}
	return v, nil
}
