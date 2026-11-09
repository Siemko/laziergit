#!/bin/sh
# Writes resources/THIRD_PARTY_NOTICES.md: the license of each module
# compiled into the app, as their licenses ask copies to carry.
set -eu
cd "$(dirname "$0")/.."
bin=$(mktemp)
trap 'rm -f "$bin"' EXIT
go build -o "$bin" .
modcache=$(go env GOMODCACHE)
out=resources/THIRD_PARTY_NOTICES.md
{
	echo "# Third-party notices"
	echo
	echo "laziergit includes the following modules, under their licenses."
	go version -m "$bin" | awk '$1 == "dep" { print $2, $3 }' | sort | while read -r mod ver; do
		# The module cache escapes capitals as !lowercase.
		dir="$modcache/$(echo "$mod" | sed -E 's/([A-Z])/!\1/g' | tr 'A-Z' 'a-z')@$ver"
		lic=$(ls "$dir" 2>/dev/null | grep -iE '^(licen[cs]e|copying)([.-][a-z0-9-]+)?$' | head -1 || true)
		echo
		echo "## $mod $ver"
		echo
		if [ -n "$lic" ]; then
			echo '```'
			cat "$dir/$lic"
			echo '```'
		else
			echo "(no license file found in the module)"
		fi
	done
} > "$out"
echo "wrote $out"
