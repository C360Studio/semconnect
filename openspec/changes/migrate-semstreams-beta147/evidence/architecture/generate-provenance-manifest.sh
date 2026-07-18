#!/bin/sh
set -eu

commit="c8f0b92edf5ad5b491d5f4e81891bec817fae3cd"
archive_sha256="bbac9d4537bc45cb435bb64a71c2487070047c48326dce042b47ad5bad48907e"
destination_module="github.com/c360studio/semconnect"

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    echo "usage: $0 SOURCE_ROOT [OUTPUT]" >&2
    exit 2
fi

source_root=${1%/}
output=${2:-}

trees='message/oms
parser/sensorml
pkg/swecommon
vocabulary/csapi
vocabulary/oms
vocabulary/sosa
vocabulary/swe'

for tree in $trees; do
    if [ ! -d "$source_root/$tree" ]; then
        echo "missing provenance tree: $source_root/$tree" >&2
        exit 1
    fi
done

file_list=$(mktemp "${TMPDIR:-/tmp}/semconnect-provenance.XXXXXX")
rendered=$(mktemp "${TMPDIR:-/tmp}/semconnect-provenance-json.XXXXXX")
trap 'rm -f "$file_list" "$rendered"' EXIT HUP INT TERM

for tree in $trees; do
    find "$source_root/$tree" -type f -print
done | LC_ALL=C sort > "$file_list"

file_count=$(wc -l < "$file_list" | tr -d ' ')
if [ "$file_count" -ne 55 ]; then
    echo "expected 55 provenance files, found $file_count" >&2
    exit 1
fi

{
    printf '{\n'
    printf '  "formatVersion": "1.0.0",\n'
    printf '  "repository": "https://github.com/C360Studio/semstreams",\n'
    printf '  "sourceCommit": "%s",\n' "$commit"
    printf '  "sourceArchive": {\n'
    printf '    "url": "https://github.com/C360Studio/semstreams/archive/%s.tar.gz",\n' "$commit"
    printf '    "sha256": "%s"\n' "$archive_sha256"
    printf '  },\n'
    printf '  "destinationModule": "%s",\n' "$destination_module"
    printf '  "treeCount": 7,\n'
    printf '  "fileCount": %s,\n' "$file_count"
    printf '  "trees": [\n'

    tree_index=0
    for tree in $trees; do
        tree_index=$((tree_index + 1))
        tree_count=$(find "$source_root/$tree" -type f | wc -l | tr -d ' ')
        comma=','
        if [ "$tree_index" -eq 7 ]; then
            comma=''
        fi
        printf '    {"source": "%s", "destination": "%s", "fileCount": %s}%s\n' \
            "$tree" "$tree" "$tree_count" "$comma"
    done

    printf '  ],\n'
    printf '  "files": [\n'

    index=0
    while IFS= read -r absolute_path; do
        relative_path=${absolute_path#"$source_root"/}
        case "$relative_path" in
            *'"'*|*'\'*|*'
'*)
                echo "unsupported path in provenance tree: $relative_path" >&2
                exit 1
                ;;
        esac

        if [ -x "$absolute_path" ]; then
            mode="100755"
        else
            mode="100644"
        fi
        sha256=$(shasum -a 256 "$absolute_path" | awk '{print $1}')
        bytes=$(wc -c < "$absolute_path" | tr -d ' ')
        index=$((index + 1))
        comma=','
        if [ "$index" -eq "$file_count" ]; then
            comma=''
        fi
        printf '    {"source": "%s", "destination": "%s", "mode": "%s", ' \
            "$relative_path" "$relative_path" "$mode"
        printf '"bytes": %s, "sha256": "%s"}%s\n' "$bytes" "$sha256" "$comma"
    done < "$file_list"

    printf '  ]\n'
    printf '}\n'
} > "$rendered"

if [ -n "$output" ]; then
    mv "$rendered" "$output"
    trap 'rm -f "$file_list"' EXIT HUP INT TERM
else
    sed -n '1,$p' "$rendered"
fi
