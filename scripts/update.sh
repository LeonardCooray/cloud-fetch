#!/usr/bin/env bash
# Update a systemd install of Cloud Fetch to a GitHub release, run as root:
#
#   cloud-fetch-update            latest release
#   cloud-fetch-update 0.1.2      a specific release (also to downgrade)
#   cloud-fetch-update --check    show installed and latest, change nothing
#   cloud-fetch-update --rollback put back the binary the last update replaced
#   cloud-fetch-update --force    reinstall, or replace a source build
#
# BIN, SERVICE and REPO override the defaults below. The service must
# already exist; see "Without Docker" in the README.
set -euo pipefail

BIN=${BIN:-/usr/local/bin/cloud-fetch}
SERVICE=${SERVICE:-cloud-fetch}
REPO=${REPO:-LeonardCooray/cloud-fetch}
# how long the restarted service must stay up before the update counts
SETTLE=${SETTLE:-10}

die() { echo "update: $*" >&2; exit 1; }
say() { echo "update: $*"; }

# --version prints the bare version to stderr and exits non-zero (jpillora/opts)
installed_version() {
	[ -x "$1" ] || return 0
	"$1" --version 2>&1 | tail -n 1 || true
}

latest_version() {
	local url
	# the releases/latest page redirects to .../tag/vX.Y.Z; this avoids the
	# rate-limited API and needs no JSON parser
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
		die "can't reach github.com to find the latest release"
	case $url in
	*/tag/v*) echo "${url##*/tag/v}" ;;
	*) die "no release found at https://github.com/$REPO/releases" ;;
	esac
}

# names follow goreleaser's {os}_{arch}[v{arm}] in .github/goreleaser.yml
release_arch() {
	case $(uname -m) in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	armv7*) echo armv7 ;;
	armv6*) echo armv6 ;;
	i?86) echo 386 ;;
	*) die "no release binary for $(uname -m)" ;;
	esac
}

# Restart=always hides a crash loop from is-active, so also require that
# systemd didn't restart it again while it settled.
restart_and_check() {
	local before after state
	systemctl restart "$SERVICE"
	before=$(systemctl show -p NRestarts --value "$SERVICE")
	sleep "$SETTLE"
	after=$(systemctl show -p NRestarts --value "$SERVICE")
	state=$(systemctl show -p ActiveState --value "$SERVICE")
	[ "$state" = active ] && [ "$before" = "$after" ]
}

# cp then rename, so $BIN is never missing or half-written
replace_bin() {
	cp -p "$1" "$BIN.new"
	mv -f "$BIN.new" "$BIN"
}

rollback() {
	[ -f "$BIN.prev" ] || die "nothing to roll back to ($BIN.prev doesn't exist)"
	local to
	to=$(installed_version "$BIN.prev")
	replace_bin "$BIN.prev"
	if restart_and_check; then
		say "rolled back to ${to:-unknown version}"
	else
		journalctl -u "$SERVICE" -n 20 --no-pager >&2 || true
		die "rolled back to ${to:-unknown version}, but $SERVICE isn't staying up"
	fi
}

main() {
	local mode=update want="" force=0
	while [ $# -gt 0 ]; do
		case $1 in
		--check) mode=check ;;
		--rollback) mode=rollback ;;
		--force) force=1 ;;
		-h | --help) sed -n '2,12s/^# \{0,1\}//p' "$0"; exit 0 ;;
		-*) die "unknown option $1" ;;
		*) want=${1#v} ;;
		esac
		shift
	done

	[ "$(uname -s)" = Linux ] || die "this updates a Linux systemd install"
	command -v systemctl >/dev/null || die "systemctl not found"

	local have
	have=$(installed_version "$BIN")

	if [ "$mode" = check ]; then
		echo "installed: ${have:-none} ($BIN)"
		echo "latest:    $(latest_version)"
		exit 0
	fi

	[ "$(id -u)" = 0 ] || die "run as root (sudo $0 ...)"
	systemctl cat "$SERVICE" >/dev/null 2>&1 || die "no systemd service named $SERVICE"

	if [ "$mode" = rollback ]; then
		rollback
		exit 0
	fi

	[ -n "$want" ] || want=$(latest_version)
	# go install builds report 0.0.0-src and may be newer than any release
	if [ "$have" = 0.0.0-src ] && [ $force = 0 ]; then
		die "$BIN is a source build, which may be newer than $want; --force replaces it"
	fi
	if [ "$have" = "$want" ] && [ $force = 0 ]; then
		say "already on $want (--force to reinstall)"
		exit 0
	fi

	local arch asset base tmp
	arch=$(release_arch)
	asset="cloud-fetch_${want}_linux_${arch}.gz"
	base="https://github.com/$REPO/releases/download/v$want"
	tmp=$(mktemp -d)
	# shellcheck disable=SC2064 # expand now: $tmp is local and gone by EXIT
	trap "rm -rf '$tmp'" EXIT

	say "downloading $want for linux/$arch"
	curl -fsSL -o "$tmp/$asset" "$base/$asset" ||
		die "can't download $asset; is v$want a release with a linux/$arch build?"
	curl -fsSL -o "$tmp/checksums.txt" "$base/cloud-fetch_${want}_checksums.txt" ||
		die "can't download the checksums for v$want"
	(cd "$tmp" && grep "  $asset\$" checksums.txt | sha256sum -c --status) ||
		die "checksum mismatch for $asset; not installing"

	gunzip -c "$tmp/$asset" >"$tmp/cloud-fetch"
	chmod 755 "$tmp/cloud-fetch"
	local got
	got=$(installed_version "$tmp/cloud-fetch")
	[ "$got" = "$want" ] || die "downloaded binary reports '${got}', expected $want; not installing"

	if [ -f "$BIN" ]; then
		cp -p "$BIN" "$BIN.prev"
	fi
	replace_bin "$tmp/cloud-fetch"
	say "installed $want (was ${have:-none}), restarting $SERVICE"

	if restart_and_check; then
		say "$SERVICE is up on $want"
		return 0
	fi

	journalctl -u "$SERVICE" -n 20 --no-pager >&2 || true
	if [ -f "$BIN.prev" ]; then
		say "$SERVICE didn't stay up on $want, rolling back"
		rollback
	fi
	die "$SERVICE didn't stay up on $want"
}

main "$@"
