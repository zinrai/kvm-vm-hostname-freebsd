package main

import "fmt"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func printVersion() {
	fmt.Printf("kvm-vm-hostname-freebsd %s (commit %s, built %s)\n", version, commit, date)
}
