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

func runSplitArgs(service, entrypoint string, args []string) []string {
	run := []string{"run", "--rm", "--no-TTY"}
	if entrypoint != "" {
		run = append(run, "--entrypoint", entrypoint)
	}
	return append(append(run, service), args...)
}
