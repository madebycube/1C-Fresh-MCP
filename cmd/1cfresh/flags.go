package main

import (
	"flag"
	"strings"
)

func parseFlags(set *flag.FlagSet, args []string) error {
	options := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			positionals = append(positionals, args[index:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		options = append(options, arg)
		name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		option := set.Lookup(name)
		if option == nil || strings.Contains(arg, "=") {
			continue
		}
		if boolean, ok := option.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
			continue
		}
		if index+1 < len(args) {
			index++
			options = append(options, args[index])
		}
	}
	return set.Parse(append(options, positionals...))
}
