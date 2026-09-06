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
cp "$PATCH_DIR/nas_pdn_reject_test.cc" "$PATCH_DIR/ipv4v6_fallback_test.cc" srsepc/test/

git apply --check "$PATCH_DIR/0001-protected-pdn-reject.patch"
git apply "$PATCH_DIR/0001-protected-pdn-reject.patch"
git apply --check "$PATCH_DIR/0002-ipv4v6-fallback.patch"
git apply "$PATCH_DIR/0002-ipv4v6-fallback.patch"

cmake -S . -B build -DCMAKE_BUILD_TYPE=Release \
  -DENABLE_SRSUE=OFF -DENABLE_SRSENB=OFF
cmake --build build --target nas_pdn_reject_test ipv4v6_fallback_test \
  -j "${SRSRAN_BUILD_JOBS:-2}"
ctest --test-dir build --output-on-failure --no-tests=error \
  -R '^(nas_pdn_reject_test|ipv4v6_fallback_test)$'
```

The upstream source and these derivative patches/tests are licensed under
**GNU AGPL-3.0-or-later**. See `LICENSE` for the preserved upstream license text.
上游源码及这些派生补丁/测试采用 **GNU AGPL-3.0-or-later**；完整上游许可原文见 `LICENSE`。
