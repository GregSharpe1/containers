#!/usr/bin/env bash

set -euo pipefail

shopt -s nullglob

shared_paths=(
  ".github/workflows/"
  "scripts/"
  "Makefile"
  "README.md"
  "renovate.json"
  ".gitignore"
)

list_images() {
  local dockerfile

  for dockerfile in */Dockerfile; do
    printf '%s\n' "${dockerfile%/Dockerfile}"
  done | sort
}

emit_matrix() {
  local images=("$@")
  local first=true
  local image

  printf '{"include":['
  for image in "${images[@]}"; do
    if [ "$first" = true ]; then
      first=false
    else
      printf ','
    fi

    printf '{"image":"%s","context":"%s"}' "$image" "$image"
  done
  printf ']}'
}

is_valid_commit() {
  local commit="$1"

  [ -n "$commit" ] && [ "$commit" != "0000000000000000000000000000000000000000" ] && git cat-file -e "$commit^{commit}" 2>/dev/null
}

build_all=false
changed_files=""

if [ "${FORCE_ALL:-false}" = "true" ]; then
  build_all=true
elif is_valid_commit "${BASE_SHA:-}" && is_valid_commit "${HEAD_SHA:-}"; then
  changed_files="$(git diff --name-only "$BASE_SHA" "$HEAD_SHA")"
elif git rev-parse --verify HEAD >/dev/null 2>&1; then
  changed_files="$(git diff-tree --no-commit-id --name-only -r HEAD)"
else
  build_all=true
fi

declare -A selected_images=()

if [ "$build_all" = false ] && [ -n "$changed_files" ]; then
  while IFS= read -r path; do
    [ -n "$path" ] || continue

    for shared_path in "${shared_paths[@]}"; do
      case "$path" in
        "$shared_path"*)
          build_all=true
          break 2
          ;;
      esac
    done

    top_level="${path%%/*}"
    if [ -f "$top_level/Dockerfile" ]; then
      selected_images["$top_level"]=1
    fi
  done <<< "$changed_files"
fi

if [ "$build_all" = true ]; then
  mapfile -t images < <(list_images)
elif [ "${#selected_images[@]}" -gt 0 ]; then
  mapfile -t images < <(printf '%s\n' "${!selected_images[@]}" | sort)
else
  images=()
fi

emit_matrix "${images[@]}"
