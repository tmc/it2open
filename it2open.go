// Command it2open runs commands in new iTerm2 splits.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"text/template"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	flagCols   = flag.Int("cols", 4, "number of columns")
	flagNewTab = flag.Bool("tab", true, "if true, open a new tab")
	flagDebug  = flag.Bool("debug", false, "if true, dump out applescript instead of running it")
	flagDelay  = flag.Float64("delay", 0.25, "delay in seconds")
)

func main() {
	flag.Parse()
	if err := run(*flagCols, *flagNewTab, *flagDelay, *flagDebug); err != nil {
		fmt.Fprintf(os.Stderr, "it2open: %v\n", err)
		os.Exit(1)
	}
}

func run(cols int, newTab bool, delay float64, debug bool) error {
	cmds, err := splitStdin()
	if err != nil {
		return errors.Wrap(err, "reading stdin")
	}

	rows := (len(cmds) + cols - 1) / cols

	ctx := struct {
		Cols   int
		Rows   int
		Delay  float64
		Cmds   []string
		NewTab bool
	}{
		Cols:   cols,
		Rows:   rows,
		Delay:  delay,
		Cmds:   cmds,
		NewTab: newTab,
	}

	tmpl, err := template.New("applescript-template").
		Funcs(funcMap).
		Parse(appleScriptTemplate)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	if debug {
		fmt.Printf("%+v\n", ctx)
	}
	if err := tmpl.Execute(buf, ctx); err != nil {
		return err
	}
	if debug {
		io.Copy(os.Stdout, buf)
		return nil
	}
	return runAppleScript(buf)
}

func splitStdin() ([]string, error) {
	if terminal.IsTerminal(0) {
		return nil, fmt.Errorf("expecting lines on stdin")
	}
	lines := []string{}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	return lines, s.Err()
}

func runAppleScript(script *bytes.Buffer) error {
	tempFile, err := ioutil.TempFile("", "it2open")
	if err != nil {
		return errors.Wrap(err, "creating tempFile")
	}
	defer os.Remove(tempFile.Name()) // clean up

	if _, err := io.Copy(tempFile, script); err != nil {
		return errors.Wrap(err, "copy to tempFile")
	}

	if err := tempFile.Close(); err != nil {
		log.Fatal(err)
		return errors.Wrap(err, "closing")
	}

	cmd := exec.Command("osascript", tempFile.Name())
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return errors.Wrap(cmd.Run(), "running osascript")
}

// func map:
var funcMap = template.FuncMap{
	"sub":  func(a, b int) int { return a - b },
	"mod":  func(a, b int) int { return a % b },
	"mul":  func(a, b int) int { return a * b },
	"add":  func(a, b int) int { return a + b },
	"ceil": func(a float64) int { return int(math.Ceil(a)) },
	"div":  func(a, b int) float64 { return float64(a) / float64(b) },
	"until": func(n int) []int {
		r := make([]int, n)
		for i := range r {
			r[i] = i
		}
		return r
	},
}

const appleScriptTemplate = `
tell application "iTerm2"
	{{ if .NewTab }}tell current window to create tab with default profile{{ end }}
	{{- $cols := .Cols }}
	{{- $cmds := .Cmds }}
	{{- range $i, $cmd := $cmds }}
		{{- if lt $i $cols }} {{/* For first $cols commands, create vertical splits */}}
			{{- if gt $i 0 }} {{/* Skip split before first command */}}
			# virt
				tell current session of current tab of current window to split vertically with default profile
				tell application "System Events" to keystroke "]" using {command down}
				delay {{ $.Delay }}
			{{- end }}
		{{- else }} {{/* For every $cols commands thereafter, go to first column and create horizontal split */}}
			delay {{ $.Delay }}
			# tab through all the existing splits in the new column
			{{ range until (sub $i $cols) }}
			tell application "System Events" to keystroke "]" using {command down}
			{{- end }}
			delay {{ $.Delay }}
			tell current session of current tab of current window to split horizontally with default profile
			delay {{ $.Delay }}
		{{- end }}
		tell current session of current tab of current window to write text "{{ $cmd }}"
	{{- end }}
end tell
`
