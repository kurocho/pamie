#!/usr/bin/env sh
# SPDX-License-Identifier: AGPL-3.0-only

set -eu

usage() {
	cat <<'EOF'
Usage: scripts/install-local.sh [options]

Build Pamie and install it as a local `pamie` command.

Options:
  --install-dir DIR   Install directory (default: $PAMIE_INSTALL_DIR or $HOME/.local/bin)
  --profile FILE      Shell profile to update (default: $PAMIE_SHELL_PROFILE or auto-detect)
  --version VERSION   Version embedded into the binary (default: $VERSION or dev)
  --no-path           Do not update a shell profile
  -h, --help          Show this help

Examples:
  scripts/install-local.sh
  scripts/install-local.sh --version v0.1.0
  scripts/install-local.sh --install-dir "$HOME/bin"
EOF
}

if [ -z "${HOME:-}" ]; then
	echo "HOME is not set" >&2
	exit 1
fi

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd -P)
REPO_ROOT=$(CDPATH= cd "$SCRIPT_DIR/.." && pwd -P)

INSTALL_DIR=${PAMIE_INSTALL_DIR:-"$HOME/.local/bin"}
SHELL_PROFILE=${PAMIE_SHELL_PROFILE:-}
VERSION_VALUE=${VERSION:-dev}
UPDATE_PATH=1

while [ "$#" -gt 0 ]; do
	case "$1" in
	--install-dir)
		if [ "$#" -lt 2 ]; then
			echo "--install-dir requires a value" >&2
			exit 2
		fi
		INSTALL_DIR=$2
		shift 2
		;;
	--profile)
		if [ "$#" -lt 2 ]; then
			echo "--profile requires a value" >&2
			exit 2
		fi
		SHELL_PROFILE=$2
		shift 2
		;;
	--version)
		if [ "$#" -lt 2 ]; then
			echo "--version requires a value" >&2
			exit 2
		fi
		VERSION_VALUE=$2
		shift 2
		;;
	--no-path)
		UPDATE_PATH=0
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

if ! command -v go >/dev/null 2>&1; then
	echo "Go is required but was not found on PATH" >&2
	exit 1
fi

mkdir -p "$INSTALL_DIR"

TMP_BIN="$INSTALL_DIR/.pamie.tmp.$$"
cleanup() {
	rm -f "$TMP_BIN"
}
trap cleanup EXIT INT TERM

(
	cd "$REPO_ROOT"
	go build -trimpath -ldflags "-s -w -X main.version=$VERSION_VALUE" -o "$TMP_BIN" ./cmd/pamie
)

chmod 755 "$TMP_BIN"
mv "$TMP_BIN" "$INSTALL_DIR/pamie"
trap - EXIT INT TERM

resolve_profile() {
	if [ -n "$SHELL_PROFILE" ]; then
		printf '%s\n' "$SHELL_PROFILE"
		return
	fi

	case "$(basename "${SHELL:-}")" in
	zsh)
		printf '%s\n' "$HOME/.zshrc"
		;;
	bash)
		printf '%s\n' "$HOME/.bashrc"
		;;
	*)
		printf '%s\n' "$HOME/.profile"
		;;
	esac
}

path_contains_install_dir() {
	case ":$PATH:" in
	*":$INSTALL_DIR:"*) return 0 ;;
	*) return 1 ;;
	esac
}

append_path_to_profile() {
	profile_file=$1
	marker="Pamie local install"
	escaped_install_dir=$(printf '%s' "$INSTALL_DIR" | sed 's/["\\$`]/\\&/g')

	mkdir -p "$(dirname "$profile_file")"
	touch "$profile_file"

	if grep -F "$marker" "$profile_file" >/dev/null 2>&1; then
		return
	fi

	{
		printf '\n# %s\n' "$marker"
		printf 'export PATH="%s:$PATH"\n' "$escaped_install_dir"
	} >>"$profile_file"
}

profile_file=
if [ "$UPDATE_PATH" -eq 1 ]; then
	if path_contains_install_dir; then
		echo "PATH already contains $INSTALL_DIR"
	else
		profile_file=$(resolve_profile)
		append_path_to_profile "$profile_file"
		echo "Added $INSTALL_DIR to PATH in $profile_file"
	fi
fi

echo "Installed pamie to $INSTALL_DIR/pamie"
"$INSTALL_DIR/pamie" --version

if [ "$UPDATE_PATH" -eq 1 ] && ! path_contains_install_dir; then
	echo
	echo "Open a new terminal, or run:"
	echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
	if [ -n "$profile_file" ]; then
		echo "  . \"$profile_file\""
	fi
fi
