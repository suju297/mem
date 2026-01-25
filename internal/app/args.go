package app

import (
	"fmt"
	"strings"
)

type flagSpec struct {
	RequiresValue bool
}

type globalFlags struct {
	DataDir string
}

func splitGlobalFlags(args []string) ([]string, globalFlags, error) {
	var out []string
	var globals globalFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		if arg == "--data-dir" || strings.HasPrefix(arg, "--data-dir=") {
			value := ""
			if arg == "--data-dir" {
				if i+1 >= len(args) {
					return nil, globals, fmt.Errorf("missing value for --data-dir")
				}
				value = args[i+1]
				i++
			} else {
				value = strings.TrimPrefix(arg, "--data-dir=")
			}
			if strings.TrimSpace(value) == "" {
				return nil, globals, fmt.Errorf("missing value for --data-dir")
			}
			globals.DataDir = value
			continue
		}
		out = append(out, arg)
	}
	return out, globals, nil
}

func splitFlagArgs(args []string, spec map[string]flagSpec) ([]string, []string, error) {
	var positional []string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				positional = append(positional, args[i+1:]...)
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			name := strings.TrimLeft(arg, "-")
			if name == "" {
				positional = append(positional, arg)
				continue
			}
			key := name
			if idx := strings.Index(name, "="); idx >= 0 {
				key = name[:idx]
			}
			if spec != nil {
				if f, ok := spec[key]; ok {
					flagArgs = append(flagArgs, arg)
					if f.RequiresValue && !strings.Contains(arg, "=") {
						if i+1 >= len(args) {
							return nil, nil, fmt.Errorf("missing value for --%s", key)
						}
						flagArgs = append(flagArgs, args[i+1])
						i++
					}
					continue
				}
			}
		}
		positional = append(positional, arg)
	}
	return positional, flagArgs, nil
}
