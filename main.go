package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/encoding"
)

type Note struct {
	Name     string
	Body     string
	Modified time.Time
}

var (
	termEvents                  = make(chan tcell.Event, 500)
	screen         tcell.Screen = nil
	screenWidth                 = 0
	screenHeight                = 0
	search                      = ""
	searchSelected              = false
	selectedIndex               = -1
	extension                   = ".md"
	notes                       = []*Note{}
	results                     = []*Note{}
)

func main() {
	if len(os.Args) >= 2 {
		extension = "." + os.Args[1]
	}

	defer func() {
		if err := recover(); err != nil {
			fatal(fmt.Sprintf("nvterm fatal error:\n%v\n%s", err, string(debug.Stack())))
		}
	}()

	var err error
	screen, err = tcell.NewScreen()
	fatalError(err)
	err = screen.Init()
	fatalError(err)

	encoding.Register()
	tcell.SetEncodingFallback(tcell.EncodingFallbackASCII)

	screen.SetStyle(tcell.StyleDefault)
	screen.Clear()

	screenWidth, screenHeight = screen.Size()

	go func() {
		for {
			if screen == nil {
				time.Sleep(50 * time.Millisecond)
			}
			termEvents <- screen.PollEvent()
		}
	}()

	updateNotes()
	updateResults()

top:
	for {
		render()
		select {
		case ev := <-termEvents:
			switch ev := ev.(type) {
			case *tcell.EventKey:
				if ev.Key() == tcell.KeyCtrlQ || ev.Key() == tcell.KeyCtrlG {
					screen.Fini()
					screen = nil
					break top
				} else if ev.Key() == tcell.KeyBackspace2 {
					search = search[:max(0, len(search)-1)]
					updateResults()
				} else if ev.Key() == tcell.KeyRune {
					if searchSelected {
						searchSelected = false
						search = ""
					}
					search = search + string(ev.Rune())
					updateResults()
				} else if ev.Key() == tcell.KeyRight {
					searchSelected = false
				} else if ev.Key() == tcell.KeyCtrlL {
					searchSelected = true
				} else if ev.Key() == tcell.KeyCtrlK || ev.Key() == tcell.KeyCtrlP {
					selectedIndex = max(selectedIndex-1, -1)
				} else if ev.Key() == tcell.KeyCtrlJ || ev.Key() == tcell.KeyCtrlN {
					selectedIndex = min(selectedIndex+1, len(results)-1)
				} else if ev.Key() == tcell.KeyEnter {
					var filename string
					if selectedIndex == -1 {
						filename = search + extension
					} else {
						filename = results[selectedIndex].Name + extension
					}
					screen.Fini()
					screen = nil

					cmd := exec.Command(Env("EDITOR", "vim"), filename)
					cmd.Stdin = os.Stdin
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					cmd.Start()
					cmd.Wait()

					screen, err = tcell.NewScreen()
					fatalError(err)
					fatalError(screen.Init())
					screen.SetStyle(tcell.StyleDefault)
					screen.Clear()
					screenWidth, screenHeight = screen.Size()
					updateResults()
				} else {
					// search += ev.Name()
				}
			case *tcell.EventResize:
				screenWidth, screenHeight = screen.Size()
			}
		}
	}
}

func updateNotes() {
	entries, err := os.ReadDir(".")
	fatalError(err)
	for _, e := range entries {
		if e.IsDir() || e.Name()[0] == '.' || !strings.HasSuffix(e.Name(), extension) {
			continue
		}
		bs, err := os.ReadFile(e.Name())
		fatalError(err)
		info, err := os.Stat(e.Name())
		fatalError(err)
		notes = append(notes, &Note{
			Name:     e.Name()[:len(e.Name())-len(extension)],
			Body:     string(bs),
			Modified: info.ModTime(),
		})
	}
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Modified.After(notes[j].Modified)
	})
}

func updateResults() {
	if search == "" {
		results = notes
		return
	}
	results = []*Note{}
	parts := strings.Split(search, " ")
	for _, n := range notes {
		match := false
		for _, p := range parts {
			if strings.Contains(n.Name, p) || strings.Contains(n.Body, p) {
				match = true
				break
			}
		}
		if match {
			results = append(results, n)
		}
	}
	selectedIndex = min(selectedIndex, len(results)-1)
	if selectedIndex == -1 && len(results) > 0 {
		selectedIndex = 0
	}
}

func render() {
	screen.Clear()
	write(0, 0, search, tcell.StyleDefault)

	// results
	write(0, 1, padr("", screenWidth, '-'), tcell.StyleDefault)
	for i, n := range results {
		if i >= screenHeight/2-3 {
			break
		}
		if selectedIndex == i {
			write(0, 2+i, n.Name, tcell.StyleDefault.Reverse(true))
		} else {
			write(0, 2+i, n.Name, tcell.StyleDefault)
		}
	}

	// contents
	write(0, screenHeight/2, padr("", screenWidth, '-'), tcell.StyleDefault)
	if selectedIndex > -1 {
		note := results[selectedIndex]
		lines := strings.Split(note.Body, "\n")
		for i := 0; i < screenHeight/2-1 && i < len(lines)-1; i++ {
			write(0, screenHeight/2+1+i, lines[i], tcell.StyleDefault)
		}
	}

	// cursor
	if searchSelected {
		screen.ShowCursor(0, 0)
	} else {
		screen.ShowCursor(len(search), 0)
	}
	screen.Show()
}

func write(x, y int, str string, style tcell.Style) int {
	for i, r := range str {
		screen.SetContent(x+i, y, r, nil, style)
	}
	return utf8.RuneCountInString(str)
}

func padr(str string, length int, padding rune) string {
	for utf8.RuneCountInString(str) < length {
		str = str + string(padding)
	}
	return str
}

func padl(str string, length int, padding rune) string {
	for utf8.RuneCountInString(str) < length {
		str = string(padding) + str
	}
	return str
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func fatalError(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) {
	if screen != nil {
		screen.Fini()
		screen = nil
	}
	fmt.Printf("%v\n", message)
	os.Exit(1)
}

func Env(k, alt string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return alt
}
