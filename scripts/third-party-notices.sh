#!/bin/sh
# Generate THIRD-PARTY-NOTICES.md from the modules linked into the binary.
#
# MIT, BSD and ISC all require their copyright notice to travel with copies of
# the software, and a compiled binary is a copy. Generated at release time so it
# can never drift from go.mod.
#
# Deps of the main package only — `go list -m all` would also pull in test-only
# modules that never reach the binary.
set -eu

out="${1:-THIRD-PARTY-NOTICES.md}"
self="github.com/lucky7xz/garlic"

{
	echo "# Third-party notices"
	echo
	echo "garlic links the Go modules below into its release binaries. Each is listed"
	echo "with the copyright notice and licence its own terms require us to carry."
	echo
	echo "garlic itself is MIT — see LICENSE. The bootstrap config and themes written"
	echo "into your config directory on first run are garlic's own."
} >"$out"

go list -deps -f '{{with .Module}}{{.Path}}	{{.Version}}	{{.Dir}}{{end}}' . |
	sort -u |
	while IFS='	' read -r path version dir; do
		[ -n "${dir:-}" ] || continue        # stdlib has no module
		[ "$path" != "$self" ] || continue   # our own licence ships separately

		lic=$(ls "$dir" 2>/dev/null | grep -iE '^(licen[cs]e|copying)' | head -1) || true
		if [ -z "$lic" ]; then
			echo "warning: no licence file found for $path $version" >&2
			continue
		fi

		{
			echo
			echo "## $path $version"
			echo
			echo '```'
			cat "$dir/$lic"
			echo '```'
		} >>"$out"
	done

echo "wrote $out ($(grep -c '^## ' "$out") modules)"
