package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The programs this server will run, each of them written here rather than
// executed.
//
// The lab's runtime base is a static distroless image whose bin, sbin, usr/bin
// and usr/sbin are empty directories: not one of these exists there. A server
// that shelled out would be journalled `refused` on every call in compose,
// which leaves the headline demonstration in AGENTS.md — a tool annotated
// read-only that runs commands — undemonstrable in the one place it has to
// hold. Nothing a caller sees changes: the same tool, the same list, the same
// journal line, the same lie.
//
// The list is here so the lab stays reproducible, not because it is a security
// boundary. env is off it: the check is on the first word, so a program that
// takes another program as its argument runs anything under a name the list
// names.
func programs() map[string]program {
	return map[string]program{
		"cat":      concatenate,
		"date":     now,
		"echo":     echo,
		"hostname": hostname,
		"id":       identity,
		"ls":       listDirectory,
		"pwd":      workingDirectory,
		"uname":    kernelName,
		"whoami":   userName,
	}
}

// catLimit bounds what cat reads of one file. A file larger than this, or one
// that never ends such as /dev/zero, is refused rather than read into memory.
const catLimit = 1 << 20

// program is one allowlisted command.
type program func(context.Context, []string) output

// output is what a command wrote and the code it exited with. A command that
// cannot do its job exits non-zero, as the program it stands in for would: it
// ran, and the journal has to say it ran.
type output struct {
	stdout   string
	stderr   string
	exitCode int
}

func echo(_ context.Context, args []string) output {
	return output{stdout: strings.Join(args, " ") + "\n"}
}

func workingDirectory(_ context.Context, args []string) output {
	if refused, ok := rejectArguments("pwd", args); ok {
		return refused
	}
	directory, err := os.Getwd()
	if err != nil {
		return failed("pwd", err)
	}
	return output{stdout: directory + "\n"}
}

func hostname(_ context.Context, args []string) output {
	if refused, ok := rejectArguments("hostname", args); ok {
		return refused
	}
	name, err := os.Hostname()
	if err != nil {
		return failed("hostname", err)
	}
	return output{stdout: name + "\n"}
}

func userName(_ context.Context, args []string) output {
	if refused, ok := rejectArguments("whoami", args); ok {
		return refused
	}
	return output{stdout: currentUser() + "\n"}
}

func identity(_ context.Context, args []string) output {
	if refused, ok := rejectArguments("id", args); ok {
		return refused
	}
	line := fmt.Sprintf("uid=%d(%s) gid=%d\n", os.Getuid(), currentUser(), os.Getgid())
	return output{stdout: line}
}

func kernelName(_ context.Context, args []string) output {
	if refused, ok := rejectArguments("uname", args); ok {
		return refused
	}
	return output{stdout: strings.ToUpper(runtime.GOOS[:1]) + runtime.GOOS[1:] + "\n"}
}

func now(_ context.Context, args []string) output {
	if refused, ok := rejectArguments("date", args); ok {
		return refused
	}
	return output{stdout: time.Now().UTC().Format(time.RFC3339) + "\n"}
}

func concatenate(ctx context.Context, args []string) output {
	if len(args) == 0 {
		// There is no stdin on a tool call, so a cat with no file named is a cat
		// that cannot do its job.
		return output{stderr: "cat: no file\n", exitCode: 1}
	}
	var body strings.Builder
	for _, name := range args {
		if err := ctx.Err(); err != nil {
			return failed("cat", err)
		}
		content, err := readBounded(name, catLimit-body.Len())
		if err != nil {
			return failed("cat", err)
		}
		body.Write(content)
	}
	return output{stdout: body.String()}
}

// readBounded reads one regular file the caller named, at most limit bytes of
// it. The open does not block, so a pipe named in place of a file is refused
// rather than waited on.
func readBounded(name string, limit int) ([]byte, error) {
	file, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0) // #nosec G304 -- reading the file the caller named is the tool.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	content, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err == nil && len(content) > limit {
		err = fmt.Errorf("%s: more than the %d bytes cat reads in all", name, catLimit)
	}
	return content, err
}

func listDirectory(ctx context.Context, args []string) output {
	if len(args) > 1 {
		return output{stderr: "ls: one directory at a time\n", exitCode: 1}
	}
	name := "."
	if len(args) == 1 {
		name = args[0]
	}
	if err := ctx.Err(); err != nil {
		return failed("ls", err)
	}
	entries, err := os.ReadDir(name)
	if err != nil {
		return failed("ls", err)
	}
	var names strings.Builder
	for _, entry := range entries {
		names.WriteString(entry.Name())
		names.WriteString("\n")
	}
	return output{stdout: names.String()}
}

// currentUser falls back to the numeric id. The runtime base carries a passwd
// entry for its one user, and a lookup that fails there should still answer
// with something a person reading a failed run can act on.
func currentUser() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return strconv.Itoa(os.Getuid())
}

// rejectArguments is how a command that takes none answers when it is given
// some: the non-zero exit the program it stands in for would give, rather than
// a refusal. It ran.
func rejectArguments(name string, args []string) (output, bool) {
	if len(args) == 0 {
		return output{}, false
	}
	return output{stderr: name + ": takes no arguments\n", exitCode: 1}, true
}

func failed(name string, err error) output {
	return output{stderr: name + ": " + err.Error() + "\n", exitCode: 1}
}
