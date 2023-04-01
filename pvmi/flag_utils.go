// Utils for flag package

package pvmi

import (
	"regexp"
	"strings"
)

// Format multiline usage for help message:
func FormatFlagUsage(usage string) string {
	usage = strings.TrimSpace(usage)
	usage = regexp.MustCompile(`\n\s+`).ReplaceAllString(usage, "\n")
	usage = regexp.MustCompile(`[\t ]{2,}`).ReplaceAllString(usage, " ")
	return usage
}
