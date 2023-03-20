package pvmi

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Log = &logrus.Logger{
	Out: os.Stderr,
	Formatter: &logrus.TextFormatter{
		DisableColors: true,
		FullTimestamp: true,
	},
	Hooks: make(logrus.LevelHooks),
	Level: logrus.InfoLevel,
}

func SetLogLevelByName(levelName string) error {
	level, err := logrus.ParseLevel(levelName)
	if err != nil {
		return err
	}
	Log.SetLevel(level)
	return nil
}
