// common metrics functions

package pvmi

import (
	"strconv"
)

// Sanitize label values as per
// https://github.com/Showmax/prometheus-docs/blob/master/content/docs/instrumenting/exposition_formats.md
func SanitizeLabelValue(v string) string {
	qVal := strconv.Quote(v)
	// Remove enclosing `"':
	return qVal[1 : len(qVal)-1]
}
