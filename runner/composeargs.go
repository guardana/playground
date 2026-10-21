package main

// composeArgs builds the arguments after "docker compose".
func composeArgs(file, envFile string, profiles, args []string) []string {
	built := []string{"--env-file", envFile, "-f", file}
	for _, profile := range profiles {
		built = append(built, "--profile", profile)
	}
	return append(built, args...)
}

func upArgs(services []string) []string { return append([]string{"up", "-d", "--wait"}, services...) }

func runOnceArgs(service string, args []string) []string {
	return append([]string{"run", "--rm", "--no-TTY", service}, args...)
}

// runSplitArgs names each variable of runEnv with -e and no value: compose
// copies it from its own environment, so a value never reaches a command line.
func runSplitArgs(service, entrypoint string, args, runEnv []string) []string {
	run := []string{"run", "--rm", "--no-TTY"}
	for _, name := range runEnv {
		run = append(run, "-e", name)
	}
	if entrypoint != "" {
		run = append(run, "--entrypoint", entrypoint)
	}
	return append(append(run, service), args...)
}
