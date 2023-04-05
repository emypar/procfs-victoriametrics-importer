// Utils for flag package

package pvmi

import (
	"bytes"
	"strings"
)

const (
	DEFAULT_USAGE_WIDTH = 58
)

// Format command flag usage for help message.
func FormatFlagUsageWidth(usage string, width int) string {
	buf := &bytes.Buffer{}
	lineLen := 0
	for _, word := range strings.Fields(strings.TrimSpace(usage)) {
		if lineLen == 0 {
			n, err := buf.WriteString(word)
			if err != nil {
				return usage
			}
			lineLen = n
		} else {
			if lineLen+len(word)+1 > width {
				buf.WriteByte('\n')
				lineLen = 0
			} else {
				buf.WriteByte(' ')
				lineLen++
			}
			n, err := buf.WriteString(word)
			if err != nil {
				return usage
			}
			lineLen += n
		}
	}
	return buf.String()
}

func FormatFlagUsage(usage string) string {
	return FormatFlagUsageWidth(usage, DEFAULT_USAGE_WIDTH)
}
