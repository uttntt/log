package log

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	LOG_LEVEL_OFF     = 0
	LOG_LEVEL_FATAL   = 1
	LOG_LEVEL_WARNING = 2
	LOG_LEVEL_TRACE   = 3
	LOG_LEVEL_DEBUG   = 4
)

type LoggerModule *int

type Logger struct {
	fileBeginTime int
	fileSplitTime int
	filePath      string
	fileName      string
	file          *os.File
	logger        *log.Logger
	exitChan      chan bool
	moduleLevels  map[string]LoggerModule
	mainLevel     int
	mu            sync.Mutex
	debug2stdout  bool
	// For counter
	counterDumpTime     int
	counter             map[string]*int64
	counterHeader       []string
	counterValue        []string
	counterDumpTicker   *time.Ticker
	countFile           *os.File
	counterHeaderString string
}

func NewLogger(filePath, fileName string, counters []string, counterDumpTime, fileSplitTime int, debug2stdout bool) *Logger {
	now := time.Now()
	fileBeginTime := int(now.Unix() / int64(fileSplitTime))
	file, err := os.OpenFile(fmt.Sprintf("%s%c%s.%d.%d-%02d-%02d_%02d-%02d-%02d.log",
		filePath, os.PathSeparator, fileName, fileBeginTime,
		now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second()),
		os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Printf("create log file: %s%c%s failed\n", filePath, os.PathSeparator, fileName)
		return nil
	}
	var countFile *os.File
	if counters != nil && len(counters) > 0 {
		countFile, err = os.OpenFile(fmt.Sprintf("%s%c%s_counter.%d.%d-%02d-%02d_%02d-%02d-%02d.log",
			filePath, os.PathSeparator, fileName, fileBeginTime,
			now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second()),
			os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			return nil
		}
	}
	counterDumpTicker := time.NewTicker(time.Duration(counterDumpTime) * time.Second)
	logger := Logger{
		file:                file,
		filePath:            filePath,
		fileName:            fileName,
		fileBeginTime:       fileBeginTime,
		fileSplitTime:       fileSplitTime,
		counterDumpTime:     counterDumpTime,
		logger:              log.New(file, "", log.LstdFlags),
		counter:             make(map[string]*int64),
		counterHeader:       counters,
		counterValue:        make([]string, len(counters)),
		counterDumpTicker:   counterDumpTicker,
		countFile:           countFile,
		exitChan:            make(chan bool),
		counterHeaderString: "time," + strings.Join(counters, ",") + "\r\n",
		mainLevel:           LOG_LEVEL_FATAL,
		moduleLevels:        make(map[string]LoggerModule),
		debug2stdout:        debug2stdout,
	}
	if countFile != nil {
		countFile.WriteString(logger.counterHeaderString)
		countFile.Sync()
		for index, counter := range counters {
			var i int64 = 0
			logger.counter[counter] = &i
			logger.counterValue[index] = ""
		}
		go logger.counterDump()
	}
	return &logger
}

func (logger *Logger) MainLevel() int {
	return logger.mainLevel
}
func (logger *Logger) SetMainLevel(level int) {
	logger.mainLevel = level
}

func (logger *Logger) Close() {
	logger.exitChan <- true
	logger.counterDumpTicker.Stop()
	logger.file.Close()
}

func (logger *Logger) ForcePrint(v ...interface{}) {
	logger.logger.Print(v...)
}
func (logger *Logger) ForcePrintf(format string, v ...interface{}) {
	logger.logger.Printf(format, v...)
}
func (logger *Logger) ForcePrintln(v ...interface{}) {
	logger.logger.Println(v...)
}

func (logger *Logger) Print(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_TRACE {
		logger.logger.Print(v...)
	}
}
func (logger *Logger) Printf(format string, v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_TRACE {
		logger.logger.Printf(format, v...)
	}
}
func (logger *Logger) Println(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_TRACE {
		logger.logger.Println(v...)
	}
}

func (logger *Logger) Fatal(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_FATAL {
		logger.logger.Print(v...)
	}
}
func (logger *Logger) Fatalf(format string, v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_FATAL {
		logger.logger.Printf(format, v...)
	}
}
func (logger *Logger) Fatalln(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_FATAL {
		logger.logger.Println(v...)
	}
}

func (logger *Logger) Warning(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_WARNING {
		logger.logger.Print(v...)
	}
}
func (logger *Logger) Warningf(format string, v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_WARNING {
		logger.logger.Printf(format, v...)
	}
}
func (logger *Logger) Warningln(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_WARNING {
		logger.logger.Println(v...)
	}
}

func (logger *Logger) Debug(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_DEBUG {
		if logger.debug2stdout {
			fmt.Print(v...)
		} else {
			logger.logger.Print(v...)
		}
	}
}
func (logger *Logger) Debugf(format string, v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_DEBUG {
		if logger.debug2stdout {
			fmt.Printf(format, v...)
		} else {
			logger.logger.Printf(format, v...)
		}
	}
}
func (logger *Logger) Debugln(v ...interface{}) {
	if logger.mainLevel >= LOG_LEVEL_DEBUG {
		if logger.debug2stdout {
			fmt.Println(v...)
		} else {
			logger.logger.Println(v...)
		}
	}
}

func (logger *Logger) LoggerModule(moduleName string) LoggerModule {
	logger.mu.Lock()
	m, found := logger.moduleLevels[moduleName]
	if !found {
		defaultLevel := LOG_LEVEL_FATAL
		m = &defaultLevel
		logger.moduleLevels[moduleName] = m
	}
	logger.mu.Unlock()
	return m
}

func (logger *Logger) ModuleLevel(module LoggerModule) int {
	return *module
}
func (logger *Logger) SetModuleLevel(module LoggerModule, level int) {
	*module = level
}
func (logger *Logger) ModuleLevelCheck(module LoggerModule, level int) bool {
	return (level <= logger.mainLevel || level <= *module)
}
func (logger *Logger) SetModuleLevelByName(moduleName string, level int) error {
	if moduleName == "main" {
		logger.SetMainLevel(level)
		return nil
	} else {
		logger.mu.Lock()
		m, found := logger.moduleLevels[moduleName]
		if !found {
			return errors.New(fmt.Sprintf("Not found module name %s", moduleName))
		}
		logger.mu.Unlock()
		*m = level
	}
	return nil
}

func (logger *Logger) ModulePrint(module LoggerModule, level int, v ...interface{}) {
	if logger.ModuleLevelCheck(module, level) {
		logger.logger.Print(v...)
	}
}

func (logger *Logger) ModulePrintf(module LoggerModule, level int, format string, v ...interface{}) {
	if logger.ModuleLevelCheck(module, level) {
		logger.logger.Printf(format, v...)
	}
}

func (logger *Logger) ModulePrintln(module LoggerModule, level int, v ...interface{}) {
	if logger.ModuleLevelCheck(module, level) {
		logger.logger.Println(v...)
	}
}

func (logger *Logger) counterDump() {
	for {
		select {
		case now := <-logger.counterDumpTicker.C:
			fileBeginTime := int(now.Unix() / int64(logger.fileSplitTime))
			if fileBeginTime > logger.fileBeginTime {
				dateStr := fmt.Sprintf("%d-%02d-%02d", now.Year(), now.Month(), now.Day())
				timeStr := fmt.Sprintf("%02d-%02d-%02d", now.Hour(), now.Minute(), now.Second())
				// New file
				file, err := os.OpenFile(fmt.Sprintf("%s%c%s.%d.%s_%s.log",
					logger.filePath, os.PathSeparator, logger.fileName, fileBeginTime,
					dateStr, timeStr),
					os.O_CREATE|os.O_RDWR, 0666)
				if err == nil {
					logger.file.Close()
					logger.file = file
					logger.logger = log.New(logger.file, "", log.LstdFlags)
				}
				countFile, err := os.OpenFile(fmt.Sprintf("%s%c%s_counter.%d.%s_%s.log",
					logger.filePath, os.PathSeparator, logger.fileName, fileBeginTime,
					dateStr, timeStr),
					os.O_CREATE|os.O_RDWR, 0666)
				if err == nil {
					logger.countFile.Close()
					logger.countFile = countFile
				}
				countFile.WriteString(logger.counterHeaderString)
				countFile.Sync()
				logger.fileBeginTime = fileBeginTime
			}
			values := make([]string, len(logger.counterHeader))
			for index, counter := range logger.counterHeader {
				// Todo: atomic
				value := atomic.LoadInt64(logger.counter[counter])
				atomic.AddInt64(logger.counter[counter], int64(-1)*value)
				values[index] = strconv.FormatInt(value, 10)
			}
			logger.countFile.WriteString(fmt.Sprintf("%d,", now.Unix()))
			logger.countFile.WriteString(strings.Join(values, ","))
			logger.countFile.WriteString("\r\n")
			logger.countFile.Sync()
		case <-logger.exitChan:
			return
		}
	}
}

func (logger *Logger) Add(name string, value int64) {
	v, _ := logger.counter[name]
	if v != nil {
		atomic.AddInt64(v, value)
	}
}

func (logger *Logger) Store(name string, value int64) {
	v, _ := logger.counter[name]
	if v != nil {
		atomic.StoreInt64(v, value)
	}
}

// If the value larger than value in map, replace it.
func (logger *Logger) Max(name string, value int64) {
	v, _ := logger.counter[name]
	if v != nil {
		cur := atomic.LoadInt64(v)
		if value > cur {
			atomic.StoreInt64(v, value)
		}
	}
}
