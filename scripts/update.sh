#!/usr/bin/env bash
# Update a systemd install of Cloud Fetch to a GitHub release, run as root:
#
#   cloud-fetch-update            latest release
#   cloud-fetch-update 0.1.2      a specific release (also to downgrade)
#   cloud-fetch-update --check    show installed and latest, change nothing
#   cloud-fetch-update --rollback put back the binary the last update replaced
#   cloud-fetch-update --force    reinstall, or replace a source build
#
# BIN, SERVICE, REPO, SETTLE, HEALTH_WAIT and HEALTH_URL override the
# defaults below. The service must already exist; see "Without Docker" in
# the README.
set -euo pipefail

BIN=${BIN:-/usr/local/bin/cloud-fetch}
SERVICE=${SERVICE:-cloud-fetch}
REPO=${REPO:-LeonardCooray/cloud-fetch}
# how long the restarted service must stay up before the update counts
SETTLE=${SETTLE:-10}
# how long /healthz may keep answering "unavailable" after that, in seconds
HEALTH_WAIT=${HEALTH_WAIT:-30}
# HEALTH_URL unset works the URL out from the running service's arguments
# and environment, and HEALTH_URL= (empty) skips the /healthz check

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

# Sets health_url; for --domain also health_resolve (so curl reaches this
# machine under the certificate's name without asking DNS), and
# health_pending=1 while Let's Encrypt hasn't issued the certificate yet.
# All from the running
# process's arguments and environment as systemd expanded them. Fails when
# it can't read them, or the process isn't $BIN (a wrapper that didn't exec).
health_target() {
	health_url="" health_resolve="" health_pending=""
	if [ -n "${HEALTH_URL+set}" ]; then
		health_url=$HEALTH_URL
		return 0
	fi
	local pid name val port=3000 host="" domain="" scheme=http addr
	local cache="" config=cloud-fetch.json
	local -a args env
	pid=$(systemctl show -p MainPID --value "$SERVICE")
	if [ "${pid:-0}" = 0 ] || [ ! -r "/proc/$pid/cmdline" ]; then
		return 1
	fi
	[ "$(readlink -f "/proc/$pid/exe")" = "$(readlink -f "$BIN")" ] || return 1
	mapfile -d '' args <"/proc/$pid/cmdline" || return 1
	mapfile -d '' env <"/proc/$pid/environ" || return 1
	# opts reads these two from the environment, unless empty, and flags
	# override them
	for val in "${env[@]}"; do
		case $val in
		PORT=?*) port=${val#PORT=} ;;
		DOMAIN=?*) domain=${val#DOMAIN=} ;;
		esac
	done
	set -- "${args[@]:1}"
	while [ $# -gt 0 ]; do
		case $1 in
		-*)
			name=${1#-} name=${name#-} val=""
			case $name in
			*=*) val=${name#*=} name=${name%%=*} ;;
			# a boolean flag (--log) is followed by another flag or nothing,
			# since cloud-fetch takes no other arguments
			*) case ${2--} in -*) ;; *) val=$2 && shift ;; esac ;;
			esac
			case $name in
			port | p) port=$val ;;
			host | h) host=$val ;;
			domain) domain=$val ;;
			cert-path | key-path | k) scheme=https ;;
			cert-cache) cache=$val ;;
			config-path | c) config=$val ;;
			esac
			;;
		esac
		shift
	done
	case $port in '' | *[!0-9]*) return 1 ;; esac
	# autocert's cache uses the lowercase name without a trailing dot
	domain=${domain%.} domain=${domain,,}
	case $host in
	'' | 0.0.0.0 | :: | '[::]') addr=127.0.0.1 ;;
	*:*) addr="[${host#[}" addr="${addr%]}]" ;;
	*[!0-9.]*)
		# a name: Go listens on its first IPv4 address if it has one
		addr=$( (getent ahostsv4 "$host" || getent ahosts "$host") | awk 'NR == 1 { print $1 }') || true
		case $addr in *:*) addr="[$addr]" ;; esac
		;;
	*) addr=$host ;;
	esac
	[ -n "$addr" ] || return 1
	if [ -z "$domain" ]; then
		health_url="$scheme://$addr:$port/healthz"
		return 0
	fi
	case $domain in *[!A-Za-z0-9.-]*) return 1 ;; esac
	health_url="https://$domain:$port/healthz"
	health_resolve="$domain:$port:$addr"
	# where autocert keeps it: --cert-cache, else certs/ next to the config,
	# relative to the server's working directory
	[ -n "$cache" ] || cache=$(dirname "$config")/certs
	case $cache in /*) ;; *) cache=/proc/$pid/cwd/$cache ;; esac
	if [ ! -e "$cache/$domain" ] && [ ! -e "$cache/$domain+rsa" ]; then
		health_pending=1
	fi
}

# still_up NRESTARTS: the service is active and systemd hasn't restarted it
# since NRestarts was NRESTARTS. Restart=always hides a crash loop from
# is-active, hence the count.
still_up() {
	[ "$(systemctl show -p ActiveState --value "$SERVICE")" = active ] &&
		[ "$(systemctl show -p NRestarts --value "$SERVICE")" = "$1" ]
}

# health_result HTTP_CODE CURL_EXIT BODY CERT_PENDING prints what one try
# means: ok, unhealthy (it says so), old (a release before /healthz, which
# answers 401 or 404), refused (nothing listening), waiting (the first Let's
# Encrypt certificate may still be being issued) or broken.
health_result() {
	case $2 in
	0) ;;
	7) echo refused && return ;;
	# 28 timed out, 35 TLS handshake failed
	28 | 35) [ "$4" = 1 ] && echo waiting || echo broken && return ;;
	*) echo broken && return ;;
	esac
	case $1 in
	200) [ "${3%$'\n'}" = ok ] && echo ok || echo old ;;
	5[0-9][0-9]) echo unhealthy ;;
	*) echo old ;;
	esac
}

# check_health NRESTARTS fails when /healthz isn't ok within HEALTH_WAIT
# seconds, except that a release from before /healthz, or a --domain server
# with no certificate yet that doesn't answer, leaves it to the systemd checks. It stops
# waiting if the service goes down, which they catch.
check_health() {
	local health_url health_resolve health_pending resp rc result deadline
	# already down: the systemd checks report that
	still_up "$1" || return 0
	if ! health_target; then
		say "can't tell where $SERVICE listens, so not checking /healthz (set HEALTH_URL)"
		return 0
	fi
	[ -n "$health_url" ] || return 0
	deadline=$((SECONDS + HEALTH_WAIT))
	while still_up "$1"; do
		# -k: it's this machine, and a staging certificate is fine
		rc=0
		resp=$(curl -ks --noproxy '*' --max-time 5 -w '%{http_code}' ${health_resolve:+--resolve "$health_resolve"} "$health_url") || rc=$?
		result=$(health_result "${resp: -3}" "$rc" "${resp%???}" "$health_pending")
		case $result in
		ok)
			say "$health_url says ok"
			return 0
			;;
		old)
			say "no /healthz at $health_url (likely a release before it), so not checking it"
			return 0
			;;
		esac
		if [ "$SECONDS" -ge "$deadline" ]; then
			case $result in
			unhealthy) say "$health_url still says unavailable after ${HEALTH_WAIT}s" ;;
			waiting)
				say "no answer from $health_url, which may be a certificate still being issued, so not checking /healthz"
				return 0
				;;
			refused) say "nothing accepts connections at $health_url after ${HEALTH_WAIT}s (set HEALTH_URL if that's the wrong address)" ;;
			*) say "no working answer from $health_url after ${HEALTH_WAIT}s (curl exit $rc)" ;;
			esac
			return 1
		fi
		sleep 2
	done
}

restart_and_check() {
	local before healthy=1
	systemctl restart "$SERVICE"
	before=$(systemctl show -p NRestarts --value "$SERVICE")
	sleep "$SETTLE"
	check_health "$before" || healthy=0
	still_up "$before" && [ "$healthy" = 1 ]
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
		die "rolled back to ${to:-unknown version}, but $SERVICE isn't up and healthy on it either"
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

	# checked now: bash arithmetic on HEALTH_WAIT later would end the script
	# mid-update, and a SETTLE sleep can't take would skip the settle time
	case $HEALTH_WAIT in '' | *[!0-9]*) die "HEALTH_WAIT must be a whole number of seconds, not '$HEALTH_WAIT'" ;; esac
	case $SETTLE in '' | *[!0-9]*) die "SETTLE must be a whole number of seconds, not '$SETTLE'" ;; esac
	# base 10, or a leading zero (08) would be read as octal
	HEALTH_WAIT=$((10#$HEALTH_WAIT))

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
		say "$SERVICE isn't up and healthy on $want, rolling back"
		rollback
	fi
	die "$SERVICE isn't up and healthy on $want"
}

# sourcing it (scripts/update_test.sh) only defines the functions
# (piped to bash, BASH_SOURCE is empty)
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	main "$@"
fi
