# Security policy

## Supported versions

Only the latest `master` (and the `ltesystem-dep:2.x` image line) receives
security fixes. The `1.x` line is end-of-life.

## Reporting a vulnerability

Open a **private security advisory** on GitHub (Security tab → Advisories),
or contact the maintainer directly. Do not open public issues for
vulnerabilities. We aim to acknowledge within 7 days.

## Known threat model (read before deploying)

- The tool API has **no authentication** — expose it on a trusted LAN only,
  never directly to the internet. Put a reverse proxy with auth in front if
  remote access is needed.
- `/start` runs privileged radio + iptables operations by design; anyone with
  API access fully controls the host's RF and NAT.
- RF transmission is regulated: only transmit on frequencies you are licensed
  or authorized to use, with suitable antennas and power. The authors accept
  no liability for unlawful transmission.
- This is a defensive/research lab tool. Do not use it against networks or
  devices you do not own or have explicit permission to test.
