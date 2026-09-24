package pages

import (
	"fmt"
	"io/fs"
	"slices"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/frontmatter"
	"github.com/guardana/playground/internal/docscheck/glob"
)

// budgetProblem holds a file to its budget, or to its ceiling when the
// configuration pins one.
func budgetProblem(cfg docsconfig.Config, rel string, words, budget int) []string {
	if ceiling, ok := cfg.Ceilings[rel]; ok {
		if words > ceiling {
			return []string{fmt.Sprintf("%d words, over its ceiling of %d", words, ceiling)}
		}
		return nil
	}
	if words > budget {
		return []string{fmt.Sprintf("%d words, over the budget of %d", words, budget)}
	}
	return nil
}

func judgeReadme(fsys fs.FS, cfg docsconfig.Config, rel string) ([]string, error) {
	data, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return nil, err
	}
	budget := cfg.Readme.Folder
	if rel == "README.md" {
		budget = cfg.Readme.Root
	}
	return budgetProblem(cfg, rel, frontmatter.Words(data), budget), nil
}

// configProblems refuses entries of the configuration that name nothing the
// repository tracks: a surface no file is under would never need a page.
func configProblems(files Files, cfg docsconfig.Config) []string {
	var problems []string
	for name, list := range map[string][]string{"surfaces": cfg.Surfaces, "frozen": cfg.Frozen} {
		for _, pattern := range list {
			g, err := glob.Compile(pattern)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if !slices.ContainsFunc(files.Tracked, g.Match) {
				problems = append(problems, fmt.Sprintf("%s %s matches no file the repository tracks", name, pattern))
			}
		}
	}
	for rel := range cfg.Ceilings {
		if !slices.Contains(files.Listed, rel) {
			problems = append(problems, fmt.Sprintf("ceilings name %s, which the repository does not hold", rel))
		}
	}
	slices.Sort(problems)
	return problems
}
