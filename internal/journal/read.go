package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/guardana/playground/internal/runfile"
)

// ErrUnknownLine reports a line this reader cannot place: a newer version, a
// kind it does not know, or a field an auth line does not have. It is refused
// rather than skipped, because a skipped line could be a call.
var ErrUnknownLine = errors.New("journal: a line this reader does not know")

// lines is one journal, by kind.
type lines struct {
	calls []Entry
	auth  []Auth
}

// ReadFile reads the calls in one server's journal; its auth lines are read
// by ReadAuthFile and are never calls. A journal that does not exist is an
// error and never an empty one: a server that never started would otherwise
// look exactly like a server that served nothing, and those are the two answers
// a denial scenario has to tell apart. The journal sits where its server can
// write, so a link, a FIFO or a file past MaxFileBytes there is refused.
func ReadFile(path string) ([]Entry, error) {
	read, err := readLines(path)
	if err != nil {
		return nil, err
	}
	return read.calls, nil
}

// ReadAuthFile reads the auth lines in one server's journal, refusing the
// journal on the same terms as ReadFile.
func ReadAuthFile(path string) ([]Auth, error) {
	read, err := readLines(path)
	if err != nil {
		return nil, err
	}
	return read.auth, nil
}

func readLines(path string) (lines, error) {
	body, err := runfile.ReadRegular(path, MaxFileBytes)
	if err != nil {
		return lines{}, err
	}

	var read lines
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64<<10), MaxLineBytes)
	line := 1
	for ; scanner.Scan(); line++ {
		if err := read.add(scanner.Bytes()); err != nil {
			return lines{}, fmt.Errorf("%s line %d: %w", path, line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			// Named, because the caller's next move is to open that line, and
			// "token too long" says neither which line nor how long is too long.
			return lines{}, fmt.Errorf("%s line %d: %w: limit %d bytes", path, line, ErrLineTooLong, MaxLineBytes)
		}
		return lines{}, fmt.Errorf("%s line %d: %w", path, line, err)
	}
	return read, nil
}

// add places one line by the version and kind it states.
func (l *lines) add(raw []byte) error {
	var head struct {
		Version int    `json:"version"`
		Kind    string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return err
	}
	if head.Version < 0 || head.Version > Version {
		return fmt.Errorf("%w: version %d, this reader knows %d and older", ErrUnknownLine, head.Version, Version)
	}
	switch head.Kind {
	case "":
		entry, err := decodeCall(raw)
		if err != nil {
			return err
		}
		l.calls = append(l.calls, entry)
	case KindAuth:
		if head.Version < Version {
			return fmt.Errorf("%w: an auth line states version %d, want %d", ErrUnknownLine, head.Version, Version)
		}
		auth, err := decodeAuth(raw)
		if err != nil {
			return err
		}
		l.auth = append(l.auth, auth)
	default:
		return fmt.Errorf("%w: kind %q", ErrUnknownLine, head.Kind)
	}
	return nil
}

func decodeCall(raw []byte) (Entry, error) {
	var entry Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return Entry{}, err
	}
	// An effect written as {} would decode to an empty map and read as none
	// at all; the writer never emits one.
	if entry.Effect != nil {
		if err := entry.Effect.Validate(); err != nil {
			return Entry{}, err
		}
	}
	return entry, nil
}

// decodeAuth decodes strictly: a field an auth line does not have is a newer
// format, and a value the writer would refuse is a line it did not write.
func decodeAuth(raw []byte) (Auth, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var line authLine
	if err := decoder.Decode(&line); err != nil {
		return Auth{}, fmt.Errorf("%w: %w", ErrUnknownLine, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Auth{}, fmt.Errorf("%w: data after the auth line", ErrUnknownLine)
	}
	if err := line.validate(); err != nil {
		return Auth{}, err
	}
	return line.Auth, nil
}
