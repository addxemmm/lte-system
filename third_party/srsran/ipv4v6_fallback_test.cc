/*
 * Regression test for the srsEPC IPv4v6-to-IPv4 fallback indication.
 * This invokes nas::pack_attach_accept and decodes its real NAS output.
 */
#include "srsepc/hdr/mme/nas.h"
#include "srsran/asn1/liblte_mme.h"
#include "srsran/srslog/srslog.h"

#include <arpa/inet.h>
#include <cstring>
#include <iostream>

#define TESTASSERT(cond)                                                                                               \
  do {                                                                                                                 \
    if (!(cond)) {                                                                                                     \
      std::cerr << __FILE__ << ':' << __LINE__ << ": assertion failed: " #cond << std::endl;                          \
      return false;                                                                                                    \
    }                                                                                                                  \
  } while (false)

namespace {

class test_s1ap final : public srsepc::s1ap_interface_nas
{
public:
  uint32_t allocate_m_tmsi(uint64_t) override { return 0x12345678; }
  uint32_t get_next_mme_ue_s1ap_id() override { return 1; }
  bool     add_nas_ctx_to_imsi_map(srsepc::nas*) override { return true; }
  bool     add_nas_ctx_to_mme_ue_s1ap_id_map(srsepc::nas*) override { return true; }
  bool     add_ue_to_enb_set(int32_t, uint32_t) override { return true; }
  bool     release_ue_ecm_ctx(uint32_t) override { return true; }
  bool     delete_ue_ctx(uint64_t) override { return true; }
  uint64_t find_imsi_from_m_tmsi(uint32_t) override { return 0; }
  srsepc::nas* find_nas_ctx_from_imsi(uint64_t) override { return nullptr; }
  bool         send_initial_context_setup_request(uint64_t, uint16_t) override { return true; }
  bool         send_ue_context_release_command(uint32_t) override { return true; }
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
  bool send_downlink_nas_transport(uint32_t, uint32_t, srsran::byte_buffer_t*, struct sctp_sndrcvinfo) override
  {
    return true;
  }
};

srsepc::nas_init_t make_nas_args()
{
  srsepc::nas_init_t args = {};
  args.mcc                = 0xF001;
  args.mnc                = 0xFF01;
  args.mme_code           = 0x1a;
  args.mme_group          = 1;
  args.tac                = 7;
  args.apn                = "test";
  args.dns                = "8.8.8.8";
  args.cipher_algo        = srsran::CIPHERING_ALGORITHM_ID_EEA0;
  args.integ_algo         = srsran::INTEGRITY_ALGORITHM_ID_EIA0;
  return args;
}

bool decode_packed_default_bearer(srsepc::nas& ctx,
                                  LIBLTE_MME_ACTIVATE_DEFAULT_EPS_BEARER_CONTEXT_REQUEST_MSG_STRUCT* bearer)
{
  auto packed = srsran::make_byte_buffer();
  TESTASSERT(packed != nullptr);
  TESTASSERT(ctx.pack_attach_accept(packed.get()));

  LIBLTE_MME_ATTACH_ACCEPT_MSG_STRUCT attach_accept = {};
  TESTASSERT(liblte_mme_unpack_attach_accept_msg(reinterpret_cast<LIBLTE_BYTE_MSG_STRUCT*>(packed.get()),
                                                 &attach_accept) == LIBLTE_SUCCESS);
  TESTASSERT(liblte_mme_unpack_activate_default_eps_bearer_context_request_msg(&attach_accept.esm_msg, bearer) ==
             LIBLTE_SUCCESS);
  return true;
}

bool decode_default_bearer(uint8_t requested_pdn_type,
                           LIBLTE_MME_ACTIVATE_DEFAULT_EPS_BEARER_CONTEXT_REQUEST_MSG_STRUCT* bearer)
{
  test_s1ap       s1ap;
  srsepc::nas_if_t itf = {};
  itf.s1ap              = &s1ap;
  srsepc::nas ctx(make_nas_args(), itf);
  ctx.m_emm_ctx.imsi                     = 100100000000001;
  ctx.m_emm_ctx.attach_type              = 1;
  ctx.m_emm_ctx.procedure_transaction_id = 7;
  ctx.m_emm_ctx.requested_pdn_type        = requested_pdn_type;
  ctx.m_emm_ctx.ue_ip.s_addr              = inet_addr("172.16.0.2");
  ctx.m_esm_ctx[5].qci                    = 9;
  return decode_packed_default_bearer(ctx, bearer);
}

bool test_ipv4v6_fallback_cause()
{
  LIBLTE_MME_ACTIVATE_DEFAULT_EPS_BEARER_CONTEXT_REQUEST_MSG_STRUCT bearer = {};
  TESTASSERT(decode_default_bearer(LIBLTE_MME_PDN_TYPE_IPV4V6, &bearer));
  TESTASSERT(bearer.pdn_addr.pdn_type == LIBLTE_MME_PDN_TYPE_IPV4);
  TESTASSERT(bearer.esm_cause_present);
  TESTASSERT(bearer.esm_cause == LIBLTE_MME_ESM_CAUSE_PDN_TYPE_IPV4_ONLY_ALLOWED);
  return true;
}

bool test_other_requests_keep_original_behavior()
{
  for (uint8_t requested : {uint8_t{0}, uint8_t{LIBLTE_MME_PDN_TYPE_IPV4}}) {
    LIBLTE_MME_ACTIVATE_DEFAULT_EPS_BEARER_CONTEXT_REQUEST_MSG_STRUCT bearer = {};
    TESTASSERT(decode_default_bearer(requested, &bearer));
    TESTASSERT(bearer.pdn_addr.pdn_type == LIBLTE_MME_PDN_TYPE_IPV4);
    TESTASSERT(!bearer.esm_cause_present);
  }
  return true;
}

bool test_reset_clears_attach_request_type()
{
  test_s1ap        s1ap;
  srsepc::nas_if_t itf = {};
  itf.s1ap              = &s1ap;
  srsepc::nas ctx(make_nas_args(), itf);
  ctx.m_emm_ctx.requested_pdn_type = LIBLTE_MME_PDN_TYPE_IPV4V6;
  ctx.reset();
  TESTASSERT(ctx.m_emm_ctx.requested_pdn_type == 0);
  return true;
}

bool test_reattach_overwrites_fallback_state()
{
  test_s1ap        s1ap;
  srsepc::nas_if_t itf = {};
  itf.s1ap              = &s1ap;
  srsepc::nas ctx(make_nas_args(), itf);
  ctx.m_emm_ctx.imsi                     = 100100000000001;
  ctx.m_emm_ctx.attach_type              = 1;
  ctx.m_emm_ctx.procedure_transaction_id = 7;
  ctx.m_emm_ctx.ue_ip.s_addr              = inet_addr("172.16.0.2");
  ctx.m_esm_ctx[5].qci                    = 9;

  LIBLTE_MME_ACTIVATE_DEFAULT_EPS_BEARER_CONTEXT_REQUEST_MSG_STRUCT bearer = {};
  ctx.m_emm_ctx.requested_pdn_type = LIBLTE_MME_PDN_TYPE_IPV4V6;
  TESTASSERT(decode_packed_default_bearer(ctx, &bearer));
  TESTASSERT(bearer.esm_cause_present);

  bearer = {};
  ctx.m_emm_ctx.requested_pdn_type = LIBLTE_MME_PDN_TYPE_IPV4;
  TESTASSERT(decode_packed_default_bearer(ctx, &bearer));
  TESTASSERT(!bearer.esm_cause_present);
  return true;
}

} // namespace

int main()
{
  srslog::init();
  if (!test_ipv4v6_fallback_cause() || !test_other_requests_keep_original_behavior() ||
      !test_reset_clears_attach_request_type() || !test_reattach_overwrites_fallback_state()) {
    return 1;
  }
  return 0;
}
