#include "srsepc/hdr/mme/apn_policy.h"
#include "srsepc/hdr/mme/nas.h"
#include "srsran/common/test_common.h"
#include <cstring>
#include <set>
#include <vector>
using namespace srsepc;
namespace {
class s1ap_spy final : public s1ap_interface_nas
{
public:
  uint32_t allocate_m_tmsi(uint64_t) override { return 1; }
  uint32_t get_next_mme_ue_s1ap_id() override { return ++next_id; }
  bool     add_nas_ctx_to_imsi_map(nas* n) override { live.insert(n); last = n; return true; }
  bool     add_nas_ctx_to_mme_ue_s1ap_id_map(nas* n) override { live.insert(n); last = n; return true; }
  bool     add_ue_to_enb_set(int32_t, uint32_t) override { return true; }
  bool     release_ue_ecm_ctx(uint32_t) override { return true; }
  bool delete_ue_ctx(uint64_t imsi) override {
    for (auto i = live.begin(); i != live.end(); ++i) if ((*i)->m_emm_ctx.imsi == imsi) {
      delete *i; live.erase(i); return true;
    }
    return false;
  }
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

  ~s1ap_spy() { for (auto n : live) delete n; }
  std::set<nas*> live;
  nas* last = nullptr;
  uint32_t next_id = 0;
  unsigned             send_count = 0;
  std::vector<uint8_t> downlink;
};


class gtpc_spy : public gtpc_interface_nas {
public:
  unsigned creates = 0, deletes = 0;
  bool send_create_session_request(uint64_t) override { ++creates; return true; }
  bool send_delete_session_request(uint64_t) override { ++deletes; return true; }
  bool send_modify_bearer_request(uint64_t, uint16_t, srsran::gtp_fteid_t*) override { return true; }
  bool send_downlink_data_notification_failure_indication(uint64_t, srsran::gtpc_cause_value) override { return true; }
};
class hss_spy : public hss_interface_nas {
public:
  bool gen_auth_info_answer(uint64_t, uint8_t* k, uint8_t* a, uint8_t* r, uint8_t* x) override {
    memset(k, 0, 32); memset(a, 0, 16); memset(r, 0, 16); memset(x, 0, 16); return true;
  }
  bool gen_update_loc_answer(uint64_t, uint8_t* qci) override { *qci = 9; return true; }
  bool resync_sqn(uint64_t, uint8_t*) override { return true; }
};
nas_init_t args() {
  nas_init_t a{}; a.apn = "addxLTE";
  a.cipher_algo = srsran::CIPHERING_ALGORITHM_ID_EEA0;
  a.integ_algo = srsran::INTEGRITY_ALGORITHM_ID_128_EIA2;
  return a;
}
LIBLTE_MME_PDN_CONNECTIVITY_REQUEST_MSG_STRUCT request(const char* apn, bool eit = false) {
  LIBLTE_MME_PDN_CONNECTIVITY_REQUEST_MSG_STRUCT p{};
  p.proc_transaction_id = 11; p.pdn_type = LIBLTE_MME_PDN_TYPE_IPV4;
  p.request_type = 1; p.apn_present = apn != nullptr;
  if (apn) strcpy(p.apn.apn, apn);
  p.esm_info_transfer_flag_present = true;
  p.esm_info_transfer_flag = eit ? LIBLTE_MME_ESM_INFO_TRANSFER_FLAG_REQUIRED : LIBLTE_MME_ESM_INFO_TRANSFER_FLAG_NOT_REQUIRED;
  return p;
}
int parser_tests() {
  std::string normalized;
  TESTASSERT(apn_policy::normalize("AdDxLTE", normalized) && normalized == "addxlte");
  for (const char* bad : {"", ".a", "a.", "a..b", "-a", "a-", "a_b", "a b", "*"})
    TESTASSERT(!apn_policy::normalize(bad, normalized));
  TESTASSERT(!apn_policy::normalize(std::string(64, 'a'), normalized));
  TESTASSERT(apn_policy::normalize(std::string(63, 'a') + "." + std::string(35, 'b'), normalized));
  TESTASSERT(!apn_policy::normalize(std::string(63, 'a') + "." + std::string(36, 'b'), normalized));
  LIBLTE_BYTE_MSG_STRUCT bytes{};
  auto p = request("addxLTE");
  TESTASSERT(liblte_mme_pack_pdn_connectivity_request_msg(&p, &bytes) == LIBLTE_SUCCESS);
  apn_policy::request parsed;
  TESTASSERT(apn_policy::parse(bytes.msg, bytes.N_bytes, false, parsed));
  TESTASSERT(parsed.present && !parsed.eit && parsed.apn == "addxlte");
  TESTASSERT(apn_policy::matches(false, "", "addxLTE"));
  TESTASSERT(!apn_policy::matches(true, "", "addxLTE"));
  TESTASSERT(!apn_policy::matches(true, "addxLTE111", "addxLTE"));
  for (unsigned cut = 0; cut < 4; ++cut) TESTASSERT(!apn_policy::parse(bytes.msg, cut, false, parsed));
  for (const auto& v : std::vector<std::vector<uint8_t>>{
    {2,11,0xd0,0x11,0x28}, {2,11,0xd0,0x11,0x28,0},
    {2,11,0xd0,0x11,0x28,2,3,'a'}, {2,11,0xd0,0x11,0x28,1,0},
    {2,11,0xd0,0x11,0x28,2,1,'a',0x28,2,1,'a'},
    {2,11,0xd0,0x11,0x7b,0xff,0xff}, {2,11,0xd0,0x11,0xd1,0xd0}})
    TESTASSERT(!apn_policy::parse(v.data(), v.size(), false, parsed));
  LIBLTE_MME_ATTACH_REQUEST_MSG_STRUCT attach{};
  attach.eps_mobile_id.type_of_id=LIBLTE_MME_EPS_MOBILE_ID_TYPE_IMSI;
  attach.eps_mobile_id.imsi[0]=1; attach.eps_mobile_id.imsi[14]=1;
  attach.eps_attach_type=1; attach.esm_msg=bytes;
  LIBLTE_BYTE_MSG_STRUCT encoded{};
  TESTASSERT(liblte_mme_pack_attach_request_msg(&attach,&encoded)==LIBLTE_SUCCESS);
  TESTASSERT(apn_policy::validate_attach(encoded.msg,encoded.N_bytes));
  for (unsigned cut=0; cut<encoded.N_bytes; ++cut)
    TESTASSERT(!apn_policy::validate_attach(encoded.msg,cut));
  encoded.msg[0]=0x17;
  TESTASSERT(!apn_policy::validate_attach(encoded.msg,3));
  return 0;
}
int decision_tests() {
  s1ap_spy s; gtpc_spy g; hss_spy h;
  nas_if_t it{}; it.s1ap=&s; it.gtpc=&g; it.hss=&h;
  nas n(args(), it);
  auto bad=request("addxLTE111"); n.set_pdn_apn(bad); n.m_emm_ctx.procedure_transaction_id=11;
  n.m_security_ready=true; n.m_sec_ctx.dl_nas_count=255;
  TESTASSERT(n.start_pdn_session()); TESTASSERT(g.creates==0 && n.m_pdn_rejected);
  TESTASSERT(n.m_sec_ctx.dl_nas_count==256 && s.downlink[5]==0);
  LIBLTE_BYTE_MSG_STRUCT plain{}; plain.N_bytes=s.downlink.size()-6;
  memcpy(plain.msg,s.downlink.data()+6,plain.N_bytes);
  LIBLTE_MME_ATTACH_REJECT_MSG_STRUCT reject{};
  TESTASSERT(liblte_mme_unpack_attach_reject_msg(&plain,&reject)==LIBLTE_SUCCESS);
  TESTASSERT(reject.emm_cause==19 && reject.esm_msg_present);
  LIBLTE_MME_PDN_CONNECTIVITY_REJECT_MSG_STRUCT esm{};
  TESTASSERT(liblte_mme_unpack_pdn_connectivity_reject_msg(&reject.esm_msg,&esm)==LIBLTE_SUCCESS);
  TESTASSERT(esm.esm_cause==27 && esm.proc_transaction_id==11);
  TESTASSERT(!n.start_pdn_session() && n.m_sec_ctx.dl_nas_count==256);
  for (const char* allowed : {"ADDXlte", static_cast<const char*>(nullptr)}) {
    n.reset(); n.set_pdn_apn(request(allowed)); n.m_security_ready=true;
    TESTASSERT(n.start_pdn_session() && n.m_apn_validated && n.m_selected_apn=="addxlte");
  }
  TESTASSERT(g.creates==2);
  LIBLTE_MME_SECURITY_MODE_COMPLETE_MSG_STRUCT complete{};
  LIBLTE_BYTE_MSG_STRUCT complete_wire{};
  TESTASSERT(liblte_mme_pack_security_mode_complete_msg(&complete,
      LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED_WITH_NEW_EPS_SECURITY_CONTEXT,0,&complete_wire)==LIBLTE_SUCCESS);
  auto smc=srsran::make_byte_buffer(); smc->N_bytes=complete_wire.N_bytes;
  memcpy(smc->msg,complete_wire.msg,complete_wire.N_bytes);
  n.reset(); n.set_pdn_apn(request("addxLTE"));
  TESTASSERT(n.handle_security_mode_complete(smc.get()) && g.creates==3);
  n.reset(); n.set_pdn_apn(request("wrong"));
  TESTASSERT(n.handle_security_mode_complete(smc.get()) && n.m_pdn_rejected && g.creates==3);
  n.reset(); n.set_pdn_apn(request(nullptr,true)); n.m_security_ready=true; n.m_waiting_esm=true;
  n.m_emm_ctx.procedure_transaction_id=11;
  auto response=srsran::make_byte_buffer();
  const uint8_t raw[]={0x27,0,0,0,0,0,2,11,0xda,0x28,12,11,'a','d','d','x','L','T','E','1','1','1','1'};
  memcpy(response->msg,raw,sizeof(raw)); response->N_bytes=sizeof(raw);
  TESTASSERT(n.handle_esm_information_response(response.get()));
  TESTASSERT(n.m_pdn_rejected && g.creates==3);
  TESTASSERT(!n.handle_esm_information_response(response.get()));
  for (const char* apn : {"addxLTE", "ADDXlte", static_cast<const char*>(nullptr)}) {
    n.reset(); n.set_pdn_apn(request(nullptr,true));
    n.m_security_ready=true; n.m_waiting_esm=true; n.m_ecm_ctx.eit=true;
    const unsigned before=g.creates;
    LIBLTE_MME_ESM_INFORMATION_RESPONSE_MSG_STRUCT information{};
    information.proc_transaction_id=12; // wrong PTI must not consume the pending transaction
    information.apn_present=apn!=nullptr;
    if (apn) strcpy(information.apn.apn,apn);
    LIBLTE_BYTE_MSG_STRUCT information_wire{};
    TESTASSERT(liblte_mme_pack_esm_information_response_msg(&information,
        LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED,0,&information_wire)==LIBLTE_SUCCESS);
    memcpy(response->msg,information_wire.msg,information_wire.N_bytes); response->N_bytes=information_wire.N_bytes;
    TESTASSERT(!n.handle_esm_information_response(response.get()));
    TESTASSERT(n.m_waiting_esm && !n.m_session_requested && g.creates==before);
    information.proc_transaction_id=11;
    TESTASSERT(liblte_mme_pack_esm_information_response_msg(&information,
        LIBLTE_MME_SECURITY_HDR_TYPE_INTEGRITY_AND_CIPHERED,0,&information_wire)==LIBLTE_SUCCESS);
    memcpy(response->msg,information_wire.msg,information_wire.N_bytes); response->N_bytes=information_wire.N_bytes;
    TESTASSERT(n.handle_esm_information_response(response.get()));
    TESTASSERT(g.creates==before+1 && n.m_apn_validated && n.m_selected_apn=="addxlte");
    TESTASSERT(!n.m_waiting_esm && n.m_session_requested);
    TESTASSERT(n.m_apn_source==(apn ? "esm_information_response" : "omitted"));
    TESTASSERT(!n.handle_esm_information_response(response.get()) && g.creates==before+1);
  }
  return 0;
}
int attach_paths() {
  s1ap_spy s; gtpc_spy g; hss_spy h;
  nas_if_t it{}; it.s1ap=&s; it.gtpc=&g; it.hss=&h;
  LIBLTE_MME_ATTACH_REQUEST_MSG_STRUCT a{};
  a.eps_mobile_id.type_of_id=LIBLTE_MME_EPS_MOBILE_ID_TYPE_IMSI;
  a.eps_mobile_id.imsi[0]=1; a.eps_mobile_id.imsi[14]=1;
  a.eps_attach_type=1; auto pdn=request("addxLTE111");
  sctp_sndrcvinfo sri{}; sri.sinfo_assoc_id=7;
  TESTASSERT(nas::handle_imsi_attach_request_unknown_ue(1,&sri,a,pdn,args(),it));
  TESTASSERT(s.last && !s.last->apn_allowed() && !s.last->m_ecm_ctx.eit);
  auto rx=srsran::make_byte_buffer(); rx->N_bytes=8; memset(rx->msg,0,32);
  rx->msg[0]=0x17; rx->msg[6]=7; rx->msg[7]=0x41;
  TESTASSERT(nas::handle_imsi_attach_request_known_ue(s.last,2,&sri,a,pdn,rx.get(),args(),it));
  TESTASSERT(s.last && !s.last->apn_allowed());
  TESTASSERT(nas::handle_guti_attach_request_unknown_ue(3,&sri,a,pdn,args(),it));
  TESTASSERT(s.last && !s.last->apn_allowed());
  nas* guti=s.last; guti->m_emm_ctx.imsi=100000000000002;
  TESTASSERT(nas::handle_guti_attach_request_known_ue(guti,4,&sri,a,pdn,rx.get(),args(),it));
  TESTASSERT(!guti->apn_allowed()); // invalid-integrity fallback retains the new request.
  TESTASSERT(g.creates==0);
  guti->m_sec_ctx.ul_nas_count=0; guti->m_emm_ctx.state=EMM_STATE_DEREGISTERED;
  uint8_t mac[4]{};
  srsran::security_128_eia2(&guti->m_sec_ctx.k_nas_int[16],0,0,
      srsran::SECURITY_DIRECTION_UPLINK,rx->msg+5,rx->N_bytes-5,mac);
  memcpy(rx->msg+1,mac,4);
  TESTASSERT(nas::handle_guti_attach_request_known_ue(guti,5,&sri,a,pdn,rx.get(),args(),it));
  TESTASSERT(guti->m_pdn_rejected && guti->m_sec_ctx.ul_nas_count==1 && g.creates==0);
  TESTASSERT(liblte_mme_pack_pdn_connectivity_request_msg(&pdn,&a.esm_msg)==LIBLTE_SUCCESS);
  LIBLTE_BYTE_MSG_STRUCT attach_wire{};
  TESTASSERT(liblte_mme_pack_attach_request_msg(&a,&attach_wire)==LIBLTE_SUCCESS);
  memcpy(rx->msg,attach_wire.msg,attach_wire.N_bytes); rx->N_bytes=attach_wire.N_bytes;
  auto instance=new nas(args(),it);
  TESTASSERT(instance->handle_attach_request(rx.get()));
  TESTASSERT(!instance->apn_allowed() && s.live.count(instance));
  TESTASSERT(nas::handle_attach_request(6,&sri,rx.get(),args(),it));
  TESTASSERT(s.last && !s.last->apn_allowed() && g.creates==0);

  return 0;
}
}
int main(int argc,char** argv) {
  srsran::test_init(argc,argv);
  TESTASSERT(parser_tests()==0); TESTASSERT(decision_tests()==0); TESTASSERT(attach_paths()==0);
  return 0;
}
