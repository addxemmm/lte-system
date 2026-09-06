# 安全政策 Security Policy

安全政策。
Security policy.

## 支持的版本 Supported Versions

仅最新的 `master`（以及 `ltesystem-dep:2.x` 镜像线）接收安全修复。`1.x` 线已终止支持。
Only the latest `master` (and the `ltesystem-dep:2.x` image line) receives security fixes. The `1.x` line is end-of-life.

## 报告漏洞 Reporting a Vulnerability

报告漏洞。
Reporting a vulnerability.

在 GitHub 上开 **private security advisory**（Security 选项卡 → Advisories），或直接联系维护者。漏洞不要开公开 issue。我们目标 7 天内确认。
Open a **private security advisory** on GitHub (Security tab → Advisories), or contact the maintainer directly. Do not open public issues for vulnerabilities. We aim to acknowledge within 7 days.

## 已知威胁模型 Known Threat Model

部署deploy前请先阅读。
Read before deploying.

- 工具 API **无鉴权** — 仅在可信局域网暴露，绝不要直接暴露到公网。如需远程访问，请在前面加带鉴权的反向代理。 / The tool API has **no authentication** — expose it on a trusted LAN only, never directly to the internet. Put a reverse proxy with auth in front if remote access is needed.
- `/start` 按设计运行特权射频 + iptables 操作；任何有 API 访问权限的人完全控制主机的射频与 NAT。 / `/start` runs privileged radio + iptables operations by design; anyone with API access fully controls the host's RF and NAT.
- 射频发射受法规管制：仅在已许可或已授权的频点上发射，使用合适的天线与功率。非法发射作者不承担责任。 / RF transmission is regulated: only transmit on frequencies you are licensed or authorized to use, with suitable antennas and power. The authors accept no liability for unlawful transmission.
- 这是防御/研究实验工具。不要用于你不拥有或未经明确许可的网络或设备。 / This is a defensive/research lab tool. Do not use it against networks or devices you do not own or have explicit permission to test.
