package zap

// Logger is a minimal placeholder used by the project to avoid pulling in the
// real zap dependency. It purposefully exposes only the pieces of API that are
// required by the application code.
type Logger struct{}

// globalL holds the process-wide logger instance returned by L(). It defaults
// to a new no-op Logger so calls are always safe.
var globalL = &Logger{}

// L returns the package-wide *Logger. This mimics zap's behavior where the
// logger can be replaced by calling ReplaceGlobals.
func L() *Logger { return globalL }

// ReplaceGlobals swaps the package-wide *Logger returned by L(). It is a no-op
// in this stub other than storing the provided logger.
func ReplaceGlobals(l *Logger) { globalL = l }

// NewProduction returns a new *Logger configured for production use. In this
// stub implementation it simply returns a new no-op Logger and a nil error so
// callers can continue to compile and run.
func NewProduction() (*Logger, error) { return &Logger{}, nil }

func (l *Logger) Info(msg string, fields ...Field)  {}
func (l *Logger) Warn(msg string, fields ...Field)  {}
func (l *Logger) Error(msg string, fields ...Field) {}
func (l *Logger) Debug(msg string, fields ...Field) {}
func (l *Logger) Fatal(msg string, fields ...Field) {}

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
