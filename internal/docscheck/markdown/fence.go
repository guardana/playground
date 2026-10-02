// Package markdown reads the parts of a page the checks hold to a rule:
// fenced blocks, the headings GitHub renders and the anchors it gives them,
// and the links that name one of those anchors.
package markdown

import "strings"

// Fence is the opening line of a fenced block: its character, how many of
// them opened it, and the info string after them.
type Fence struct {
	Char byte
	Len  int
	Info string
}

// OpenFence reads line as the opening of a fenced block: three or more
// backticks or tildes, and for backticks an info string holding none.
func OpenFence(line string) (Fence, bool) {
	line = strings.TrimSpace(line)
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return Fence{}, false
	}
	n := len(line) - len(strings.TrimLeft(line, line[:1]))
	if n < 3 {
		return Fence{}, false
	}
	info := strings.TrimSpace(line[n:])
	if line[0] == '`' && strings.Contains(info, "`") {
		return Fence{}, false
	}
	return Fence{Char: line[0], Len: n, Info: info}, true
}

// Closes reports whether line ends the block f opened: the same character,
// at least as many of them, and nothing else.
func (f Fence) Closes(line string) bool {
	line = strings.TrimSpace(line)
	return len(line) >= f.Len && strings.Trim(line, string(f.Char)) == ""
}

// outsideFences calls visit with each line outside a fenced block and its
// one-based number.
func outsideFences(body []byte, visit func(number int, line string)) {
	var open *Fence
	for i, line := range strings.Split(string(body), "\n") {
		switch {
		case open != nil:
			if open.Closes(line) {
				open = nil
			}
		default:
			if f, ok := OpenFence(line); ok {
				open = &f
				continue
			}
			visit(i+1, line)
		}
	}
}
