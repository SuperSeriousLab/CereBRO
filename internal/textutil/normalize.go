// Package textutil provides shared text processing utilities for COGs.
package textutil

import "strings"

// NormalizeQuotes replaces Unicode curly/smart quotes with ASCII equivalents.
func NormalizeQuotes(s string) string {
	r := strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", // single curly quotes
		"\u201C", "\"", "\u201D", "\"", // double curly quotes
	)
	return r.Replace(s)
}
