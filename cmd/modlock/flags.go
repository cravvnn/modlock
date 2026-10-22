package main

import (
	"strings"

	"github.com/aperturerobotics/cli"
)

// flagsFirst moves a command's flags ahead of its arguments, so
// "modlock new my-mod --language go" reads as "modlock new --language go my-mod",
// and a subcommand's flags move behind the subcommand's name.
// The command line stops reading flags at the first argument and would take
// every later flag as another argument. A "--" keeps what follows as
// arguments.
func flagsFirst(commands []*cli.Command, args []string) []string {
	// Find the command after the program's own flags, which take no value.
	at := 1
	for at < len(args) && strings.HasPrefix(args[at], "-") {
		at++
	}
	if at == len(args) {
		return args
	}
	command := named(commands, args[at])
	if command == nil {
		return args
	}

	// Descend to the subcommand, such as "panorama compile".
	for at+1 < len(args) {
		sub := named(command.Subcommands, args[at+1])
		if sub == nil {
			break
		}
		command, at = sub, at+1
	}

	// Sort the command's words into flags, with their values, and arguments.
	var flags, rest []string
	words := args[at+1:]
	for i := 0; i < len(words); i++ {
		word := words[i]
		if word == "--" {
			rest = append(rest, words[i:]...)
			break
		}
		if len(word) < 2 || word[0] != '-' {
			rest = append(rest, word)
			continue
		}
		flags = append(flags, word)
		name := strings.TrimLeft(word, "-")
		if !strings.Contains(name, "=") && takesValue(command, name) && i+1 < len(words) {
			i++
			flags = append(flags, words[i])
		}
	}
	return append(append(args[:at+1:at+1], flags...), rest...)
}

// named returns the command called name, or nil.
func named(commands []*cli.Command, name string) *cli.Command {
	for _, c := range commands {
		if c.HasName(name) {
			return c
		}
	}
	return nil
}

// takesValue reports whether the command's flag of this name reads the next
// word as its value; a boolean flag, or an unknown one, does not.
func takesValue(command *cli.Command, name string) bool {
	for _, flag := range command.Flags {
		for _, n := range flag.Names() {
			if n == name {
				_, boolean := flag.(*cli.BoolFlag)
				return !boolean
			}
		}
	}
	return false
}
