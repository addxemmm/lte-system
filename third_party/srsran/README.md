# srsRAN_4G NAS patches / NAS 补丁

These patches apply only to the official `srsRAN_4G release_23_11` commit
`eea87b1d893ae58e0b08bc381730c502024ae71f`.
这些补丁仅适用于官方 `srsRAN_4G release_23_11` 的上述完整提交。

## Scope / 范围

1. `0001-protected-pdn-reject.patch` fixes the post-attach, protected additional-PDN reject path: bounded extraction of the decrypted inner ESM fixed header, preserved PTI, security-header validation, protected/ciphered reject, one DL NAS COUNT advance, and full 32-bit UL/DL COUNT use for NAS ciphering.
   修复附着后额外 PDN 请求拒绝路径：有界提取已解密的内层 ESM 固定头、保留 PTI、校验安全头、加密并完整性保护拒绝响应、DL NAS COUNT 仅递增一次，并以完整 32 位 UL/DL COUNT 执行 NAS 加解密。
2. `0002-ipv4v6-fallback.patch` reports cause 50 when an initial IPv4v6 request is served as IPv4, without adding IPv6/IMS or additional-PDN support.
   初始 IPv4v6 请求降级为 IPv4 时返回 cause 50；不增加 IPv6、IMS 或额外 PDN 支持。

The tests use deterministic synthetic NAS fixtures and test-only keys; they contain no real subscriber keys. These NAS fixes do not address B210/VMware USB RF Late/Underflow/Overflow behavior.
测试使用确定性的合成 NAS 样本及测试专用密钥，不含真实用户密钥。这些 NAS 修复不解决 B210/VMware USB 的 RF Late/Underflow/Overflow。

## Apply and test / 应用与测试

Run from a clean checkout of the commit above. Copy this directory to `PATCH_DIR`, then:
从上述提交的干净检出目录执行；先将本目录复制为 `PATCH_DIR`：

```bash
mkdir -p srsepc/test
cp "$PATCH_DIR/test-CMakeLists.txt" srsepc/test/CMakeLists.txt
cp "$PATCH_DIR/"*_test.cc srsepc/test/

git apply --check "$PATCH_DIR/0001-protected-pdn-reject.patch"
git apply "$PATCH_DIR/0001-protected-pdn-reject.patch"
git apply --check "$PATCH_DIR/0002-ipv4v6-fallback.patch"
git apply "$PATCH_DIR/0002-ipv4v6-fallback.patch"
git apply --check "$PATCH_DIR/0003-strict-apn-session-safety.patch"
git apply "$PATCH_DIR/0003-strict-apn-session-safety.patch"

cmake -S . -B build -DCMAKE_BUILD_TYPE=Release \
  -DENABLE_SRSUE=OFF -DENABLE_SRSENB=OFF
cmake --build build --target srsepc nas_pdn_reject_test ipv4v6_fallback_test \
  strict_apn_test spgw_session_safety_test ue_snapshot_test \
  -j "${SRSRAN_BUILD_JOBS:-2}"
ctest --test-dir build --output-on-failure --no-tests=error \
  -R '^(nas_pdn_reject_test|ipv4v6_fallback_test|strict_apn_test|spgw_session_safety_test|ue_snapshot_test)$'
```

The upstream source and these derivative patches/tests are licensed under
**GNU AGPL-3.0-or-later**. See `LICENSE` for the preserved upstream license text.
上游源码及这些派生补丁/测试采用 **GNU AGPL-3.0-or-later**；完整上游许可原文见 `LICENSE`。

## 0003: multi-UE session safety / 多 UE 会话一致性

Apply `0003-strict-apn-session-safety.patch` after 0001/0002 at the same pinned commit. It changes NAS/MME/S1AP/SPGW only; no firmware, RF, SIM keys/SQN, DNS or Linux firewall settings are changed.

- Explicit APNs match the configured complete name case-insensitively. Omitted APN uses the default; present empty/malformed values never fall back. The bounded inner-ESM walker checks label lengths, duplicates, truncation and supported IE formats; the outer Attach envelope is checked before legacy ESM-container copying. EIT requires both presence and the REQUIRED value. Initial rejection is EMM 19 with embedded ESM 27 and original PTI; a protected post-attach wrong-APN request returns ESM 27 without deleting the established default bearer. Matching additional PDNs retain cause 32.
- Supported name syntax: ASCII alphanumeric labels with internal hyphens; labels 1–63 characters; display name ≤99 (encoded ≤100). This does not infer APN-NI/APN-OI components or claim every 3GPP naming restriction. Unsupported ambiguous optional encodings fail closed.
- One SGi `/24`, gateway `.1`, dynamic `.2–.254` minus static reservations. Static addresses must be in the same subnet and exclude `.0/.1/.255`; duplicate static IMSIs fail initialization, duplicate static IPs remain rejected by HSS. No native multiple APNs/pools/VRFs or IPv6 is added.
- Dynamic IPs return only on session deletion/replacement/failed response rollback. Release Access Bearers/ECM idle retains the IP; Modify Bearer restores its uplink TEID binding. Exhaustion returns GTP cause 84 and ESM insufficient-resources rejection, not a zero-address success.
- GTP-U checks base GTPv1-U framing, IPv4 length/checksum and assigned uplink TEID→source IP before TUN write. Unknown/released TEIDs, source spoofing and malformed packets are dropped. GTP extension/sequence-header variants are intentionally rejected in this single-EPC scope. Linux FORWARD/INPUT policy still supplies actual network segmentation and management-access restriction.

### Current-state snapshot contract

Both `LTE_UE_SNAPSHOT_PATH` and `LTE_UE_RUN_ID` enable publication. The launcher must provide a new unpredictable run ID for each EPC start; consumers must match the current process/run and freshness. No listener or packet capture is added.

The MME owner loop serializes pointer-deduplicated NAS maps after changed control-plane batches and on a 1-second idle heartbeat. No packet-thread traversal or per-packet disk writes occur. Shutdown joins this thread before freeing maps and publishes an empty stopped snapshot. S1AP release/delete removes obsolete pointer aliases.

Same-directory exclusive temporary files have mode 0600. Close+atomic rename publishes whole JSON. Write failure, invalid metadata or >256 contexts leaves the preceding file unchanged so consumers report stale rather than partial current state. A crash may leave a temporary file, which is never consumed as the snapshot.

Root fields: `schema_version=1`, `run_id`, `pid`, `started_at_unix_ms`, `updated_at_unix_ms`, `sequence`, `state` (`running`/`stopped`), `sessions` array.
Session fields: `session_id` (`run_id:context_generation`), `imsi` (15 digits or empty before identification), `mme_ue_s1ap_id`, `enb_ue_s1ap_id`, `sctp_assoc_id`, `emm_state`, `ecm_state`, `ue_ipv4`, `requested_apn`, `apn_source`, `selected_apn`, `apn_validated`, `bearers` array (`ebi`, `qci`, `state`). QCI 0 in deactivated/requested state means unknown; no default is invented. Reset/re-attach changes generation. Registered+idle still means attached, not necessarily connected or Internet-reachable. No keys, authentication vectors, NAS counters, passwords or raw packets are exported.

CPU tests use synthetic identities/keys, spies and pipe/file fixtures without RF, TUN creation or server connections. Coverage includes Attach branches/APN boundaries/protected reject counts, >253 allocation/free cycles, exhaustion/static exclusions, idle/resume ownership, source spoofing, two-UE/reset/remove snapshots, leading-zero IMSIs and atomic publication. The eNB compile-time 64-context ceiling is not a claim of 64 tested over-the-air UEs.

Standards: [TS 23.003 §9.1](https://www.etsi.org/deliver/etsi_ts/123000_123099/123003/16.11.01_60/ts_123003v161101p.pdf), [TS 23.401 default APN](https://www.etsi.org/deliver/etsi_ts/123400_123499/123401/16.12.00_60/ts_123401v161200p.pdf), [TS 24.301 Attach/ESM rejection](https://www.etsi.org/deliver/etsi_ts/124300_124399/124301/13.04.00_60/ts_124301v130400p.pdf).
