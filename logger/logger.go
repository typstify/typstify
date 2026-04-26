package logger

import (
	"io"
	"log"
	"os"
)

const (
	MaxLogFileSize = 1024 * 1024 * 50
)

type FileLogger struct {
	*log.Logger
	file *os.File
}

var AppLogger *FileLogger

func NewFileLogger(filename string) *FileLogger {
	logger := &FileLogger{
		Logger: log.Default(),
	}

	// log to console and file
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	if stat, err := f.Stat(); err == nil {
		if stat.Size() >= MaxLogFileSize {
			f.Truncate(0)
			f.Seek(0, 0)
		}
	}

	wrt := io.MultiWriter(os.Stdout, f)

	logger.file = f
	logger.SetOutput(wrt)
	logger.Logger.SetFlags(log.Default().Flags() | log.Lshortfile)

	return logger
}

func (l *FileLogger) Close() error {
	return l.file.Close()
}

func InitLogger(filename string) {
	AppLogger = NewFileLogger(filename)
}
