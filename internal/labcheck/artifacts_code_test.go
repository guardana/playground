package labcheck_test

import (
	"encoding/json"
	"path"
	"strings"
)

// hubFields are the config keys whose repository the loader fetches and runs.
var hubFields = []string{"_attn_implementation", "_attn_implementation_internal", "auto_map"}

// hubRepoProblems refuses a config naming, in a field the loader fetches code
// from, a repository the hub client would accept: a name with "--" or ".." is
// refused by the client before any request, so nobody can publish one.
func hubRepoProblems(data []byte) []string {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return []string{"not JSON: " + err.Error()}
	}
	var problems []string
	for _, value := range hubValues(document, false) {
		if repo := hubRepo(value); strings.Contains(repo, "/") && !strings.Contains(repo, "--") && !strings.Contains(repo, "..") {
			problems = append(problems, "names "+value+", a repository the hub client would fetch")
		}
	}
	return problems
}

// hubRepo is the repository a hub field's value names: after a "prefix|",
// before "--module.Class" as auto_map writes it, and before "@revision" or
// ":function".
func hubRepo(value string) string {
	if at := strings.LastIndex(value, "|"); at >= 0 {
		value = value[at+1:]
	}
	if at := strings.Index(value, "--"); at >= 0 && strings.Contains(value[:at], "/") {
		value = value[:at]
	}
	if fields := strings.FieldsFunc(value, func(r rune) bool { return r == '@' || r == ':' }); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// hubValues lists every string under a hub field, at any depth.
func hubValues(value any, under bool) []string {
	switch typed := value.(type) {
	case string:
		if under {
			return []string{typed}
		}
	case []any:
		var found []string
		for _, item := range typed {
			found = append(found, hubValues(item, under)...)
		}
		return found
	case map[string]any:
		var found []string
		for key, item := range typed {
			found = append(found, hubValues(item, under || isHubField(key))...)
		}
		return found
	}
	return nil
}

func isHubField(key string) bool {
	for _, field := range hubFields {
		if key == field {
			return true
		}
	}
	return false
}

// toolRunNames are Python files pytest, pip, site or an IDE run from a
// checkout without anyone calling them.
var toolRunNames = []string{"conftest.py", "setup.py", "__init__.py", "sitecustomize.py", "usercustomize.py", "noxfile.py", "__main__.py"}

// pythonProblems refuses a name tooling runs, and a statement at module level
// that is not a function, a docstring or a comment: the artifact's code may
// only sit in functions nothing calls.
func pythonProblems(name string, data []byte) []string {
	base := path.Base(name)
	for _, run := range toolRunNames {
		if base == run {
			return []string{base + " is a name tooling runs"}
		}
	}
	if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
		return []string{base + " is a name tooling runs"}
	}
	var problems []string
	inDocstring := false
	for number, line := range strings.Split(string(data), "\n") {
		switch {
		case inDocstring:
			inDocstring = !strings.Contains(line, `"""`)
		case strings.HasPrefix(line, `"""`):
			inDocstring = strings.Count(line, `"""`) == 1
		case line == "", strings.HasPrefix(line, " "), strings.HasPrefix(line, "#"), strings.HasPrefix(line, "def "):
		default:
			problems = append(problems, "line "+itoa(number+1)+" runs on import")
		}
	}
	return problems
}

func itoa(n int) string {
	digits := ""
	for {
		digits = string(rune('0'+n%10)) + digits
		if n /= 10; n == 0 {
			return digits
		}
	}
}
