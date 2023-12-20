package ui

import (
	"flag"
)

type InteractivityLevel int

// InteractivityLevel constants are ordered such that ordering comparisons
// make as much sense as equality comparisons.
const (
	NonInteractive InteractivityLevel = iota
	MinimallyInteractive
	FullyInteractive
)

func Interactivity() InteractivityLevel {
	if fullyInteractive == nil || minimallyInteractive == nil || nonInteractive == nil { // true if InteractivityFlags was never called
		return MinimallyInteractive // default
	}

	if !*fullyInteractive && !*minimallyInteractive && !*nonInteractive {
		return MinimallyInteractive // default
	}
	if !*fullyInteractive && !*minimallyInteractive && *nonInteractive {
		return NonInteractive
	}
	if !*fullyInteractive && *minimallyInteractive && !*nonInteractive {
		return MinimallyInteractive
	}
	if *fullyInteractive && !*minimallyInteractive && !*nonInteractive {
		return FullyInteractive
	}
	Fatal("can't mix -non-interactive, -minimally-interactive, and -fully-interactive")
	return 0
}

func InteractivityFlags() {
	fullyInteractive = flag.Bool("fully-interactive", false, "fully interactive mode - all prompts and file editors")
	minimallyInteractive = flag.Bool("minimally-interactive", false, "minimally interactive mode - only prompts with no cached responses (default)")
	nonInteractive = flag.Bool("non-interactive", false, "non-interactive mode - no prompts or file editors")
}

func Quiet() {
	op(opQuiet, "")
}

var fullyInteractive, minimallyInteractive, nonInteractive *bool
