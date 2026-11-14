#!/usr/bin/env bash
# Tests update.sh's /healthz check: how it works out the URL from the running
# process, and what it makes of each answer. Linux only, since it reads /proc;
# needs python3 for a stand-in process.
set -uo pipefail
# shellcheck source=scripts/update.sh
. "$(dirname "$0")/update.sh"
set +e

fails=0
check() {
	if [ "$2" = "$3" ]; then
		echo "ok   $1"
	else
		echo "FAIL $1: got '$2', want '$3'"
		fails=$((fails + 1))
	fi
}

# health_result HTTP_CODE CURL_EXIT BODY DOMAIN_MODE
check "200 ok" "$(health_result 200 0 $'ok\n' '')" ok
check "200 with another body" "$(health_result 200 0 '<html>' '')" old
check "401 from an old release behind --auth" "$(health_result 401 0 Unauthorized '')" old
check "404 from an old release" "$(health_result 404 0 '' '')" old
check "503" "$(health_result 503 0 $'unavailable\n' '')" unhealthy
check "500" "$(health_result 500 0 '' '')" unhealthy
check "refused" "$(health_result 000 7 '' '')" refused
check "refused with --domain" "$(health_result 000 7 '' 1)" refused
check "timeout" "$(health_result 000 28 '' '')" broken
check "timeout with --domain" "$(health_result 000 28 '' 1)" waiting
check "TLS failure with --domain" "$(health_result 000 35 '' 1)" waiting
check "empty reply with --domain" "$(health_result 000 52 '' 1)" broken

# health_target reads a stand-in process: python3 with the flags appended
# (its own "-c CODE" parses as one flag and its value). fake_pid, not pid,
# since health_target's own local pid would hide it from systemctl.
SERVICE=cloud-fetch
python=$(readlink -f "$(command -v python3)")
BIN=$python
fake_pid=0
systemctl() {
	case $3 in
	MainPID) echo "$fake_pid" ;;
	ActiveState) echo active ;;
	NRestarts) echo 0 ;;
	esac
}
target() {
	local vars=$1
	shift
	# shellcheck disable=SC2086 # vars is a list of NAME=value words
	env $vars python3 -c 'import time; time.sleep(30)' "$@" >/dev/null 2>&1 &
	fake_pid=$!
	# env execs python3; until then /proc shows env
	local tries=0
	while [ "$(readlink -f "/proc/$fake_pid/exe")" != "$python" ]; do
		tries=$((tries + 1))
		if [ "$tries" -gt 100 ] || ! kill -0 "$fake_pid" 2>/dev/null; then
			echo "stand-in process never became $python"
			return
		fi
		sleep 0.05
	done
	if health_target; then
		echo "$health_url${health_resolve:+ $health_resolve}${health_pending:+ pending}"
	else
		echo "can't tell"
	fi
	kill "$fake_pid"
}
check "--port" "$(target '' --port 3300 --auth a:b)" "http://127.0.0.1:3300/healthz"
check "-p=, single-dash long flag" "$(target '' -p=3301 -auth=a:b)" "http://127.0.0.1:3301/healthz"
check "default port" "$(target '' --log)" "http://127.0.0.1:3000/healthz"
check "PORT from the environment" "$(target PORT=3200)" "http://127.0.0.1:3200/healthz"
check "an empty PORT is ignored, as opts does" "$(target 'PORT= DOMAIN=')" "http://127.0.0.1:3000/healthz"
check "a flag overrides PORT" "$(target PORT=3200 --port 3201)" "http://127.0.0.1:3201/healthz"
check "--domain" "$(target '' --domain f.example --port 443)" "https://f.example:443/healthz f.example:443:127.0.0.1 pending"
check "DOMAIN from the environment" "$(target 'DOMAIN=e.example PORT=443')" "https://e.example:443/healthz e.example:443:127.0.0.1 pending"
check "--domain with --host localhost" "$(target '' --domain f.example --host localhost --port 443)" "https://f.example:443/healthz f.example:443:127.0.0.1 pending"
# a certificate already in autocert's cache means nothing is pending
certs=$(mktemp -d)
mkdir -p "$certs/conf/certs" "$certs/own"
touch "$certs/conf/certs/f.example" "$certs/own/f.example+rsa"
check "cached certificate next to the config" "$(cd "$certs" && target '' --domain f.example --config-path conf/cloud-fetch.json)" "https://f.example:3000/healthz f.example:3000:127.0.0.1"
check "cached certificate in --cert-cache" "$(target '' --domain f.example --cert-cache "$certs/own")" "https://f.example:3000/healthz f.example:3000:127.0.0.1"
check "a mixed-case domain with a trailing dot" "$(target '' --domain F.Example. --cert-cache "$certs/own")" "https://f.example:3000/healthz f.example:3000:127.0.0.1"
check "a certificate for another domain" "$(target '' --domain g.example --cert-cache "$certs/own")" "https://g.example:3000/healthz g.example:3000:127.0.0.1 pending"
check "--host address" "$(target '' --host=10.0.0.5)" "http://10.0.0.5:3000/healthz"
check "--host IPv6" "$(target '' -h ::1 -p 3000)" "http://[::1]:3000/healthz"
check "--host all IPv6" "$(target '' --host '[::]')" "http://127.0.0.1:3000/healthz"
check "--cert-path makes it https" "$(target '' --cert-path c.pem -k k.pem --port 8443)" "https://127.0.0.1:8443/healthz"
check "boolean flags don't take the next flag" "$(target '' --some-new-bool --port 3302 --log)" "http://127.0.0.1:3302/healthz"
check "a value with spaces and a glob" "$(target '' --title 'My * Fetch' --port 3303)" "http://127.0.0.1:3303/healthz"
check "port that isn't a number" "$(target '' -p 99x)" "can't tell"
check "--host that doesn't resolve" "$(target '' --host no-such-host.invalid)" "can't tell"
check "HEALTH_URL wins" "$(HEALTH_URL=http://x/healthz target '' --port 1)" "http://x/healthz"
BIN=/usr/local/bin/cloud-fetch
check "a process that isn't BIN (a wrapper)" "$(target '' --port 3300)" "can't tell"
check "no MainPID" "$(fake_pid=0 health_target && echo found || echo "can't tell")" "can't tell"

# check_health with a fake curl and no waiting
HEALTH_WAIT=0
HEALTH_URL=http://127.0.0.1:3000/healthz
curl() {
	printf '%s' "$fake"
	return "$fake_rc"
}
say() { :; }
health() {
	fake=$1 fake_rc=$2
	check_health 0 && echo pass || echo fail
}
check "check_health: ok" "$(health $'ok\n200' 0)" pass
check "check_health: old release" "$(health 'Unauthorized401' 0)" pass
check "check_health: unavailable" "$(health $'unavailable\n503' 0)" fail
check "check_health: nothing listening" "$(health 000 7)" fail
check "check_health: no answer" "$(health 000 28)" fail
check "check_health: HEALTH_URL= skips" "$(HEALTH_URL='' health 000 7)" pass
# with --domain: a timeout passes only while no certificate is cached
unset HEALTH_URL
health_target() {
	health_url=https://f.example:443/healthz health_resolve=f.example:443:127.0.0.1 health_pending=$pending
}
pending=1
check "check_health: timeout, certificate pending" "$(health 000 28)" pass
check "check_health: TLS failure, certificate pending" "$(health 000 35)" pass
check "check_health: empty reply, certificate pending" "$(health 000 52)" fail
pending=""
check "check_health: timeout, certificate cached" "$(health 000 28)" fail

rm -rf "$certs"

check "HEALTH_WAIT that isn't a number stops before anything changes" "$(HEALTH_WAIT=30s main --rollback 2>&1)" "update: HEALTH_WAIT must be a whole number of seconds, not '30s'"
# id -u comes straight after the HEALTH_WAIT check; a fake one reports the
# value main went on with, then stops it as not root
check "HEALTH_WAIT with a leading zero is read as decimal" "$(id() { echo "wait=$HEALTH_WAIT" >&2; echo 1000; }; HEALTH_WAIT=08 main --rollback 2>&1 | head -n 1)" "wait=8"

# piped to bash, it still runs main (here as far as the Linux check)
# shellcheck disable=SC2329 # the fake uname is exported to the child bash
check "piped to bash runs main" "$(uname() { echo Darwin; }; export -f uname; bash -s -- --check <"$(dirname "$0")/update.sh" 2>&1)" "update: this updates a Linux systemd install"

if [ "$fails" != 0 ]; then
	echo "$fails failed"
	exit 1
fi
echo "all passed"
