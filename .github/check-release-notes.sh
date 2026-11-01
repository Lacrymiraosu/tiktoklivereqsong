#!/usr/bin/env bash
# Fails when a release-notes file has a bare @word: GitHub takes it for a
# mention of that user and lists the account under the release's
# Contributors. Write it in backticks (`@name`) or without the @.
[ $# -gt 0 ] || set -- .github/release-notes/*.md
status=0
for f in "$@"; do
  found=$(sed 's/`[^`]*`//g' "$f" | grep -nE '(^|[^A-Za-z0-9_./-])@[A-Za-z0-9][A-Za-z0-9-]*')
  if [ -n "$found" ]; then
    echo "::error file=$f::@word is read as a GitHub mention; put it in backticks or drop the @:"
    echo "$found"
    status=1
  fi
done
exit $status
