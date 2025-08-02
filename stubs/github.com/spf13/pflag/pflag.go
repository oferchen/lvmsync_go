package pflag

import "time"

type FlagSet struct{}

func NewFlagSet(name string, errorHandling int) *FlagSet { return &FlagSet{} }

func (f *FlagSet) String(name, value, usage string) *string         { return new(string) }
func (f *FlagSet) Bool(name string, value bool, usage string) *bool { return new(bool) }
func (f *FlagSet) Int(name string, value int, usage string) *int    { return new(int) }
func (f *FlagSet) Duration(name string, value time.Duration, usage string) *time.Duration {
	return new(time.Duration)
}
func (f *FlagSet) CountP(name, shorthand, usage string) *int { return new(int) }
func (f *FlagSet) AddFlagSet(fs *FlagSet)                    {}
func (f *FlagSet) Parse(args []string) error                 { return nil }
func (f *FlagSet) PrintDefaults()                            {}

var CommandLine = NewFlagSet("", ExitOnError)

func Parse() {}

var Usage func()

const (
	ExitOnError = iota
)
