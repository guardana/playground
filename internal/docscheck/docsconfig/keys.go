package docsconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// frame is one open object or array while the token stream is walked.
type frame struct {
	object bool
	seen   map[string]bool
	key    bool
}

// checkKeysOnce walks the token stream and refuses an object that names a key
// twice, which encoding/json would otherwise read as the last one.
func checkKeysOnce(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var stack []*frame
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if top := last(stack); top != nil && top.object && top.key && tok != json.Delim('}') {
			if err := top.readKey(tok); err != nil {
				return err
			}
			continue
		}
		stack, err = step(stack, tok)
		if err != nil {
			return err
		}
	}
}

func (f *frame) readKey(tok json.Token) error {
	key, ok := tok.(string)
	if !ok {
		return fmt.Errorf("an object key is not a string: %v", tok)
	}
	if f.seen[key] {
		return fmt.Errorf("key %q is given twice in one object", key)
	}
	f.seen[key] = true
	f.key = false
	return nil
}

// step consumes one value token: a delimiter opens or closes a frame, and
// after any value an enclosing object expects a key next.
func step(stack []*frame, tok json.Token) ([]*frame, error) {
	if d, ok := tok.(json.Delim); ok {
		switch d {
		case '{':
			return append(stack, &frame{object: true, seen: map[string]bool{}, key: true}), nil
		case '[':
			return append(stack, &frame{}), nil
		default:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unexpected %v", d)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if top := last(stack); top != nil && top.object {
		top.key = true
	}
	return stack, nil
}

func last(stack []*frame) *frame {
	if len(stack) == 0 {
		return nil
	}
	return stack[len(stack)-1]
}
