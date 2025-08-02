package viper

type Viper struct {
	data map[string]string
}

func New() *Viper                                { return &Viper{data: make(map[string]string)} }
func (v *Viper) SetConfigName(name string)       {}
func (v *Viper) SetConfigType(t string)          {}
func (v *Viper) AddConfigPath(p string)          {}
func (v *Viper) AutomaticEnv()                   {}
func (v *Viper) SetEnvKeyReplacer(r interface{}) {}
func (v *Viper) SetEnvPrefix(p string)           {}
func (v *Viper) BindPFlags(fs interface{}) error { return nil }
func (v *Viper) GetString(key string) string     { return v.data[key] }
func (v *Viper) SetConfigFile(file string)       {}
func (v *Viper) ReadInConfig() error             { return nil }
func (v *Viper) Unmarshal(i interface{}) error   { return nil }
