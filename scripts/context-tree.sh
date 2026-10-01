# shellcheck shell=bash
# context_tree hashes a directory the way git hashes a commit's tree, with no
# conversion: neither the building machine's configuration nor the directory's
# own .gitattributes may normalise a file. It fails when any step fails, since
# a caller reads its output from a command substitution, where errexit does
# not reach. It leaves a .git directory behind, which the caller removes before
# the directory becomes a build context.
context_tree() {
	local run=(env GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
		git -C "$1" -c core.autocrlf=false -c core.attributesFile=/dev/null)
	"${run[@]}" init -q || return 1
	printf '* -text -eol -filter -ident -working-tree-encoding\n' >"$1/.git/info/attributes" || return 1
	"${run[@]}" add -A -f || return 1
	"${run[@]}" write-tree
}
