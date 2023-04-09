package pvmi

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	DEFAULT_LOG_LEVEL           = logrus.InfoLevel
	LOGGER_COMPONENT_FIELD_NAME = "comp"
)

// Maintain a cache for caller PC -> (file:line#, function) to speed up the formatting:
type LogFunctionFilePair struct {
	function string
	file     string
}

type LogFunctionFileCache struct {
	m           *sync.Mutex
	fnFileCache map[uintptr]*LogFunctionFilePair
}

func (c *LogFunctionFileCache) LogCallerPrettyfier(f *runtime.Frame) (function string, file string) {
	c.m.Lock()
	defer c.m.Unlock()
	fnFile := c.fnFileCache[f.PC]
	if fnFile == nil {
		_, filename := path.Split(f.File)
		// function := f.Function
		// i := strings.LastIndex(function, "/")
		// if i >= 0 {
		// 	function = function[i+1:]
		// }
		function := ""
		fnFile = &LogFunctionFilePair{
			function,
			fmt.Sprintf("%s:%d", filename, f.Line),
		}
		c.fnFileCache[f.PC] = fnFile
	}
	return fnFile.function, fnFile.file
}

var logFunctionFileCache = &LogFunctionFileCache{
	m:           &sync.Mutex{},
	fnFileCache: make(map[uintptr]*LogFunctionFilePair),
}

var KeyIndexOrder = map[string]int{
	// The desired order is time, level, file, func, other keys sorted
	// alphabetically, msg. Use negative numbers to capitalize on the fact that
	// other keys will return 0 at lookup.
	logrus.FieldKeyTime:         -5,
	logrus.FieldKeyLevel:        -4,
	LOGGER_COMPONENT_FIELD_NAME: -3,
	logrus.FieldKeyFile:         -2,
	logrus.FieldKeyFunc:         -1,
	logrus.FieldKeyMsg:          1,
}

type LogKeySortInterface struct {
	keys []string
}

func (d *LogKeySortInterface) Len() int {
	return len(d.keys)
}

func (d *LogKeySortInterface) Less(i, j int) bool {
	key_i, key_j := d.keys[i], d.keys[j]
	order_i, order_j := KeyIndexOrder[key_i], KeyIndexOrder[key_j]
	if order_i != 0 || order_j != 0 {
		return order_i < order_j
	}
	return strings.Compare(key_i, key_j) == -1
}

func (d *LogKeySortInterface) Swap(i, j int) {
	d.keys[i], d.keys[j] = d.keys[j], d.keys[i]
}

func LogSortkeys(keys []string) {
	sort.Sort(&LogKeySortInterface{keys})
}

var LogTextFormatter = &logrus.TextFormatter{
	DisableColors:    true,
	FullTimestamp:    true,
	CallerPrettyfier: logFunctionFileCache.LogCallerPrettyfier,
	DisableSorting:   false,
	SortingFunc:      LogSortkeys,
}

var LogJsonFormatter = &logrus.JSONFormatter{
	CallerPrettyfier: logFunctionFileCache.LogCallerPrettyfier,
}

var Log = &logrus.Logger{
	ReportCaller: true,
	Out:          os.Stderr,
	Formatter:    LogTextFormatter,
	Hooks:        make(logrus.LevelHooks),
	Level:        DEFAULT_LOG_LEVEL,
}

func GetLogLevelNames() []string {
	levelNames := make([]string, len(logrus.AllLevels))
	for i, level := range logrus.AllLevels {
		levelNames[i] = level.String()
	}
	return levelNames
}

func SetLoggerFromArgs() error {
	level, err := logrus.ParseLevel(*LoggerLevelArg)
	if err != nil {
		return err
	}
	Log.SetLevel(level)
	if *LoggerUseJsonArg {
		Log.SetFormatter(LogJsonFormatter)
	}
	return nil
}
