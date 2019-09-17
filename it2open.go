// Command it2open runs commands in new iTerm2 splits.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	flagCols   = flag.Int("cols", 4, "number of columns")
	flagNewTab = flag.Bool("tab", true, "if true, open a new tab")
	flagDebug  = flag.Bool("debug", false, "if true, dump out applescript instead of running it")
	flagDelay  = flag.Float64("delay", 0.25, "delay in seconds")
)

const tmpl = `
tell application "iTerm2"
  tell current window
    {% if .NewTab %}
    create tab with default profile
    {% else %}
    # use current tab
    {% end %}
    set panes to {}
    {% range .Cmds -%}
    set panes to panes & {{cmd:"{% . %}"}}
    {% end -%}
    set layout to {}
    {% range .LayoutCmds -%}
    set layout to layout & {{"{% . %}"}}
    {% end -%}
    #repeat with i from 1 to n
    repeat with currentLayout in items of layout
      tell application "System Events" to keystroke currentLayout using command down
    end repeat
    delay {% .Delay %}
    repeat with currentPane in items of panes
      tell the current session
        delay 0.25
        write text cmd of currentPane
        tell application "System Events" to keystroke "]" using command down
      end tell
    end repeat
  end tell
end tell
`

func main() {
	flag.Parse()
	if err := run(*flagCols, *flagNewTab, *flagDelay, *flagDebug); err != nil {
		fmt.Fprintf(os.Stderr, "it2open: %v\n", err)
		os.Exit(1)
	}
}

func run(cols int, newTab bool, delay float64, debug bool) error {
	var t = template.Must(template.New("applescript-template").Delims("{%", "%}").Parse(tmpl))
	cmds, err := splitStdin()
	if err != nil {
		return errors.Wrap(err, "reading stdin")
	}
	rows := int(math.Ceil(float64(len(cmds)) / float64(cols)))
	if len(cmds) < cols {
		cols = len(cmds)
	}
	ctx := struct {
		Cols       int
		Rows       int
		Delay      float64
		Cmds       []string
		LayoutCmds []string
		NewTab     bool
	}{
		Cols:       cols,
		Rows:       rows,
		Delay:      delay,
		Cmds:       cmds,
		LayoutCmds: mkLayoutCmds(cols, rows),
		NewTab:     newTab,
	}

	buf := new(bytes.Buffer)
	if debug {
		fmt.Printf("%+v\n", ctx)
	}
	if err := t.Execute(buf, ctx); err != nil {
		return err
	}
	if debug {
		io.Copy(os.Stdout, buf)
		return nil
	}
	tempFile, err := ioutil.TempFile("", "it2open")
	if err != nil {
		return errors.Wrap(err, "creating tempFile")
	}
	defer os.Remove(tempFile.Name()) // clean up
	if _, err := io.Copy(tempFile, buf); err != nil {
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

func splitStdin() ([]string, error) {
	if terminal.IsTerminal(0) {
		return nil, fmt.Errorf("expecting lines on stdin")
	}
	result := []string{}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		result = append(result, s.Text())
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func mkLayoutCmds(cols, rows int) []string {
	result := []string{}
	for i := 0; i < cols; i++ {
		if i < cols-1 {
			result = append(result, "d[")
		}
		for j := 0; j < rows-1; j++ {
			result = append(result, "D")
		}
		result = append(result, "]")
	}
	return result
}
