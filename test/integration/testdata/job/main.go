// Command job is the program the integration tests use as a job.
//
// It exists so that a test can watch a real process, started by the real cronx
// binary, and see exactly what that process received: its arguments verbatim,
// its working directory, its environment and its process group. It is
// deliberately tiny, and it lives under testdata, which the Go toolchain
// ignores: it is never part of what cronx ships.
//
// The program is driven by a list of verbs, each taking a fixed number of
// arguments:
//
//	report <path>                  record what the process sees, as JSON
//	out <text>                     write text to standard output
//	err <text>                     write text to standard error
//	write <path> <content>         write content to a file
//	append <path> <content>        append content to a file
//	sleep <duration>               sleep for a Go duration
//	exit <code>                    exit with the given status
//	fail <path> <times> <code>     fail while the attempt counter says so
//	spawn <report> <duration>      start a child of itself, then sleep
//	ignore-term <duration>         refuse SIGTERM, then sleep
//
// Everything after the first "--" is payload: it is recorded by "report" and
// otherwise ignored. That is how a test passes arguments a shell would have
// interpreted, which is the point of passing them at all.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// report is what the program records about itself when asked to.
type report struct {
	// Argv is everything the program was started with, argv[0] included.
	Argv []string `json:"argv"`
	// Payload is everything after the first "--".
	Payload []string `json:"payload"`
	// Dir is the working directory the process observes.
	Dir string `json:"dir"`
	// PID is the process identifier.
	PID int `json:"pid"`
	// PGID is the process group the process belongs to, or zero when the
	// system has none.
	PGID int `json:"pgid"`
	// Env is the environment the process received, as a map.
	Env map[string]string `json:"env"`
}

func main() {
	verbs, payload := split(os.Args[1:])
	if err := apply(verbs, payload); err != nil {
		fmt.Fprintln(os.Stderr, "job:", err)
		os.Exit(2)
	}
}

// split separates the verbs from the payload that follows the first "--".
func split(args []string) (verbs, payload []string) {
	for index, arg := range args {
		if arg == "--" {
			return args[:index], args[index+1:]
		}
	}
	return args, nil
}

// arguments reads the verb arguments one at a time.
type arguments struct {
	values []string
	at     int
}

// remaining reports whether a verb is left to read.
func (a *arguments) remaining() bool {
	return a.at < len(a.values)
}

// next returns the next verb.
func (a *arguments) next() string {
	verb := a.values[a.at]
	a.at++
	return verb
}

// take returns the next argument of a verb.
func (a *arguments) take(verb string) (string, error) {
	if a.at >= len(a.values) {
		return "", fmt.Errorf("the verb %q needs another argument", verb)
	}
	value := a.values[a.at]
	a.at++
	return value, nil
}

// duration returns the next argument of a verb as a duration.
func (a *arguments) duration(verb string) (time.Duration, error) {
	value, err := a.take(verb)
	if err != nil {
		return 0, err
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("the verb %q wants a duration, got %q", verb, value)
	}
	return parsed, nil
}

// number returns the next argument of a verb as a number.
func (a *arguments) number(verb string) (int, error) {
	value, err := a.take(verb)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("the verb %q wants a number, got %q", verb, value)
	}
	return parsed, nil
}

// apply runs the verbs in order.
func apply(verbs, payload []string) error {
	reader := &arguments{values: verbs}
	for reader.remaining() {
		verb := reader.next()
		switch verb {
		case "report":
			path, err := reader.take(verb)
			if err != nil {
				return err
			}
			if err := recordReport(path, payload); err != nil {
				return err
			}
		case "out", "err":
			text, err := reader.take(verb)
			if err != nil {
				return err
			}
			if verb == "out" {
				fmt.Println(text)
			} else {
				fmt.Fprintln(os.Stderr, text)
			}
		case "write", "append":
			path, err := reader.take(verb)
			if err != nil {
				return err
			}
			content, err := reader.take(verb)
			if err != nil {
				return err
			}
			if err := write(path, content, verb == "append"); err != nil {
				return err
			}
		case "sleep":
			duration, err := reader.duration(verb)
			if err != nil {
				return err
			}
			time.Sleep(duration)
		case "exit":
			code, err := reader.number(verb)
			if err != nil {
				return err
			}
			os.Exit(code)
		case "fail":
			path, err := reader.take(verb)
			if err != nil {
				return err
			}
			times, err := reader.number(verb)
			if err != nil {
				return err
			}
			code, err := reader.number(verb)
			if err != nil {
				return err
			}
			if err := failWhileCounting(path, times, code); err != nil {
				return err
			}
		case "spawn":
			path, err := reader.take(verb)
			if err != nil {
				return err
			}
			duration, err := reader.duration(verb)
			if err != nil {
				return err
			}
			if err := spawn(path, duration); err != nil {
				return err
			}
			time.Sleep(duration)
		case "ignore-term":
			duration, err := reader.duration(verb)
			if err != nil {
				return err
			}
			signal.Ignore(syscall.SIGTERM)
			time.Sleep(duration)
		default:
			return fmt.Errorf("unknown verb %q", verb)
		}
	}
	return nil
}

// recordReport writes what the process sees, as JSON. The payload is always
// written as a list, even when it is empty.
func recordReport(path string, payload []string) error {
	if payload == nil {
		payload = []string{}
	}

	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		environment[name] = value
	}

	directory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reporting the working directory: %w", err)
	}

	content, err := json.MarshalIndent(report{
		Argv:    os.Args,
		Payload: payload,
		Dir:     directory,
		PID:     os.Getpid(),
		PGID:    processGroup(),
		Env:     environment,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("recording the report: %w", err)
	}
	return replace(path, content)
}

// write creates or replaces a file, or appends to it.
func write(path, content string, append bool) error {
	if append {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		if _, err := file.WriteString(content); err != nil {
			return err
		}
		return nil
	}
	return replace(path, []byte(content))
}

// failWhileCounting fails while the attempt counter in path has not reached
// times, so that a job fails a predictable number of times and then succeeds.
// The counter is incremented before the decision is taken, which makes the
// first attempt fail.
func failWhileCounting(path string, times, code int) error {
	attempt := 0
	if content, err := os.ReadFile(path); err == nil {
		attempt, _ = strconv.Atoi(strings.TrimSpace(string(content)))
	}
	attempt++
	if err := replace(path, []byte(strconv.Itoa(attempt))); err != nil {
		return err
	}
	if attempt <= times {
		os.Exit(code)
	}
	return nil
}

// spawn starts another instance of this program, in the same process group, so
// that a test can check that stopping a job also stops what the job started.
func spawn(path string, duration time.Duration) error {
	child := exec.Command(os.Args[0], "report", path, "sleep", duration.String())
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	return child.Start()
}

// replace writes content to path through a temporary file, so that a reader
// sees either the previous content or the new one, never a half written file.
func replace(path string, content []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
