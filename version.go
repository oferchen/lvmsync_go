package main

// version is populated at build time via -ldflags.
var version = "dev"

func init() { _ = version }
