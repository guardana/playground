package labcheck_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// absentModule is the one module a lab pickle may import. Nothing provides it,
// so loading such a pickle raises on the import before anything could run.
const absentModule = "clab_never_a_module"

// pickleArg is how an opcode's argument is laid out: newline-terminated lines,
// a fixed number of bytes, or a little-endian length of sized bytes followed
// by that many bytes.
type pickleArg struct{ lines, fixed, sized int }

// allowedOpcodes are the opcodes a lab pickle may hold. Every opcode that
// calls, builds, imports by another route or asks the loader for an object is
// absent, so the walker refuses it like any byte it does not know.
func allowedOpcodes() map[byte]pickleArg {
	none, line, global := pickleArg{}, pickleArg{lines: 1}, pickleArg{lines: 2}
	return map[byte]pickleArg{
		'(': none, '.': none, '0': none, '1': none, '2': none, 'N': none,
		'a': none, 'd': none, '}': none, 'e': none, 'l': none, ']': none,
		's': none, 't': none, ')': none, 'u': none,
		0x85: none, 0x86: none, 0x87: none, 0x88: none, 0x89: none,
		0x8f: none, 0x90: none, 0x91: none, 0x94: none,
		'F': line, 'I': line, 'L': line, 'S': line, 'V': line, 'g': line, 'p': line,
		'c': global,
		'K': {fixed: 1}, 'h': {fixed: 1}, 'q': {fixed: 1}, 0x80: {fixed: 1},
		'M': {fixed: 2}, 'J': {fixed: 4}, 'j': {fixed: 4}, 'r': {fixed: 4},
		'G': {fixed: 8}, 0x95: {fixed: 8},
		'U': {sized: 1}, 'C': {sized: 1}, 0x8a: {sized: 1}, 0x8c: {sized: 1},
		'T': {sized: 4}, 'X': {sized: 4}, 'B': {sized: 4}, 0x8b: {sized: 4},
		0x8d: {sized: 8}, 0x8e: {sized: 8}, 0x96: {sized: 8},
	}
}

func opcodeName(op byte) string {
	names := map[byte]string{
		'R': "REDUCE", 'b': "BUILD", 'i': "INST", 'o': "OBJ", 'P': "PERSID", 'Q': "BINPERSID",
		0x81: "NEWOBJ", 0x82: "EXT1", 0x83: "EXT2", 0x84: "EXT4", 0x92: "NEWOBJ_EX",
		0x93: "STACK_GLOBAL", 0x97: "NEXT_BUFFER", 0x98: "READONLY_BUFFER",
	}
	if name, ok := names[op]; ok {
		return fmt.Sprintf("%s (0x%02x)", name, op)
	}
	return fmt.Sprintf("0x%02x", op)
}

// checkPickle walks a pickle opcode by opcode without loading it. It accepts
// exactly one pickle of allowed opcodes whose every GLOBAL imports the absent
// module, ending at STOP with nothing after it.
func checkPickle(data []byte) error {
	allowed := allowedOpcodes()
	for pos := 0; pos < len(data); {
		op := data[pos]
		arg, ok := allowed[op]
		if !ok {
			return fmt.Errorf("offset %d: opcode %s is not allowed", pos, opcodeName(op))
		}
		next, err := skipArgument(data, pos+1, arg)
		if err != nil {
			return fmt.Errorf("offset %d: opcode %s: %w", pos, opcodeName(op), err)
		}
		if op == 'c' {
			module, _, _ := bytes.Cut(data[pos+1:], []byte("\n"))
			if string(module) != absentModule {
				return fmt.Errorf("offset %d: GLOBAL imports %q, not %s", pos, module, absentModule)
			}
		}
		if op == '.' {
			if next != len(data) {
				return fmt.Errorf("offset %d: %d bytes after STOP", pos, len(data)-next)
			}
			return nil
		}
		pos = next
	}
	return errors.New("no STOP opcode")
}

func skipArgument(data []byte, pos int, arg pickleArg) (int, error) {
	for range arg.lines {
		end := bytes.IndexByte(data[pos:], '\n')
		if end < 0 {
			return 0, errors.New("argument has no line end")
		}
		pos += end + 1
	}
	pos, err := skipBytes(data, pos, arg.fixed)
	if err != nil || arg.sized == 0 {
		return pos, err
	}
	sizeAt := pos
	if pos, err = skipBytes(data, pos, arg.sized); err != nil {
		return 0, err
	}
	size := 0
	for i := arg.sized - 1; i >= 0; i-- {
		if size > len(data) {
			return 0, errors.New("argument length runs past the end")
		}
		size = size<<8 | int(data[sizeAt+i])
	}
	return skipBytes(data, pos, size)
}

func skipBytes(data []byte, pos, n int) (int, error) {
	if n > len(data)-pos {
		return 0, fmt.Errorf("argument of %d bytes runs past the end", n)
	}
	return pos + n, nil
}

func TestThePickleWalkerRefusesEveryOpcodeThatCallsOrImports(t *testing.T) {
	for _, op := range []byte{'R', 'b', 'i', 'o', 'P', 'Q', 0x81, 0x82, 0x83, 0x84, 0x92, 0x93, 0x97, 0x98} {
		err := checkPickle([]byte{'(', op, '.'})
		if err == nil || !strings.Contains(err.Error(), "is not allowed") {
			t.Errorf("opcode 0x%02x: got %v, want it refused", op, err)
		}
	}
}

func TestThePickleWalkerTakesAGlobalIntoTheAbsentModuleOnly(t *testing.T) {
	if err := checkPickle([]byte("c" + absentModule + "\nnothing\n.")); err != nil {
		t.Errorf("the absent module: %v", err)
	}
	for _, module := range []string{"o" + "s", "built" + "ins", absentModule + "_too", ""} {
		err := checkPickle([]byte("c" + module + "\nsystem\n."))
		if err == nil || !strings.Contains(err.Error(), "GLOBAL imports") {
			t.Errorf("module %q: got %v, want it refused", module, err)
		}
	}
}

// Each refusal names its reason, so a guard removed is not hidden behind a
// later refusal of the same bytes for another reason.
func TestThePickleWalkerRefusesWhatItCannotParse(t *testing.T) {
	cases := map[string]struct{ data, want string }{
		"empty":                     {"", "no STOP"},
		"no STOP":                   {"(d", "no STOP"},
		"GLOBAL without a line end": {"c" + absentModule, "no line end"},
		"a length past the end":     {"X\x10\x00\x00\x00ab.", "16 bytes runs past the end"},
		"a length with the top bit": {"\x8e\xf8\xff\xff\xff\xff\xff\xff\xff.", "length runs past the end"},
		"a truncated length":        {"X\x01\x00", "4 bytes runs past the end"},
		"bytes after STOP":          {"(d..", "1 bytes after STOP"},
		"an unknown opcode":         {"(\xff.", "0xff is not allowed"},
	}
	for name, c := range cases {
		err := checkPickle([]byte(c.data))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want a refusal naming %q", name, err, c.want)
		}
	}
}

// The binary protocols' bytes are what pickle.dumps writes for {}, {'a': 1}
// and bytearray(b'ab') at protocols 2, 4 and 5.
func TestThePickleWalkerReadsTheBinaryProtocols(t *testing.T) {
	for _, data := range []string{
		"\x80\x02}q\x00.",
		"\x80\x04\x95\x0a\x00\x00\x00\x00\x00\x00\x00}\x94\x8c\x01a\x94K\x01s.",
		"\x80\x05\x96\x02\x00\x00\x00\x00\x00\x00\x00ab\x94.",
		"(dp0\n.",
	} {
		if err := checkPickle([]byte(data)); err != nil {
			t.Errorf("%q: %v", data, err)
		}
	}
}
