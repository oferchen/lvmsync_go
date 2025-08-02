package zap

type Logger struct{}

type Field struct{}

func L() *Logger { return &Logger{} }

func (l *Logger) Info(msg string, fields ...Field)  {}
func (l *Logger) Debug(msg string, fields ...Field) {}
func (l *Logger) Warn(msg string, fields ...Field)  {}

func String(key, val string) Field          { return Field{} }
func Uint64(key string, val uint64) Field   { return Field{} }
func Error(err error) Field                 { return Field{} }
func Float64(key string, val float64) Field { return Field{} }
