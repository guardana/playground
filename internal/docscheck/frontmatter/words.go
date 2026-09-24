package frontmatter

import (
	"strings"
	"unicode"
)

// Words counts the words of a body: the white-space separated tokens that
// hold a letter or a digit, outside fenced code blocks. A token of
// punctuation alone, such as a table's pipe, is not a word.
func Words(body []byte) int {
	n := 0
	fenced := false
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, token := range strings.Fields(line) {
			if strings.ContainsFunc(token, isWordRune) {
				n++
			}
		}
	}
	return n
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
