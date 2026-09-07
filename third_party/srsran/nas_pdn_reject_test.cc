/**
 * Regression test for a protected, post-attach PDN Connectivity Request.
 * The inner request bytes mirror the captured ESM payload: EBI 0, PTI 11,
 * IPv6, initial request, APN addxLTE. No subscriber secrets are used.
 */

#include "srsepc/hdr/mme/nas.h"
#include "srsran/common/security.h"
#include "srsran/common/test_common.h"
#include <algorithm>
#include <cstring>
#include <map>
#include <vector>

using namespace srsepc;

namespace {

class s1ap_spy final : public s1ap_interface_nas
{
public:
  uint32_t allocate_m_tmsi(uint64_t) override { return 1; }
  uint32_t get_next_mme_ue_s1ap_id() override { return 1; }
  bool     add_nas_ctx_to_imsi_map(nas*) override { return true; }
  bool     add_nas_ctx_to_mme_ue_s1ap_id_map(nas*) override { return true; }
  bool     add_ue_to_enb_set(int32_t, uint32_t) override { return true; }
  bool     release_ue_ecm_ctx(uint32_t) override { return true; }
  bool     delete_ue_ctx(uint64_t) override { return true; }
  uint64_t find_imsi_from_m_tmsi(uint32_t) override { return 0; }
  nas*     find_nas_ctx_from_imsi(uint64_t) override { return nullptr; }
  bool     send_initial_context_setup_request(uint64_t, uint16_t) override { return true; }
  bool     send_ue_context_release_command(uint32_t) override { return true; }
  bool send_erab_release_command(uint32_t, uint32_t, std::vector<uint16_t>, struct sctp_sndrcvinfo) override
  {
    return true;
  }
  bool send_erab_modify_request(uint32_t,
                                uint32_t,
                                std::map<uint16_t, uint16_t>,
                                srsran::byte_buffer_t*,
                                struct sctp_sndrcvinfo) override
  {
    return true;
  }
  bool send_downlink_nas_transport(uint32_t,
                                   uint32_t,
                                   srsran::byte_buffer_t* nas_msg,
                                   struct sctp_sndrcvinfo) override
  {
    ++send_count;
    downlink.assign(nas_msg->msg, nas_msg->msg + nas_msg->N_bytes);
    return true;
  }

  unsigned             send_count = 0;
  std::vector<uint8_t> downlink;
};

const uint8_t captured_inner_request[] = {
    0x02, 0x0b, 0xd0, 0x21, 0x28, 0x08, 0x07, 0x61, 0x64, 0x64, 0x78, 0x4c, 0x54, 0x45};

void set_keys(nas& ctx)
{
  for (unsigned i = 0; i != 32; ++i) {
    ctx.m_sec_ctx.k_nas_enc[i] = static_cast<uint8_t>(0x10 + i);
    ctx.m_sec_ctx.k_nas_int[i] = static_cast<uint8_t>(0x80 + i);
  }
}

srsran::unique_byte_buffer_t make_uplink(nas&    ctx,
                                         uint32_t count,
                                         bool     encrypt,
                                         bool     valid_mac  = true,
                                         uint8_t  pti        = 11,
                                         uint8_t  first_byte = 0x02,
                                         uint8_t  msg_type   = LIBLTE_MME_MSG_TYPE_PDN_CONNECTIVITY_REQUEST)
{
  auto pdu = srsran::make_byte_buffer();
  TESTASSERT(pdu != nullptr);
  uint8_t inner[sizeof(captured_inner_request)] = {};
  memcpy(inner, captured_inner_request, sizeof(inner));
  inner[0] = first_byte;
  inner[1] = pti;
  inner[2] = msg_type;

  pdu->N_bytes = 6 + sizeof(captured_inner_request);
  pdu->msg[0]  = (LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED << 4) |
                LIBLTE_MME_PD_EPS_MOBILITY_MANAGEMENT;
  memset(&pdu->msg[1], 0, 4);
  pdu->msg[5] = count & 0xff;
  memcpy(&pdu->msg[6], inner, sizeof(inner));

  if (encrypt) {
    uint8_t encrypted[sizeof(captured_inner_request)] = {};
    srsran::security_128_eea2(&ctx.m_sec_ctx.k_nas_enc[16],
                              count,
                              0,
                              srsran::SECURITY_DIRECTION_UPLINK,
                              &pdu->msg[6],
                              sizeof(inner),
                              encrypted);
    memcpy(&pdu->msg[6], encrypted, sizeof(encrypted));
  }

  uint8_t mac[4] = {};
  srsran::security_128_eia2(&ctx.m_sec_ctx.k_nas_int[16],
                            count,
                            0,
                            srsran::SECURITY_DIRECTION_UPLINK,
                            &pdu->msg[5],
                            pdu->N_bytes - 5,
                            mac);
  memcpy(&pdu->msg[1], mac, sizeof(mac));
  if (!valid_mac) {
    pdu->msg[1] ^= 0x80;
  }

  // Make the historical msg-pointer-as-struct bug deterministic.
  memset(&pdu->msg[pdu->N_bytes], 0xa5, std::min<uint32_t>(2048, pdu->get_tailroom()));
  return pdu;
}

int check_reject_round_trip(srsran::CIPHERING_ALGORITHM_ID_ENUM cipher_algo, uint8_t pti)
{
  s1ap_spy spy;
  nas_init_t args = {};
  args.apn = "addxLTE";
  args.cipher_algo = cipher_algo;
  args.integ_algo  = srsran::INTEGRITY_ALGORITHM_ID_128_EIA2;
  nas_if_t interfaces = {};
  interfaces.s1ap     = &spy;
  nas ctx(args, interfaces);
  set_keys(ctx);

  const uint32_t ul_count = 0x107;
  const uint32_t dl_count = cipher_algo == srsran::CIPHERING_ALGORITHM_ID_128_EEA2 ? 0xff : 0x31;
  ctx.m_sec_ctx.ul_nas_count = ul_count;
  ctx.m_sec_ctx.dl_nas_count = dl_count;
  ctx.m_esm_ctx[5].state     = ERAB_ACTIVE;
  ctx.m_esm_ctx[5].erab_id   = 5;
  ctx.m_emm_ctx.ue_ip.s_addr = 0x01020304;

  auto uplink = make_uplink(ctx, ul_count, cipher_algo == srsran::CIPHERING_ALGORITHM_ID_128_EEA2, true, pti);
  TESTASSERT(ctx.integrity_check(uplink.get()));
  ctx.cipher_decrypt(uplink.get());
  TESTASSERT(uplink->msg[6] == captured_inner_request[0]);
  TESTASSERT(uplink->msg[7] == pti);
  TESTASSERT(memcmp(&uplink->msg[8], &captured_inner_request[2], sizeof(captured_inner_request) - 2) == 0);
  TESTASSERT(ctx.handle_pdn_connectivity_request(
      uplink.get(), LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED, true));

  TESTASSERT(spy.send_count == 1);
  TESTASSERT(spy.downlink.size() == 10);
  TESTASSERT(spy.downlink[0] == ((LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED << 4) |
                                LIBLTE_MME_PD_EPS_MOBILITY_MANAGEMENT));
  TESTASSERT(spy.downlink[5] == ((dl_count + 1) & 0xff));
  TESTASSERT(ctx.m_sec_ctx.dl_nas_count == dl_count + 1);

  uint8_t expected_mac[4] = {};
  srsran::security_128_eia2(&ctx.m_sec_ctx.k_nas_int[16],
                            dl_count + 1,
                            0,
                            srsran::SECURITY_DIRECTION_DOWNLINK,
                            &spy.downlink[5],
                            spy.downlink.size() - 5,
                            expected_mac);
  TESTASSERT(memcmp(&spy.downlink[1], expected_mac, sizeof(expected_mac)) == 0);
  TESTASSERT(expected_mac[0] != 0 || expected_mac[1] != 0 || expected_mac[2] != 0 || expected_mac[3] != 0);

  uint8_t decoded[4] = {};
  if (cipher_algo == srsran::CIPHERING_ALGORITHM_ID_128_EEA2) {
    srsran::security_128_eea2(&ctx.m_sec_ctx.k_nas_enc[16],
                              dl_count + 1,
                              0,
                              srsran::SECURITY_DIRECTION_DOWNLINK,
                              &spy.downlink[6],
                              4,
                              decoded);
  } else {
    memcpy(decoded, &spy.downlink[6], sizeof(decoded));
  }

  LIBLTE_BYTE_MSG_STRUCT inner_reject = {};
  inner_reject.N_bytes                = sizeof(decoded);
  memcpy(inner_reject.msg, decoded, sizeof(decoded));
  LIBLTE_MME_PDN_CONNECTIVITY_REJECT_MSG_STRUCT reject = {};
  TESTASSERT(liblte_mme_unpack_pdn_connectivity_reject_msg(&inner_reject, &reject) == LIBLTE_SUCCESS);
  TESTASSERT(reject.eps_bearer_id == 0);
  TESTASSERT(reject.proc_transaction_id == pti);
  TESTASSERT(reject.esm_cause == LIBLTE_MME_ESM_CAUSE_SERVICE_OPTION_NOT_SUPPORTED);

  TESTASSERT(ctx.m_esm_ctx[5].state == ERAB_ACTIVE);
  TESTASSERT(ctx.m_esm_ctx[5].erab_id == 5);
  TESTASSERT(ctx.m_emm_ctx.ue_ip.s_addr == 0x01020304);
  return SRSRAN_SUCCESS;
}

int check_malformed_inputs_are_not_sent()
{
  s1ap_spy spy;
  nas_init_t args = {};
  args.apn = "addxLTE";
  args.cipher_algo = srsran::CIPHERING_ALGORITHM_ID_EEA0;
  args.integ_algo  = srsran::INTEGRITY_ALGORITHM_ID_128_EIA2;
  nas_if_t interfaces = {};
  interfaces.s1ap     = &spy;
  nas ctx(args, interfaces);
  set_keys(ctx);
  ctx.m_sec_ctx.dl_nas_count = 9;

  auto truncated = srsran::make_byte_buffer();
  TESTASSERT(truncated != nullptr);
  truncated->N_bytes = 9;
  truncated->msg[0]  = 0x27;
  TESTASSERT(!ctx.handle_pdn_connectivity_request(
      truncated.get(), LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED, true));

  ctx.m_sec_ctx.ul_nas_count = 3;
  auto wrong_type = make_uplink(ctx, 3, false, true, 11, 0x02, LIBLTE_MME_MSG_TYPE_PDN_DISCONNECT_REQUEST);
  TESTASSERT(ctx.integrity_check(wrong_type.get()));
  TESTASSERT(!ctx.handle_pdn_connectivity_request(
      wrong_type.get(), LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED, true));

  ctx.m_sec_ctx.ul_nas_count = 4;
  auto wrong_pd = make_uplink(ctx, 4, false, true, 11, 0x07);
  TESTASSERT(ctx.integrity_check(wrong_pd.get()));
  TESTASSERT(!ctx.handle_pdn_connectivity_request(
      wrong_pd.get(), LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED, true));

  auto plain = make_uplink(ctx, 3, false);
  plain->msg[0] = LIBLTE_MME_PD_EPS_MOBILITY_MANAGEMENT;
  TESTASSERT(!ctx.handle_pdn_connectivity_request(plain.get(), LIBLTE_MME_SECURITY_HDR_TYPE_PLAIN_NAS, true));

  TESTASSERT(spy.send_count == 0);
  TESTASSERT(ctx.m_sec_ctx.dl_nas_count == 9);
  return SRSRAN_SUCCESS;
}

int check_invalid_mac_is_not_dispatched()
{
  s1ap_spy spy;
  nas_init_t args = {};
  args.apn = "addxLTE";
  args.cipher_algo = srsran::CIPHERING_ALGORITHM_ID_EEA0;
  args.integ_algo  = srsran::INTEGRITY_ALGORITHM_ID_128_EIA2;
  nas_if_t interfaces = {};
  interfaces.s1ap     = &spy;
  nas ctx(args, interfaces);
  set_keys(ctx);
  ctx.m_sec_ctx.ul_nas_count = 4;

  auto invalid = make_uplink(ctx, 4, false, false);
  const bool mac_valid = ctx.integrity_check(invalid.get(), false);
  TESTASSERT(!mac_valid);
  TESTASSERT(!ctx.handle_pdn_connectivity_request(
      invalid.get(), LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED, mac_valid));
  TESTASSERT(spy.send_count == 0);
  return SRSRAN_SUCCESS;
}

} // namespace

int main(int argc, char** argv)
{
  srsran::test_init(argc, argv);
  TESTASSERT(check_reject_round_trip(srsran::CIPHERING_ALGORITHM_ID_EEA0, 0x5a) == SRSRAN_SUCCESS);
  TESTASSERT(check_reject_round_trip(srsran::CIPHERING_ALGORITHM_ID_128_EEA2, 11) == SRSRAN_SUCCESS);
  TESTASSERT(check_malformed_inputs_are_not_sent() == SRSRAN_SUCCESS);
  TESTASSERT(check_invalid_mac_is_not_dispatched() == SRSRAN_SUCCESS);
  return SRSRAN_SUCCESS;
}
