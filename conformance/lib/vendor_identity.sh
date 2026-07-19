#!/usr/bin/env bash

# Docker builds worktree bytes, not merely Git's HEAD. Reuse is safe only
# when the materialized source is the requested commit with no tracked,
# untracked, ignored, or submodule drift.
git_source_matches_commit() {
    local source_dir="$1"
    local expected_commit="$2"
    local current
    local drift

    [[ -d "$source_dir/.git" ]] || return 1
    current="$(git -C "$source_dir" rev-parse HEAD 2>/dev/null || true)"
    [[ "$current" == "$expected_commit" ]] || return 1

    if ! drift="$(git -C "$source_dir" status \
        --porcelain=v1 \
        --untracked-files=all \
        --ignored=matching \
        --ignore-submodules=none 2>/dev/null)"; then
        return 1
    fi
    [[ -z "$drift" ]]
}
