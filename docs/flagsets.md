# FlagSets

LVMSync groups related command-line options into `pflag.FlagSet`s. Each group is
registered with the root command and bound to Viper so configuration can be read
from flags, environment variables, or a YAML file.

## Registration and binding

```go
defaults, _ := config.DefaultConfig()
flagSets := config.NewFlagSets(defaults)

root := pflag.NewFlagSet("lvmsync", pflag.ExitOnError)
for _, fs := range flagSets.All() {
        root.AddFlagSet(fs)
}

v := viper.New()
v.SetEnvPrefix("LVMSYNC")
v.AutomaticEnv()
for _, fs := range flagSets.All() {
        v.BindPFlags(fs)
}
```

`FlagSets.All()` returns the groups in a stable order, making it easy to iterate
and apply additional bindings.
