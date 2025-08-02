package zap

type Logger struct{}

func L() *Logger { return &Logger{} }

func (l *Logger) Info(msg string, fields ...Field)  {}
func (l *Logger) Warn(msg string, fields ...Field)  {}
func (l *Logger) Error(msg string, fields ...Field) {}
func (l *Logger) Debug(msg string, fields ...Field) {}

// Sugared Logger placeholder
func (l *Logger) Sugar() *SugaredLogger { return &SugaredLogger{} }

func (l *Logger) Sync() error { return nil }

type SugaredLogger struct{}

func (s *SugaredLogger) Info(args ...interface{}) {}

// Field type and constructors

type Field struct{}

func String(key, val string) Field          { return Field{} }
func Int(key string, val int) Field         { return Field{} }
func Int64(key string, val int64) Field     { return Field{} }
func Uint64(key string, val uint64) Field   { return Field{} }
func Float64(key string, val float64) Field { return Field{} }
func Bool(key string, val bool) Field       { return Field{} }
func Error(err error) Field                 { return Field{} }
