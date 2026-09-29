// *******************************************************
// YASMU - Yet Another System Monitoring Utility
// The utility for monitoring system integrity.
// And for web monitoring of websites availability.
// *******************************************************
// Author: Sergey Kalinin
// Home page: https://nuk-svk.ru
// License: GPL v3
// *******************************************************
// Utility module
// *******************************************************

package mainlibs

import (
    "strings"
)

// EscapeJSONBody escapes characters that break JSON if they end up
// inside string values: real line breaks, tabs, quotation marks, backslashes.
// The outer curly braces and the structure are not touched — this is a "blind" escape,
// suitable for simple cases when the template is assembled manually.
func EscapeJSONString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}
