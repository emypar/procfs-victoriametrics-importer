// time package function re-directors useful for test mocking

package pvmi

import "time"

const (
	ISO8601Micro = "2006-01-02T15:04:05.999999-07:00"
)

var TimeNow func() time.Time = time.Now
var TimeSince func(time.Time) time.Duration = time.Since
