# Sourced by scripts/package.sh. It never deletes or overwrites anything.
#
# prepare_out OUT REPO checks that a new package may be written to OUT, creates
# OUT if it is missing, and prints it as an absolute path. It refuses when OUT:
#   - is a symlink or a file, or its parent folder does not exist;
#   - is REPO, HOME or / or any parent folder of REPO or HOME (whatever it holds);
#   - is inside a .git folder;
#   - is an existing folder that is not empty, cannot be listed, or is not
#     writable and searchable;
#   - or OUT, REPO or HOME contains a newline (command substitution would change
#     such a path, so it is refused before anything else).
# Variables are prefixed with package_ so the caller's names are left alone.
package_newline='
'
prepare_out() {
  package_out=$1
  package_repo=$2
  [ -n "$package_out" ] && [ -n "$package_repo" ] || { echo "prepare_out: OUT and REPO are required" >&2; return 1; }
  case "$package_out$package_repo${HOME:-}" in
    *"$package_newline"*) echo "package output, repository or HOME path contains a newline; refusing" >&2; return 1 ;;
  esac
  if [ -L "$package_out" ]; then
    echo "package output $package_out is a symlink; refusing" >&2; return 1
  fi
  if [ -e "$package_out" ] && [ ! -d "$package_out" ]; then
    echo "package output $package_out is not a folder; refusing" >&2; return 1
  fi
  package_repo=$(CDPATH= cd -- "$package_repo" && pwd -P) || return 1
  package_home=$(CDPATH= cd -- "${HOME:?}" && pwd -P) || return 1
  if [ -d "$package_out" ]; then
    package_abs=$(CDPATH= cd -- "$package_out" && pwd -P) || return 1
  else
    package_parent=$(dirname -- "$package_out")
    [ -d "$package_parent" ] || { echo "package output parent $package_parent does not exist; refusing" >&2; return 1; }
    package_parent=$(CDPATH= cd -- "$package_parent" && pwd -P) || return 1
    case "$package_parent" in
      /) package_abs="/$(basename -- "$package_out")" ;;
      *) package_abs="$package_parent/$(basename -- "$package_out")" ;;
    esac
  fi
  if [ "$package_abs" = / ]; then
    echo "package output is /; refusing" >&2; return 1
  fi
  for package_protected in "$package_repo" "$package_home"; do
    case "$package_protected/" in
      "$package_abs"/*) echo "package output $package_abs is $package_protected or one of its parent folders; refusing" >&2; return 1 ;;
    esac
  done
  case "$package_abs/" in
    */.git/*) echo "package output $package_abs is inside a .git folder; refusing" >&2; return 1 ;;
  esac
  if [ -d "$package_abs" ]; then
    [ -w "$package_abs" ] && [ -x "$package_abs" ] || { echo "package output $package_abs is not writable and searchable; refusing" >&2; return 1; }
    # One x per entry, whatever its name (even a newline); a plain assignment
    # keeps find's exit status, so a folder that cannot be listed fails closed.
    package_entries=$(find "$package_abs" -mindepth 1 -maxdepth 1 -exec printf x \;) || { echo "package output $package_abs cannot be listed; refusing" >&2; return 1; }
    [ -z "$package_entries" ] || { echo "package output $package_abs is not empty; refusing (use a new folder)" >&2; return 1; }
  else
    mkdir -- "$package_abs" || return 1
  fi
  printf '%s\n' "$package_abs"
}
