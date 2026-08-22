#!/bin/sh
# Regenerates Sources/TossBinCore/OperatingDoc.generated.swift from
# operating.md. Runs before every swift build via the Makefile; the
# generated file is committed so a plain `swift build` also works.
set -eu
cd "$(dirname "$0")/.."

doc="operating.md"
out="Sources/TossBinCore/OperatingDoc.generated.swift"

# The doc is emitted into a ##"""..."""## raw string literal. Two sequences
# can break out of it: the closing delimiter itself, and the \## escape
# family (\##( starts interpolation, \##n and friends are escapes).
if grep -q '"""##' "$doc"; then
    echo "gen-operating-doc: $doc contains the raw-string terminator \"\"\"##" >&2
    exit 1
fi
if grep -q '\\##' "$doc"; then
    echo "gen-operating-doc: $doc contains the raw-string escape sequence \\##" >&2
    exit 1
fi

{
    echo "// Generated from operating.md by scripts/gen-operating-doc.sh. Do not edit."
    echo ""
    echo "public let operatingDoc = ##\"\"\""
    cat "$doc"
    # The closing delimiter must start on its own line; supply the newline
    # cat did not emit when operating.md lacks a trailing one.
    if [ -n "$(tail -c 1 "$doc")" ]; then
        echo ""
    fi
    echo "\"\"\"##"
} > "$out.tmp"

# Leave the file untouched when nothing changed so swift build does not
# recompile the target on every run.
if cmp -s "$out.tmp" "$out" 2>/dev/null; then
    rm "$out.tmp"
else
    mv "$out.tmp" "$out"
fi
