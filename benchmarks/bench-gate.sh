#!/usr/bin/env bash
#
# Benchmark regression gate for the hot paths listed in benchmarks/hotpaths.txt.
#
#   bench-gate.sh gate                  run the list here and gate it against the committed baseline
#   bench-gate.sh baseline              run the list here and overwrite the committed baseline
#   bench-gate.sh pair BASE NEW OUTDIR  run the list in checkouts BASE and NEW, interleaved, and gate NEW
#   bench-gate.sh run DIR=OUT [...]     run the list in each checkout DIR, interleaved, into OUT
#   bench-gate.sh compare OLD NEW       benchstat OLD vs NEW and fail on a regression (single pass)
#
# A regression is any of:
#   sec/op     statistically significant (p < ALPHA) and slower by more than TIME_THRESHOLD %
#   allocs/op  median higher than before, by any amount, regardless of p
#   missing    a benchmark present in OLD has no result in NEW
#
# Allocation counts are deterministic for a given code path, so they are compared strictly and fail
# at once. Time is subject to machine noise, so it needs both significance and a margin, and gate and
# pair re-measure once when sec/op is the only failure: a time regression fails only when the same
# benchmark regresses again. B/op is reported but not gated: amortized buffer growth (WAL segments,
# pools) makes it fractional and noisy, and a new allocation already shows up in allocs/op. Results
# are only comparable when produced on the same machine: compare refuses files whose
# goos/goarch/cpu differ.
#
# Environment overrides: BENCH_GATE_LIST, BENCH_GATE_BASELINE, BENCH_GATE_COUNT (6),
# BENCH_GATE_BENCHTIME (200ms), BENCH_GATE_ALPHA (0.05), BENCH_GATE_TIME_THRESHOLD (15).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIST="${BENCH_GATE_LIST:-$ROOT/benchmarks/hotpaths.txt}"
BASELINE="${BENCH_GATE_BASELINE:-$ROOT/benchmarks/baseline.txt}"
COUNT="${BENCH_GATE_COUNT:-6}"
BENCHTIME="${BENCH_GATE_BENCHTIME:-200ms}"
ALPHA="${BENCH_GATE_ALPHA:-0.05}"
TIME_THRESHOLD="${BENCH_GATE_TIME_THRESHOLD:-15}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

log() { printf '%s\n' "$*" >&2; }
die() { log "bench-gate: $*"; exit 2; }

usage() {
	sed -n '3,9p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2
	exit 2
}

# benchstat is pinned in the devtools module, like the other dev tools.
benchstat_bin() {
	local bin="$WORK/benchstat"
	if [[ ! -x "$bin" ]]; then
		go -C "$ROOT/devtools" build -o "$bin" golang.org/x/perf/cmd/benchstat
	fi
	printf '%s' "$bin"
}

# Loads the list into the PKGS and REGEXES arrays.
load_list() {
	[[ -f "$LIST" ]] || die "benchmark list not found: $LIST"
	PKGS=()
	REGEXES=()
	local pkg re rest
	while read -r pkg re rest; do
		[[ -z "$pkg" || "$pkg" == \#* ]] && continue
		[[ -n "$re" && -z "$rest" ]] || die "malformed line in $LIST: $pkg $re $rest"
		[[ "$re" != */* ]] || die "regex for $pkg contains '/': $re"
		PKGS+=("$pkg")
		REGEXES+=("$re")
	done <"$LIST"
	[[ ${#PKGS[@]} -gt 0 ]] || die "benchmark list is empty: $LIST"
}

# run DIR=OUT [DIR=OUT ...]
#
# Compiles every listed package once per checkout, then runs COUNT rounds in which each package's
# benchmarks execute once per checkout, back to back. Interleaving spreads slow drift on the machine
# (thermal throttling, noisy neighbors) evenly over all checkouts instead of biasing whichever ran
# last. A package or benchmark missing from a checkout is skipped with a warning, except in the last
# checkout, the candidate, where it is an error: the list must match the code it gates.
cmd_run() {
	[[ $# -ge 1 ]] || usage
	load_list

	local dirs=() outs=() pair dir out
	for pair in "$@"; do
		[[ "$pair" == *=* ]] || usage
		dir="${pair%%=*}"
		out="${pair#*=}"
		[[ -d "$dir" ]] || die "not a directory: $dir"
		dirs+=("$(cd "$dir" && pwd)")
		outs+=("$out")
	done
	local last=$((${#dirs[@]} - 1))

	local i j bin
	for j in "${!dirs[@]}"; do
		for i in "${!PKGS[@]}"; do
			bin="$WORK/bin-$j-$i.test"
			if [[ ! -d "${dirs[j]}/${PKGS[i]}" ]]; then
				[[ $j -eq $last ]] && die "${PKGS[i]}: package not found in ${dirs[j]}"
				log "warning: ${PKGS[i]}: package not found in ${dirs[j]}, skipped"
				continue
			fi
			log "build ${dirs[j]}: ${PKGS[i]}"
			go -C "${dirs[j]}" test -c -o "$bin" "${PKGS[i]}"
		done
	done

	for j in "${!outs[@]}"; do
		: >"${outs[j]}"
	done

	local round chunk="$WORK/chunk.txt"
	for ((round = 1; round <= COUNT; round++)); do
		log "round $round/$COUNT"
		for i in "${!PKGS[@]}"; do
			for j in "${!dirs[@]}"; do
				bin="$WORK/bin-$j-$i.test"
				[[ -x "$bin" ]] || continue
				(cd "${dirs[j]}/${PKGS[i]}" && "$bin" -test.run '^$' -test.bench "${REGEXES[i]}" \
					-test.benchmem -test.benchtime "$BENCHTIME" -test.count 1) >"$chunk" ||
					{ cat "$chunk" >&2; die "${PKGS[i]}: benchmark run failed in ${dirs[j]}"; }
				if ! grep -q '^Benchmark' "$chunk"; then
					[[ $j -eq $last ]] && die "${PKGS[i]}: no benchmark matches ${REGEXES[i]} in ${dirs[j]}"
					[[ $round -eq 1 ]] && log "warning: ${PKGS[i]}: no benchmark matches ${REGEXES[i]} in ${dirs[j]}"
				fi
				grep -E '^(goos|goarch|pkg|cpu):|^Benchmark' "$chunk" >>"${outs[j]}" || true
			done
		done
	done
}

machine() {
	grep -E '^(goos|goarch|cpu):' "$1" | sort -u
}

# compare OLD NEW
cmd_compare() {
	[[ $# -eq 2 ]] || usage
	local old="$1" new="$2"
	[[ -s "$old" ]] || die "no benchmark results in $old"
	[[ -s "$new" ]] || die "no benchmark results in $new"
	if [[ "$(machine "$old")" != "$(machine "$new")" ]]; then
		log "bench-gate: $old and $new come from different machines:"
		diff <(machine "$old") <(machine "$new") >&2 || true
		die "regenerate the baseline on this machine first (make bench-baseline on the base commit)"
	fi

	local benchstat
	benchstat="$(benchstat_bin)"
	"$benchstat" -alpha "$ALPHA" "old=$old" "new=$new" 2>/dev/null || die "benchstat failed"
	"$benchstat" -alpha "$ALPHA" -format csv "old=$old" "new=$new" 2>/dev/null >"$WORK/stat.csv" ||
		die "benchstat failed"

	echo
	awk -F, -v tt="$TIME_THRESHOLD" '
		/^pkg: / { pkg = substr($0, 6); next }
		/^(goos|goarch|cpu): / || NF < 2 { next }
		# Column labels: a table holds both inputs, or only one of them when a whole
		# package exists on one side only.
		$1 == "" && $3 != "CI" { hasold = ($2 == "old"); hasnew = ($2 == "new" || $4 == "new"); next }
		$1 == "" && $3 == "CI" { unit = $2; next }
		$1 == "geomean" { next }
		{
			name = pkg "." $1; old = $2; cur = $4; delta = $6
			if (!hasold) { old = ""; cur = $2 }
			if (!hasnew) cur = ""
			if (unit != "sec/op" && unit != "allocs/op") next
			if (old == "") { if (unit == "sec/op") printf "new       %s\n", name; next }
			if (cur == "") { if (unit == "sec/op") { printf "MISSING   %s\n", name; bad++ }; next }
			pct = delta; sub(/%$/, "", pct)
			if (unit == "allocs/op" && cur + 0 > old + 0) {
				printf "REGRESSED %s allocs/op %s -> %s\n", name, old, cur; bad++
			} else if (unit == "sec/op" && delta != "~" && pct + 0 > tt) {
				printf "REGRESSED %s sec/op %s (%s, threshold +%s%%)\n", name, delta, $7, tt; bad++
			}
		}
		END {
			if (bad) { printf "bench-gate: %d regression(s)\n", bad; exit 1 }
			print "bench-gate: no regressions"
		}
	' "$WORK/stat.csv" | tee "$WORK/verdict.txt"
}

# Names of the benchmarks whose sec/op regressed in the last compare.
time_regressions() {
	awk '$1 == "REGRESSED" && $3 == "sec/op" { print $2 }' "$WORK/verdict.txt" | sort -u
}

# confirm MEASURE: calls MEASURE N, which must benchmark into fresh files and set OLD_FILE and
# NEW_FILE, then compares them. When sec/op is the only failure, measures and compares once more
# and fails only on the benchmarks that regressed both times.
confirm() {
	local measure="$1" rc=0
	"$measure" 1
	cmd_compare "$OLD_FILE" "$NEW_FILE" || rc=$?
	[[ $rc -eq 1 ]] || return "$rc"
	grep -qE '^MISSING|^REGRESSED .* allocs/op ' "$WORK/verdict.txt" && return 1

	time_regressions >"$WORK/first.txt"
	log ""
	log "bench-gate: only sec/op regressed; measuring again to rule out machine noise"
	"$measure" 2
	rc=0
	cmd_compare "$OLD_FILE" "$NEW_FILE" || rc=$?
	[[ $rc -eq 1 ]] || return "$rc"
	grep -qE '^MISSING|^REGRESSED .* allocs/op ' "$WORK/verdict.txt" && return 1

	local repeated
	repeated="$(time_regressions | comm -12 "$WORK/first.txt" -)"
	if [[ -z "$repeated" ]]; then
		echo "bench-gate: sec/op regressions did not reproduce, treated as noise"
		return 0
	fi
	echo "bench-gate: sec/op regressed in both measurements:"
	printf '  %s\n' "${repeated//$'\n'/$'\n'  }"
	return 1
}

measure_gate() {
	cmd_run "$ROOT=$WORK/current-$1.txt"
	OLD_FILE="$BASELINE"
	NEW_FILE="$WORK/current-$1.txt"
}

cmd_gate() {
	[[ -s "$BASELINE" ]] || die "no baseline at $BASELINE (run make bench-baseline)"
	confirm measure_gate
}

# pair BASE NEW OUTDIR
cmd_pair() {
	[[ $# -eq 3 ]] || usage
	PAIR_BASE="$1"
	PAIR_NEW="$2"
	PAIR_OUT="$3"
	mkdir -p "$PAIR_OUT"
	PAIR_OUT="$(cd "$PAIR_OUT" && pwd)"
	confirm measure_pair
}

measure_pair() {
	cmd_run "$PAIR_BASE=$PAIR_OUT/base-$1.txt" "$PAIR_NEW=$PAIR_OUT/new-$1.txt"
	OLD_FILE="$PAIR_OUT/base-$1.txt"
	NEW_FILE="$PAIR_OUT/new-$1.txt"
}

cmd_baseline() {
	cmd_run "$ROOT=$WORK/baseline.txt"
	mv "$WORK/baseline.txt" "$BASELINE"
	log "baseline written to $BASELINE"
}

[[ $# -ge 1 ]] || usage
command="$1"
shift
case "$command" in
run) cmd_run "$@" ;;
compare) cmd_compare "$@" ;;
gate) [[ $# -eq 0 ]] || usage; cmd_gate ;;
pair) cmd_pair "$@" ;;
baseline) [[ $# -eq 0 ]] || usage; cmd_baseline ;;
*) usage ;;
esac
