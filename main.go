package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var origTermios unix.Termios

const (
	HIDE_CURSOR         = "\x1B[?25l"
	SAVE_CURSOR         = "\x1B[s"
	SAVE_SCREEN         = "\x1B[?1047h"
	ENABLE_ALT_BUFFER   = "\x1B[?1049h"
	DISABLE_ALT_BUFFER  = "\x1B[?1049l"
	RESTORE_SCREEN      = "\x1B[?1047l"
	RESTORE_CURSOR      = "\x1B[u"
	SHOW_CURSOR         = "\x1B[?25h"
	CLEAR_SCREEN        = "\x1B[H\x1B[2J"
	SET_DIM_MODE        = "\x1B[2m"
	RESET_DIM_MODE      = "\x1B[22m"
	MOVE_CURSOR_BACK    = "\x1B[D"
	SET_COLOR_RED       = "\x1B[0;31m"
	SET_COLOR_GREEN     = "\x1B[0;32m"
	RESET_COLOR_STYLE   = "\x1B[0m"
	SET_STYLE_UNDERLINE = "\x1B[4m"

	CTRL_C  = 3
	BACKSPC = 127
	ENTER   = 13
	NIL     = 0
	SPACE   = 32

	TEXTBOX_WIDTH  = 50
	TEXTBOX_HEIGHT = 5
)

type Cursor struct {
	x int
	y int
}

type Char struct {
	value rune
	inp   rune
}

type Line []Char

type Page []Line

type Document struct {
	pages       []Page
	wordCount   int
	currentPage int
}

type Window struct {
	x       int
	y       int
	width   int
	height  int
	doc     Document
	cursor  Cursor
	showInp bool
}

func main() {
	err := enableRawMode()
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		disableRawMode()
		if r := recover(); r != nil {
			log.Println(r)
			buf := make([]byte, 1024)
			n := runtime.Stack(buf, false)
			fmt.Printf("Stack trace:\n%s\n", buf[:n])
		}
	}()

	windowWidth, windowHeight, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting terminal size: %v\n", err)
		return
	}
	windowX, windowY := windowWidth/2-TEXTBOX_WIDTH/2, windowHeight/2-TEXTBOX_HEIGHT/2

	text := "The quick brown fox jumps over the lazy dog The quick brown fox jumps over the lazy dog The quick brown fox jumps over the lazy dog The quick brown fox jumps over the lazy dog The quick brown fox jumps over the lazy dog The quick brown fox jumps over the lazy dog"
	doc := stringToDocument(text, TEXTBOX_WIDTH, TEXTBOX_HEIGHT)

	window := NewWindow(windowX, windowY, TEXTBOX_WIDTH, TEXTBOX_HEIGHT, doc)

	fmt.Print(SET_DIM_MODE)
	window.PrintCurrentPage()
	fmt.Print(RESET_DIM_MODE)

	isTyping := false
	var startTime time.Time
	var endTime time.Time
	for inp := userInp(); inp != CTRL_C; inp = userInp() {
		switch inp {
		case ENTER:
		case NIL:
		case BACKSPC:
			window.SetInpAtCursor(rune(0))
			window.CursorSub()
			fmt.Printf(SET_DIM_MODE+"%c"+RESET_DIM_MODE+MOVE_CURSOR_BACK, window.doc.pages[window.doc.currentPage][window.cursor.y][window.cursor.x].value)
		default:
			if !isTyping {
				isTyping = true
				startTime = time.Now()
			}
			window.SetInpAtCursor(rune(inp))
			cursorVal := window.RuneAtCursor()
			var showVal rune
			if window.showInp {
				showVal = rune(inp)
			} else {
				showVal = cursorVal
			}
			if rune(inp) == cursorVal {
				fmt.Printf(SET_COLOR_GREEN+"%c"+RESET_COLOR_STYLE, showVal)
			} else {
				if inp == SPACE && window.showInp {
					fmt.Printf(SET_COLOR_RED+SET_STYLE_UNDERLINE+"%c"+RESET_COLOR_STYLE, showVal)
				} else {
					fmt.Printf(SET_COLOR_RED+"%c"+RESET_COLOR_STYLE, showVal)
				}
			}
			window.CursorAdd()
		}
	}
}

func NewWindow(x, y, width, height int, doc Document) Window {
	return Window{
		x:       x,
		y:       y,
		width:   width,
		height:  height,
		doc:     doc,
		cursor:  Cursor{x: 0, y: 0},
		showInp: false,
	}
}

func (w *Window) PrintPage(pageNumber int) {
	fmt.Print(CLEAR_SCREEN)
	page := w.doc.pages[pageNumber]
	for i, line := range page {
		w.SetCursor(0, i)
		for _, c := range line {
			if c.inp == 0 {
				fmt.Printf("%c", c.value)
			} else {
				var char rune
				if w.showInp {
					char = c.inp
				} else {
					char = c.value
				}
				if c.value == c.inp {
					fmt.Printf(SET_COLOR_GREEN+"%c"+RESET_COLOR_STYLE, char)
				} else {
					if c.inp == SPACE && w.showInp {
						fmt.Printf(SET_COLOR_RED+SET_STYLE_UNDERLINE+"%c"+RESET_COLOR_STYLE, char)
					} else {
						fmt.Printf(SET_COLOR_RED+"%c"+RESET_COLOR_STYLE, char)
					}
				}
			}
		}
	}
	w.SetCursor(0, 0)
}

func (w *Window) CurrentPage() Page {
	return w.doc.pages[w.doc.currentPage]
}

func (w *Window) RuneAtCursor() rune {
	return w.doc.pages[w.doc.currentPage][w.cursor.y][w.cursor.x].value
}

func (w *Window) SetInpAtCursor(inp rune) {
	w.doc.pages[w.doc.currentPage][w.cursor.y][w.cursor.x].inp = inp
}

func (w *Window) PrintCurrentPage() {
	w.PrintPage(w.doc.currentPage)
}

func (w *Window) SetCursor(x, y int) {
	w.cursor.x = x
	w.cursor.y = y
	fmt.Printf("\x1B[%d;%dH", w.y+w.cursor.y, w.x+w.cursor.x)
}

func (w *Window) CursorAdd() {
	cursorX, cursorY := w.cursor.x, w.cursor.y
	w.cursor.x++
	if w.cursor.x == len(w.doc.pages[w.doc.currentPage][w.cursor.y]) {
		w.cursor.x = 0
		w.cursor.y++
		if w.cursor.y == len(w.doc.pages[w.doc.currentPage]) {
			w.cursor.x = 0
			w.cursor.y = 0
			w.doc.currentPage++
			if w.doc.currentPage == len(w.doc.pages) {
				w.cursor.x = cursorX
				w.cursor.y = cursorY
				w.doc.currentPage--
			} else {
				fmt.Print(SET_DIM_MODE)
				w.PrintCurrentPage()
				fmt.Print(RESET_DIM_MODE)
			}
		}
	}
	fmt.Printf("\x1B[%d;%dH", w.y+w.cursor.y, w.x+w.cursor.x)
}

func (w *Window) CursorSub() {
	w.cursor.x--
	if w.cursor.x < 0 {
		if w.cursor.y-1 >= 0 {
			w.cursor.y--
			w.cursor.x = len(w.doc.pages[w.doc.currentPage][w.cursor.y]) - 1
		} else {
			if w.doc.currentPage > 0 {
				w.doc.currentPage--
				w.PrintCurrentPage()
				w.cursor.y = len(w.doc.pages[w.doc.currentPage]) - 1
				w.cursor.x = len(w.doc.pages[w.doc.currentPage][w.cursor.y]) - 1
			} else {
				w.cursor.x = 0
			}
		}
	}
	fmt.Printf("\x1B[%d;%dH", w.y+w.cursor.y, w.x+w.cursor.x)
}

func enableRawMode() error {
	err := termios.Tcgetattr(os.Stderr.Fd(), &origTermios)
	if err != nil {
		return err
	}

	raw := origTermios
	raw.Iflag &= ^uint32(unix.IXOFF | unix.ICRNL | unix.BRKINT | unix.INPCK | unix.ISTRIP)
	raw.Oflag &= ^uint32(unix.OPOST)
	raw.Cflag |= uint32(unix.CS8)
	raw.Lflag &= ^uint32(unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN)

	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1

	// fmt.Printf(HIDE_CURSOR)
	fmt.Printf(SAVE_CURSOR)
	fmt.Printf(SAVE_SCREEN)
	// fmt.Printf(ENABLE_ALT_BUFFER)

	return termios.Tcsetattr(os.Stderr.Fd(), unix.TCSAFLUSH, &raw)
}

func disableRawMode() {
	// fmt.Printf(DISABLE_ALT_BUFFER)
	fmt.Printf(RESTORE_SCREEN)
	fmt.Printf(RESTORE_CURSOR)
	// fmt.Printf(SHOW_CURSOR)

	termios.Tcsetattr(os.Stdin.Fd(), unix.TCSAFLUSH, &origTermios)
}

func userInp() byte {
	inp := make([]byte, 1)
	_, err := os.Stdin.Read(inp)
	if err != nil && err != io.EOF {
		log.Fatal(err)
	}
	return inp[0]
}

func stringToDocument(str string, cols int, rows int) Document {
	var doc Document
	wordStart := 0
	pi := 0
	li := 0
	ci := 0
	var line Line
	var page Page
	for i, c := range str {
		if c == ' ' {
			doc.wordCount++
			if i-wordStart > cols-ci {
				ci = 0
				if line[len(line)-1].value == ' ' {
					page = append(page, line[:len(line)-1])
				} else {
					page = append(page, line)
				}
				line = Line{}
				li++
				if li >= rows {
					li = 0
					doc.pages = append(doc.pages, page)
					page = Page{}
					pi++
				}
			}
			for x := wordStart; x < i; x++ {
				line = append(line, Char{value: rune(str[x])})
				ci++
			}
			line = append(line, Char{value: ' '})
			ci++
			wordStart = i + 1
		}
		if ci >= cols {
			ci = 0
			if line[len(line)-1].value == ' ' {
				page = append(page, line[:len(line)-1])
			} else {
				page = append(page, line)
			}
			line = Line{}
			li++
		}
		if li >= rows {
			li = 0
			doc.pages = append(doc.pages, page)
			page = Page{}
			pi++
		}
	}

	for x := len(str) - 1; x > 0; x-- {
		if str[x] == ' ' {
			wordStart = x + 1
			break
		}
	}
	for x := wordStart; x < len(str); x++ {
		line = append(line, Char{value: rune(str[x])})
	}
	doc.wordCount++

	page = append(page, line)
	doc.pages = append(doc.pages, page)

	return doc
}
