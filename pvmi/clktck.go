package pvmi

import "github.com/tklauser/go-sysconf"

const (
	FALLBACK_CLKTCK = 100
	// procfs module uses a predefined constant userHZ which may need a
	// correction factor if the runtime SC_CLK_TCK is different:
	PROCFS_USER_HZ = 100
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
