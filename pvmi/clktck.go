package pvmi

import "github.com/tklauser/go-sysconf"

const (
	FALLBACK_CLKTCK = 100
)

var ClktckSec float64

func init() {
	clktck, err := sysconf.Sysconf(sysconf.SC_CLK_TCK)
	if err != nil {
		Log.Warnf("sysconf(SC_CLK_TCK): %s, will use fallback %d", err, FALLBACK_CLKTCK)
		clktck = FALLBACK_CLKTCK
	}

	ClktckSec = 1. / float64(clktck)
}
