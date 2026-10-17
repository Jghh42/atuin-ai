#!/bin/sh
# POSIX bootstrap. The packager replaces this sentinel with ONE immutable tag.
# No release exists yet; this source template requires an explicit --version.
set -eu
umask 077
LC_ALL=C
export LC_ALL
RELEASE_VERSION='UNPUBLISHED'
version=$RELEASE_VERSION
bin_dir=${HOME:?HOME must be set}/.local/bin
base=https://github.com/Jghh42/atuin-ai/releases/download
scratch=
stage=
legal_stage=
fail() { printf '%s\n' "install: $*" >&2; exit 1; }
cleanup() {
    [ -z "$stage" ] || rm -f -- "$stage"
    [ -z "$legal_stage" ] || rm -f -- "$legal_stage"
    [ -z "$scratch" ] || rm -rf -- "$scratch"
    :
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
while [ "$#" -gt 0 ]; do
    case $1 in
        --version) [ "$#" -ge 2 ] || fail 'missing version'; version=$2; shift 2 ;;
        --bin-dir) [ "$#" -ge 2 ] || fail 'missing bin directory'; bin_dir=$2; shift 2 ;;
        --help) printf '%s\n' 'Usage: sh install.sh [--version vX.Y.Z] [--bin-dir ABSOLUTE_PATH]'; exit 0 ;;
        *) fail 'unknown argument' ;;
    esac
done
case $version in *[!A-Za-z0-9.-]*) fail 'unsafe version' ;; esac
if [ "${#version}" -gt 100 ] || ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9]+([.-][A-Za-z0-9]+)*)?$'; then
    fail 'unpublished installer or unsafe version; specify --version'
fi
case $(uname -s) in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) fail 'unsupported operating system' ;;
esac
case $(uname -m) in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) fail 'unsupported architecture' ;;
esac
case $os/$arch in linux/amd64|darwin/arm64) : ;; *) fail 'unsupported OS/architecture pair' ;; esac
# Reject ambiguous/control-bearing paths and ALL symlink ancestors. Spaces work.
case $bin_dir in /*) : ;; *) fail 'bin directory must be absolute' ;; esac
case $bin_dir in *'
'*|*//*|*/./*|*/../*|*/.|*/..|*/) fail 'unsafe bin directory' ;; esac
if printf '%s' "$bin_dir" | grep -q '[[:cntrl:]]'; then fail 'control character in bin directory'; fi
# Validate before downloads, create only after all downloaded bytes are verified.
check_dirs() {
    rest=${bin_dir#/}
    current=
    while [ -n "$rest" ]; do
        part=${rest%%/*}
        if [ "$rest" = "$part" ]; then rest=; else rest=${rest#*/}; fi
        current=$current/$part
        [ ! -L "$current" ] || fail 'symlink directory refused'
        if [ -e "$current" ]; then
            [ -d "$current" ] || fail 'non-directory ancestor refused'
        elif [ "$1" = create ]; then
            mkdir -- "$current" || fail 'cannot create bin directory'
        fi
    done
}
check_dirs validate
target=$bin_dir/atuin-ai-server
legal=$bin_dir/atuin-ai-server.licenses-$version.txt
check_target() {
    [ ! -L "$1" ] || fail 'symlink target refused'
    if [ -e "$1" ]; then [ -f "$1" ] && [ ! -d "$1" ] || fail 'nonregular target refused'; fi
}
check_target "$target"
check_target "$legal"
for command in curl mktemp od awk wc tr chmod mv; do command -v "$command" >/dev/null 2>&1 || fail "missing tool: $command"; done
# curl <8.4 does NOT bound chunked/unknown-length downloads with --max-filesize.
# Fail before scratch/downloads instead of adding a fragile POSIX streaming pipeline.
curl_version=$(curl -q --version) || fail 'cannot determine curl version'
if ! printf '%s\n' "$curl_version" | awk '
    NR == 1 && $1 == "curl" && $2 ~ /^[0-9]+\.[0-9]+\.[0-9]+$/ {
        split($2, v, "."); ok = (v[1] > 8 || (v[1] == 8 && v[2] >= 4))
    }
    END { exit !ok }
'; then
    fail 'curl 8.4.0 or newer required to bound unknown-length downloads; install an up-to-date curl using your trusted package manager, then rerun (no automatic install)'
fi
if command -v sha256sum >/dev/null 2>&1; then hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then hash_tool=shasum
else fail 'sha256sum or shasum is required'; fi
hash() {
    if [ "$hash_tool" = sha256sum ]; then sha256sum < "$1"; else shasum -a 256 < "$1"; fi | awk '{print $1}'
}
case ${TMPDIR:-/tmp} in /*) : ;; *) fail 'TMPDIR must be absolute' ;; esac
scratch=$(mktemp -d "${TMPDIR:-/tmp}/atuin-ai-install.XXXXXXXXXX") || fail 'cannot create private scratch'
asset=atuin-ai-server-$version-$os-$arch
legal_asset=atuin-ai-server-$version-legal.txt
fetch() {
    curl -q --proto '=https' --proto-redir '=https' --tlsv1.2 --location --max-redirs 3 \
        --connect-timeout 15 --max-time 120 --fail --silent --show-error \
        --max-filesize "$2" --output "$scratch/$1" "$base/$version/$1" || fail 'HTTPS download failed'
    [ -f "$scratch/$1" ] && [ "$(wc -c < "$scratch/$1")" -le "$2" ] || fail 'download size exceeded'
}
fetch SHA256SUMS 65536
# No pathname from the remote manifest is ever passed to a file operation.
expected() {
    awk -v name="$1" '
        length($0) > 300 || NF != 2 || length($1) != 64 || $1 ~ /[^0-9a-f]/ || $2 ~ /[^A-Za-z0-9._-]/ { bad=1 }
        $2 == name { count++; value=$1 }
        END { if (bad || count != 1) exit 1; print value }
    ' "$scratch/SHA256SUMS"
}
binary_hash=$(expected "$asset") || fail 'missing, duplicate or malformed checksum'
legal_hash=$(expected "$legal_asset") || fail 'missing, duplicate or malformed legal checksum'
fetch "$asset" 67108864
fetch "$legal_asset" 1048576
[ "$(hash "$scratch/$asset")" = "$binary_hash" ] || fail 'binary checksum mismatch'
[ "$(hash "$scratch/$legal_asset")" = "$legal_hash" ] || fail 'legal checksum mismatch'
[ "$(wc -c < "$scratch/$asset")" -ge 4096 ] || fail 'truncated binary'
[ -s "$scratch/$legal_asset" ] || fail 'empty legal bundle'
header=$(od -An -tx1 -N20 "$scratch/$asset" | tr -d ' \n')
case $os/$arch:$header in
    linux/amd64:7f454c4602010100????????????????????3e00) : ;;
    darwin/arm64:cffaedfe0c000001????????????????????????) : ;;
    *) fail 'binary format/architecture mismatch' ;;
esac
check_dirs create
check_target "$target"
check_target "$legal"
# Stage IN destination filesystem; old executable is never truncated/chmodded.
stage=$(mktemp "$bin_dir/.atuin-ai-binary.XXXXXXXXXX") || fail 'cannot stage binary'
legal_stage=$(mktemp "$bin_dir/.atuin-ai-legal.XXXXXXXXXX") || fail 'cannot stage legal bundle'
# cat only copies verified data; neither the bootstrap nor downloaded code executes it.
cat "$scratch/$asset" > "$stage" || fail 'cannot copy binary'
cat "$scratch/$legal_asset" > "$legal_stage" || fail 'cannot copy legal bundle'
[ "$(hash "$stage")" = "$binary_hash" ] && [ "$(hash "$legal_stage")" = "$legal_hash" ] || fail 'staging checksum mismatch'
chmod 755 "$stage" || fail 'cannot set executable mode'
chmod 644 "$legal_stage" || fail 'cannot set legal mode'
check_dirs validate
check_target "$target"
check_target "$legal"
mv -f -- "$legal_stage" "$legal" || fail 'cannot install legal bundle'
legal_stage=
# Commit point: ignore catchable termination during the one atomic rename.
# SIGKILL/power loss can leave an owned temporary file, never a partial old binary.
trap '' HUP INT TERM
mv -f -- "$stage" "$target" || fail 'cannot replace binary'
stage=
printf '%s\n' "Installed $target ($version, $os/$arch)." "Legal notices: $legal" \
    "Add $bin_dir to PATH yourself if needed. No login or configuration was performed." || :
