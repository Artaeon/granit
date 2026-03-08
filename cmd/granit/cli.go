package main

import (
	"fmt"
	"os"
	"strings"
)

// resolveVaultPath determines the vault path from:
// 1. Explicit argument at the given index in os.Args
// 2. GRANIT_VAULT environment variable
// 3. Current working directory as fallback
func resolveVaultPath(argIndex int) string {
	if len(os.Args) > argIndex {
		return os.Args[argIndex]
	}
	if envVault := os.Getenv("GRANIT_VAULT"); envVault != "" {
		return envVault
	}
	return "."
}

// hasFlag checks whether a flag (e.g. "--json") is present in os.Args.
func hasFlag(name string) bool {
	for _, arg := range os.Args {
		if arg == name {
			return true
		}
	}
	return false
}

// getFlagValue returns the value of a --key=value or --key value flag.
// Returns empty string if not found.
func getFlagValue(name string) string {
	for i, arg := range os.Args {
		// --key=value form
		if strings.HasPrefix(arg, name+"=") {
			return arg[len(name)+1:]
		}
		// --key value form
		if arg == name && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

// getPositionalArgs returns non-flag arguments after the subcommand.
// startIndex is the index of the first argument after the subcommand name.
func getPositionalArgs(startIndex int) []string {
	var args []string
	for i := startIndex; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--") {
			// Skip --key=value
			if !strings.Contains(arg, "=") {
				// Skip the next arg too (it's the flag value)
				i++
			}
			continue
		}
		args = append(args, arg)
	}
	return args
}

// exitError prints an error message to stderr and exits with code 1.
func exitError(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
