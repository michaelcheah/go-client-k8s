package deployer

// Adapted from https://gist.github.com/ik5/d8ecde700972d4378d87

import "fmt"

var (
	DescriptionLog        = Yellow
	ActionLog             = Green
	MileStoneLog          = Magenta
	EventLog              = Teal
	ThisNeedsAttentionLog = Red
)

var (
	Black   = Colour("\033[1;30m%s\033[0m")
	Red     = Colour("\033[1;31m%s\033[0m")
	Green   = Colour("\033[1;32m%s\033[0m")
	Yellow  = Colour("\033[1;33m%s\033[0m")
	Purple  = Colour("\033[1;34m%s\033[0m")
	Magenta = Colour("\033[1;35m%s\033[0m")
	Teal    = Colour("\033[1;36m%s\033[0m")
	White   = Colour("\033[1;37m%s\033[0m")
)

func Colour(colorString string) func(...interface{}) string {
	sprint := func(args ...interface{}) string {
		if len(args) <= 1 {
			return fmt.Sprintf(colorString, args...)
		}
		return fmt.Sprintf(colorString, fmt.Sprintf(args[0].(string), args[1:]...))
	}
	return sprint
}
