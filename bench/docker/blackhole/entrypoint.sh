#!/bin/sh
set -e

# Drop all incoming TCP and ICMP traffic to simulate a blackhole.
# This makes the container unreachable for reachability probes, TCP connects,
# and DNS queries (if pointed at this IP).
iptables -A INPUT -p tcp -j DROP
iptables -A INPUT -p icmp -j DROP
iptables -A INPUT -p udp --dport 53 -j DROP

exec "$@"
