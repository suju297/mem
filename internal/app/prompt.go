package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

func promptYesNo(in io.Reader, out io.Writer, question string, defaultYes bool) (bool, error) {
	reader := bufio.NewReader(in)
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	for {
		fmt.Fprintf(out, "%s %s: ", question, suffix)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}

		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}

		fmt.Fprintln(out, "Please answer yes or no.")
		if errors.Is(err, io.EOF) {
			return defaultYes, nil
		}
	}
}

func promptText(in io.Reader, out io.Writer, question string, allowEmpty bool) (string, error) {
	reader := bufio.NewReader(in)
	for {
		fmt.Fprintf(out, "%s: ", question)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}

		value := strings.TrimSpace(line)
		if value != "" || allowEmpty {
			return value, nil
		}

		fmt.Fprintln(out, "This value is required.")
		if errors.Is(err, io.EOF) {
			return "", nil
		}
	}
}
