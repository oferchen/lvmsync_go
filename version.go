package main

// version is populated at build time via -ldflags.
var version = "v0.2.0"

func init() { _ = version }
