#!/bin/sh
set -e

# MODE controls what traffic is dropped:
#   "all"      — drop TCP, ICMP, UDP:53 (full blackhole, for reachability_timeout)
#   "tcp-only" — drop TCP and UDP:53 only, allow ICMP (for tcp_timeout)
MODE="${BLACKHOLE_MODE:-all}"

iptables -A INPUT -p tcp -j DROP
iptables -A INPUT -p udp --dport 53 -j DROP

if [ "$MODE" = "all" ]; then
  iptables -A INPUT -p icmp -j DROP
fi

exec "$@"
