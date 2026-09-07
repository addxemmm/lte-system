#include "srsran/common/test_common.h"
#include <arpa/inet.h>
#include <chrono>
#include <cstring>
#include <fcntl.h>
#include <map>
#include <memory>
#include <poll.h>
#include <set>
#include <string>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <vector>

#define private public
#include "srsepc/hdr/mme/mme_gtpc.h"
#include "srsepc/hdr/mme/s1ap.h"
#include "srsepc/hdr/mme/s1ap_ctx_mngmt_proc.h"
#undef private

using namespace srsepc;
namespace {
class s1ap_spy final : public s1ap_interface_nas
{
public:
  unsigned downlinks = 0;
  uint32_t allocate_m_tmsi(uint64_t) override { return 1; }
  uint32_t get_next_mme_ue_s1ap_id() override { return 1; }
  bool add_nas_ctx_to_imsi_map(nas*) override { return true; }
  bool add_nas_ctx_to_mme_ue_s1ap_id_map(nas*) override { return true; }
  bool add_ue_to_enb_set(int32_t, uint32_t) override { return true; }
  bool release_ue_ecm_ctx(uint32_t) override { return true; }
  bool delete_ue_ctx(uint64_t) override { return true; }
  uint64_t find_imsi_from_m_tmsi(uint32_t) override { return 0; }
  nas* find_nas_ctx_from_imsi(uint64_t) override { return nullptr; }
  bool send_initial_context_setup_request(uint64_t, uint16_t) override { return true; }
  bool send_ue_context_release_command(uint32_t) override { return true; }
  bool send_erab_release_command(uint32_t, uint32_t, std::vector<uint16_t>, sctp_sndrcvinfo) override { return true; }
  bool send_erab_modify_request(uint32_t, uint32_t, std::map<uint16_t, uint16_t>,
                                srsran::byte_buffer_t*, sctp_sndrcvinfo) override { return true; }
  bool send_downlink_nas_transport(uint32_t, uint32_t, srsran::byte_buffer_t*, sctp_sndrcvinfo) override
  {
    ++downlinks;
    return true;
  }
};

class gtpc_spy final : public gtpc_interface_nas
{
public:
  unsigned creates = 0;
  unsigned deletes = 0;
  srsran::access_policy_t last_policy = srsran::ACCESS_POLICY_DENY;
  bool send_create_session_request(uint64_t, uint64_t, srsran::access_policy_t policy) override
  {
    ++creates;
    last_policy = policy;
    return true;
  }
  bool send_modify_bearer_request(uint64_t, uint16_t, srsran::gtp_fteid_t*) override { return true; }
  bool send_delete_session_request(uint64_t) override { ++deletes; return true; }
  bool send_downlink_data_notification_failure_indication(uint64_t, srsran::gtpc_cause_value) override { return true; }
};

bool readable(int fd, int timeout_ms)
{
  pollfd item{};
  item.fd = fd;
  item.events = POLLIN;
  return poll(&item, 1, timeout_ms) == 1 && (item.revents & POLLIN) != 0;
}

class wire_fixture
{
public:
  wire_fixture()
  {
    mme = mme_gtpc::get_instance();
    owner = s1ap::get_instance();
    clear_transactions();
    mme->m_next_ctrl_teid = 1000;
    mme->m_s1ap = owner;
    owner->m_s1ap_args.mme_apn = "addxlte";
    owner->m_pcap_enable = false;
    owner->m_s1mme = -1;
    owner->m_s1ap_ctx_mngmt_proc = s1ap_ctx_mngmt_proc::get_instance();
    owner->m_s1ap_ctx_mngmt_proc->init();
    receiver = socket(AF_UNIX, SOCK_DGRAM, 0);
    TESTASSERT(receiver >= 0);
    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    const std::string name = "lte-mme-lifecycle-" + std::to_string(getpid());
    TESTASSERT(name.size() + 1 < sizeof(address.sun_path));
    address.sun_path[0] = '\0';
    std::memcpy(address.sun_path + 1, name.data(), name.size());
    TESTASSERT(bind(receiver, reinterpret_cast<const sockaddr*>(&address), sizeof(address)) == 0);
    TESTASSERT(fcntl(receiver, F_SETFL, O_NONBLOCK) == 0);
    mme->m_spgw_addr = address;
    restore_sender();
  }

  ~wire_fixture()
  {
    clear_transactions();
    owner->m_imsi_to_nas_ctx.clear();
    owner->m_mme_ue_s1ap_id_to_nas_ctx.clear();
    if (mme->m_s11 >= 0) close(mme->m_s11);
    close(receiver);
    mme->m_s11 = -1;
  }

  void clear_transactions()
  {
    mme->m_mme_ctr_teid_to_imsi.clear();
    mme->m_imsi_to_gtpc_ctx.clear();
    mme->m_cancelled_creates.clear();
    mme->m_next_delete_retry = {};
  }

  void add(nas* context)
  {
    TESTASSERT(owner->m_imsi_to_nas_ctx.emplace(context->m_emm_ctx.imsi, context).second);
  }
  void remove(nas* context) { owner->m_imsi_to_nas_ctx.erase(context->m_emm_ctx.imsi); }

  srsran::gtpc_pdu receive()
  {
    TESTASSERT(readable(receiver, 500));
    srsran::gtpc_pdu pdu{};
    TESTASSERT(recv(receiver, &pdu, sizeof(pdu), 0) == static_cast<ssize_t>(sizeof(pdu)));
    return pdu;
  }
  bool quiet() const { return !readable(receiver, 50); }
  void break_sender() { if (mme->m_s11 >= 0) close(mme->m_s11); mme->m_s11 = -1; }
  void restore_sender() { mme->m_s11 = socket(AF_UNIX, SOCK_DGRAM, 0); TESTASSERT(mme->m_s11 >= 0); }

  mme_gtpc* mme = nullptr;
  s1ap* owner = nullptr;
  int receiver = -1;
};

nas_init_t nas_args()
{
  nas_init_t args{};
  args.apn = "addxlte";
  args.dns = "192.0.2.53";
  args.apn_mismatch_policy = apn_policy::mismatch_policy_t::restricted;
  args.cipher_algo = srsran::CIPHERING_ALGORITHM_ID_EEA0;
  args.integ_algo = srsran::INTEGRITY_ALGORITHM_ID_128_EIA2;
  return args;
}

LIBLTE_MME_PDN_CONNECTIVITY_REQUEST_MSG_STRUCT pdn(const char* apn)
{
  LIBLTE_MME_PDN_CONNECTIVITY_REQUEST_MSG_STRUCT value{};
  value.proc_transaction_id = 11;
  value.request_type = 1;
  value.pdn_type = LIBLTE_MME_PDN_TYPE_IPV4;
  value.apn_present = apn != nullptr;
  if (apn) std::strncpy(value.apn.apn, apn, sizeof(value.apn.apn) - 1);
  return value;
}

std::unique_ptr<nas> pending(uint64_t imsi, const char* apn, s1ap_spy& s1, gtpc_spy& gtpc)
{
  nas_if_t interfaces{};
  interfaces.s1ap = &s1;
  interfaces.gtpc = &gtpc;
  auto context = std::unique_ptr<nas>(new nas(nas_args(), interfaces));
  context->m_emm_ctx.imsi = imsi;
  context->set_pdn_apn(pdn(apn));
  context->m_security_ready = true;
  TESTASSERT(context->start_pdn_session());
  return context;
}

srsran::gtpc_pdu response(uint32_t mme_teid,
                          uint32_t remote_teid,
                          uint64_t generation,
                          srsran::access_policy_t policy,
                          srsran::gtpc_cause_value cause)
{
  srsran::gtpc_pdu pdu{};
  pdu.header.type = srsran::GTPC_MSG_TYPE_CREATE_SESSION_RESPONSE;
  pdu.header.teid_present = true;
  pdu.header.teid = mme_teid;
  auto& body = pdu.choice.create_session_response;
  body.access.version = srsran::S11_ACCESS_METADATA_VERSION;
  body.access.policy = policy;
  body.access.session_generation = generation;
  body.cause.cause_value = cause;
  if (cause == srsran::GTPC_CAUSE_VALUE_REQUEST_ACCEPTED) {
    body.sender_f_teid_present = true;
    body.sender_f_teid.teid = remote_teid;
    body.eps_bearer_context_created.s1_u_sgw_f_teid_present = true;
    body.eps_bearer_context_created.s1_u_sgw_f_teid.teid = remote_teid + 1;
    body.eps_bearer_context_created.s1_u_sgw_f_teid.ipv4 = htonl(INADDR_LOOPBACK);
    body.paa_present = true;
    body.paa.pdn_type = srsran::GTPC_PDN_TYPE_IPV4;
    body.paa.ipv4_present = true;
    body.paa.ipv4 = inet_addr(policy == srsran::ACCESS_POLICY_NORMAL ? "172.16.0.2" : "172.16.0.250");
  }
  return pdu;
}

uint32_t start_wire_create(wire_fixture& wire, uint64_t imsi, uint64_t generation, srsran::access_policy_t policy)
{
  TESTASSERT(wire.mme->send_create_session_request(imsi, generation, policy));
  auto request = wire.receive();
  TESTASSERT(request.header.type == srsran::GTPC_MSG_TYPE_CREATE_SESSION_REQUEST);
  TESTASSERT(request.choice.create_session_request.access.session_generation == generation);
  TESTASSERT(request.choice.create_session_request.access.policy == policy);
  return request.choice.create_session_request.sender_f_teid.teid;
}

int cancelled_create_retry_test()
{
  wire_fixture wire;
  s1ap_spy s1;
  gtpc_spy gtpc;
  const uint64_t imsi = 700000000000001ULL;
  auto context = pending(imsi, "addxlte", s1, gtpc);
  wire.add(context.get());
  const uint64_t old_generation = context->m_session_generation;
  const uint32_t old_teid = start_wire_create(wire, imsi, old_generation, srsran::ACCESS_POLICY_NORMAL);
  TESTASSERT(wire.mme->send_delete_session_request(imsi));
  TESTASSERT(wire.quiet() && wire.mme->m_cancelled_creates.count(old_teid) == 1);

  context->set_pdn_apn(pdn("wrong.apn"));
  context->m_security_ready = true;
  TESTASSERT(context->start_pdn_session());
  const uint64_t new_generation = context->m_session_generation;
  const uint32_t new_teid = start_wire_create(wire, imsi, new_generation, srsran::ACCESS_POLICY_RESTRICTED);
  auto wrong_generation = response(old_teid, 9001, old_generation + 1, srsran::ACCESS_POLICY_NORMAL,
                                   srsran::GTPC_CAUSE_VALUE_REQUEST_ACCEPTED);
  TESTASSERT(!wire.mme->handle_create_session_response(&wrong_generation) && wire.quiet());
  auto wrong_policy = response(old_teid, 9001, old_generation, srsran::ACCESS_POLICY_RESTRICTED,
                               srsran::GTPC_CAUSE_VALUE_REQUEST_ACCEPTED);
  TESTASSERT(!wire.mme->handle_create_session_response(&wrong_policy) && wire.quiet());
  TESTASSERT(wire.mme->m_imsi_to_gtpc_ctx.at(imsi).session_generation == new_generation);
  TESTASSERT(wire.mme->m_mme_ctr_teid_to_imsi.at(new_teid) == imsi);

  wire.break_sender();
  auto late = response(old_teid, 9001, old_generation, srsran::ACCESS_POLICY_NORMAL,
                       srsran::GTPC_CAUSE_VALUE_REQUEST_ACCEPTED);
  TESTASSERT(!wire.mme->handle_create_session_response(&late));
  TESTASSERT(wire.mme->m_cancelled_creates.at(old_teid).sgw_ctr_fteid.teid == 9001);
  TESTASSERT(wire.mme->m_imsi_to_gtpc_ctx.at(imsi).session_generation == new_generation);
  wire.restore_sender();
  wire.mme->m_next_delete_retry = {};
  wire.mme->retry_pending_deletes();
  auto deletion = wire.receive();
  TESTASSERT(deletion.header.type == srsran::GTPC_MSG_TYPE_DELETE_SESSION_REQUEST);
  TESTASSERT(deletion.header.teid == 9001 && wire.mme->m_cancelled_creates.count(old_teid) == 0);
  TESTASSERT(wire.mme->m_imsi_to_gtpc_ctx.at(imsi).access_policy == srsran::ACCESS_POLICY_RESTRICTED);
  wire.remove(context.get());
  return 0;
}

enum class failure_point { create, confirm, ics };

void verify_failure_once(wire_fixture& wire, failure_point point, uint64_t imsi)
{
  wire.clear_transactions();
  s1ap_spy s1;
  gtpc_spy gtpc;
  const bool restricted = point == failure_point::ics;
  auto context = pending(imsi, restricted ? "wrong.apn" : "addxlte", s1, gtpc);
  wire.add(context.get());
  const auto policy = restricted ? srsran::ACCESS_POLICY_RESTRICTED : srsran::ACCESS_POLICY_NORMAL;
  const uint64_t generation = context->m_session_generation;
  const uint32_t mme_teid = start_wire_create(wire, imsi, generation, policy);
  if (point == failure_point::confirm) context->m_pending_access_policy = srsran::ACCESS_POLICY_RESTRICTED;
  if (point == failure_point::ics) {
    context->m_ecm_ctx.mme_ue_s1ap_id = 77;
    context->m_ecm_ctx.enb_ue_s1ap_id = 88;
    context->m_esm_ctx[5].erab_id = 5;
    context->m_esm_ctx[5].qci = 9;
  }
  const auto cause = point == failure_point::create ? srsran::GTPC_CAUSE_VALUE_REQUEST_REJECTED :
                                                     srsran::GTPC_CAUSE_VALUE_REQUEST_ACCEPTED;
  auto result = response(mme_teid, 9200 + static_cast<unsigned>(point), generation, policy, cause);
  TESTASSERT(!wire.mme->handle_create_session_response(&result));
  if (point == failure_point::create) {
    TESTASSERT(wire.quiet());
  } else {
    auto deletion = wire.receive();
    TESTASSERT(deletion.header.type == srsran::GTPC_MSG_TYPE_DELETE_SESSION_REQUEST);
    TESTASSERT(deletion.header.teid == 9200 + static_cast<unsigned>(point));
  }
  TESTASSERT(s1.downlinks == 1 && gtpc.deletes == 0);
  TESTASSERT(context->m_pdn_rejected && !context->m_session_requested);
  TESTASSERT(context->m_access_policy == srsran::ACCESS_POLICY_DENY);
  TESTASSERT(!wire.mme->handle_create_session_response(&result));
  TESTASSERT(s1.downlinks == 1 && wire.quiet());
  wire.remove(context.get());
}

int failure_notification_test()
{
  wire_fixture wire;
  verify_failure_once(wire, failure_point::create, 710000000000001ULL);
  verify_failure_once(wire, failure_point::confirm, 710000000000002ULL);
  verify_failure_once(wire, failure_point::ics, 710000000000003ULL);
  return 0;
}
} // namespace

int main(int argc, char** argv)
{
  srsran::test_init(argc, argv);
  TESTASSERT(cancelled_create_retry_test() == 0);
  TESTASSERT(failure_notification_test() == 0);
  return 0;
}