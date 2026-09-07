#include "srsepc/hdr/spgw/gtpc.h"
#include "srsepc/hdr/spgw/gtpu.h"
#include "srsran/common/test_common.h"
#include "srsran/upper/gtpu.h"
#include <arpa/inet.h>
#include <cerrno>
#include <cstring>
#include <fcntl.h>
#include <poll.h>
#include <set>
#include <sys/socket.h>
#include <unistd.h>
#include <vector>

namespace srsepc {
struct spgw_test_access { using gtpc = spgw::gtpc; using gtpu = spgw::gtpu; };
}
using namespace srsepc;

namespace {
class control_spy final : public spgw_test_access::gtpc
{
public:
  srsran::gtpc_pdu response{};
  unsigned sends = 0;
  bool send_ok = true;
  bool send_s11_pdu(const srsran::gtpc_pdu& pdu) override
  {
    response = pdu;
    ++sends;
    return send_ok;
  }
};

srsran::gtpc_create_session_request request(uint64_t imsi,
                                             uint64_t generation,
                                             srsran::access_policy_t policy,
                                             uint32_t mme_teid)
{
  srsran::gtpc_create_session_request value{};
  value.imsi_present = true;
  value.imsi = imsi;
  value.sender_f_teid.teid = mme_teid;
  value.access.version = srsran::S11_ACCESS_METADATA_VERSION;
  value.access.policy = policy;
  value.access.session_generation = generation;
  return value;
}

srsran::unique_byte_buffer_t ipv4(in_addr_t source, in_addr_t destination)
{
  auto packet = srsran::make_byte_buffer();
  TESTASSERT(packet != nullptr);
  packet->N_bytes = 20;
  std::memset(packet->msg, 0, packet->N_bytes);
  packet->msg[0] = 0x45;
  packet->msg[3] = 20;
  packet->msg[8] = 64;
  packet->msg[9] = 17;
  std::memcpy(packet->msg + 12, &source, 4);
  std::memcpy(packet->msg + 16, &destination, 4);
  uint32_t sum = 0;
  for (unsigned i = 0; i < 20; i += 2) sum += (uint32_t(packet->msg[i]) << 8) | packet->msg[i + 1];
  while (sum >> 16) sum = (sum & 0xffff) + (sum >> 16);
  sum = ~sum;
  packet->msg[10] = sum >> 8;
  packet->msg[11] = sum & 0xff;
  return packet;
}

srsran::unique_byte_buffer_t uplink(in_addr_t source, uint32_t teid,
                                   in_addr_t destination = inet_addr("198.51.100.1"))
{
  auto inner = ipv4(source, destination);
  auto pdu = srsran::make_byte_buffer();
  TESTASSERT(pdu != nullptr);
  pdu->N_bytes = 28;
  std::memset(pdu->msg, 0, pdu->N_bytes);
  pdu->msg[0] = 0x30;
  pdu->msg[1] = 0xff;
  pdu->msg[3] = 20;
  const uint32_t network_teid = htonl(teid);
  std::memcpy(pdu->msg + 4, &network_teid, 4);
  std::memcpy(pdu->msg + 8, inner->msg, inner->N_bytes);
  return pdu;
}

bool readable(int fd, int timeout_ms)
{
  pollfd item{};
  item.fd = fd;
  item.events = POLLIN;
  return poll(&item, 1, timeout_ms) == 1 && (item.revents & POLLIN) != 0;
}

int pool_and_queue_test()
{
  spgw_test_access::gtpu user;
  user.m_s1u_addr = {};
  control_spy control;
  control.m_gtpu = &user;
  user.m_gtpc = &control;
  spgw_args_t args{};
  args.sgi_if_addr = "172.16.0.1";
  args.max_paging_queue = 3;
  args.apn_mismatch_policy = apn_policy::mismatch_policy_t::restricted;
  control.m_apn_mismatch_policy = args.apn_mismatch_policy;
  control.m_max_paging_queue = args.max_paging_queue;
  TESTASSERT(control.init_ue_ip(&args, {}) == SRSRAN_SUCCESS);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == 237);
  TESTASSERT(control.m_restricted_ip_addr_pool.size() == 16);

  for (unsigned cycle = 0; cycle < 600; ++cycle) {
    auto value = request(200000000000001ULL + cycle, cycle + 1, srsran::ACCESS_POLICY_NORMAL, cycle + 1);
    auto* context = control.create_gtpc_ctx(value);
    TESTASSERT(context != nullptr && context->access_policy == srsran::ACCESS_POLICY_NORMAL);
    TESTASSERT(control.m_restricted_ip_addr_pool.count(context->ue_ipv4) == 0);
    TESTASSERT(control.delete_gtpc_ctx(context->up_ctrl_fteid.teid));
    TESTASSERT(control.m_ue_ip_addr_pool.size() == 237 && control.m_dynamic_ip_in_use.empty());
  }

  std::set<in_addr_t> restricted_ips;
  std::vector<uint32_t> restricted_teids;
  for (unsigned i = 0; i < 16; ++i) {
    auto value = request(300000000000001ULL + i, 100 + i, srsran::ACCESS_POLICY_RESTRICTED, 100 + i);
    auto* context = control.create_gtpc_ctx(value);
    TESTASSERT(context != nullptr && restricted_ips.insert(context->ue_ipv4).second);
    TESTASSERT(control.m_ue_ip_addr_pool.count(context->ue_ipv4) == 0);
    restricted_teids.push_back(context->up_ctrl_fteid.teid);
  }
  TESTASSERT(control.m_restricted_ip_addr_pool.empty() && control.m_ue_ip_addr_pool.size() == 237);
  auto exhausted = request(300000000000099ULL, 999, srsran::ACCESS_POLICY_RESTRICTED, 999);
  control.handle_create_session_request(exhausted);
  TESTASSERT(control.response.choice.create_session_response.cause.cause_value ==
             srsran::GTPC_CAUSE_VALUE_ALL_DYNAMIC_ADDRESSES_ARE_OCCUPIED);
  TESTASSERT(!control.response.choice.create_session_response.paa_present);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == 237);
  auto normal = request(400000000000001ULL, 2000, srsran::ACCESS_POLICY_NORMAL, 2000);
  auto* normal_context = control.create_gtpc_ctx(normal);
  TESTASSERT(normal_context != nullptr && restricted_ips.count(normal_context->ue_ipv4) == 0);
  TESTASSERT(control.delete_gtpc_ctx(normal_context->up_ctrl_fteid.teid));
  for (uint32_t teid : restricted_teids) TESTASSERT(control.delete_gtpc_ctx(teid));
  TESTASSERT(control.m_restricted_ip_addr_pool.size() == 16 && control.m_dynamic_ip_in_use.empty());

  const size_t normal_capacity = control.m_ue_ip_addr_pool.size();
  const size_t restricted_capacity = control.m_restricted_ip_addr_pool.size();
  auto invalid = request(500000000000001ULL, 1, srsran::ACCESS_POLICY_NORMAL, 1);
  invalid.access.version = 0;
  control.handle_create_session_request(invalid);
  TESTASSERT(control.response.choice.create_session_response.cause.cause_value ==
             srsran::GTPC_CAUSE_VALUE_MANDATORY_IE_INCORRECT);
  TESTASSERT(control.m_teid_to_tunnel_ctx.empty() && user.m_ip_to_access_owner.empty());
  invalid = request(500000000000002ULL, 0, srsran::ACCESS_POLICY_NORMAL, 2);
  control.handle_create_session_request(invalid);
  TESTASSERT(control.response.choice.create_session_response.cause.cause_value ==
             srsran::GTPC_CAUSE_VALUE_MANDATORY_IE_INCORRECT);
  invalid = request(500000000000003ULL, 3, srsran::ACCESS_POLICY_DENY, 3);
  control.handle_create_session_request(invalid);
  TESTASSERT(control.response.choice.create_session_response.cause.cause_value ==
             srsran::GTPC_CAUSE_VALUE_MANDATORY_IE_INCORRECT);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == normal_capacity);
  TESTASSERT(control.m_restricted_ip_addr_pool.size() == restricted_capacity);

  auto short_pdu = srsran::make_byte_buffer();
  short_pdu->N_bytes = sizeof(srsran::gtpc_pdu) - 1;
  const unsigned before_short = control.sends;
  control.handle_s11_pdu(short_pdu.get());
  TESTASSERT(control.sends == before_short);

  auto rollback = request(500000000000004ULL, 4, srsran::ACCESS_POLICY_RESTRICTED, 4);
  control.send_ok = false;
  control.handle_create_session_request(rollback);
  TESTASSERT(control.m_teid_to_tunnel_ctx.empty() && control.m_imsi_to_ctr_teid.empty());
  TESTASSERT(control.m_dynamic_ip_in_use.empty() && user.m_ip_to_access_owner.empty());
  TESTASSERT(control.m_restricted_ip_addr_pool.size() == restricted_capacity);
  control.send_ok = true;

  auto queued = request(600000000000001ULL, 50, srsran::ACCESS_POLICY_NORMAL, 50);
  auto* queued_context = control.create_gtpc_ctx(queued);
  TESTASSERT(queued_context != nullptr);
  const unsigned before_queue = control.sends;
  for (unsigned i = 0; i < 4; ++i) {
    user.handle_sgi_pdu(ipv4(inet_addr("198.51.100.1"), queued_context->ue_ipv4));
  }
  TESTASSERT(control.sends == before_queue + 1);
  TESTASSERT(queued_context->paging_pending && queued_context->paging_queue.size() == 3);
  TESTASSERT(!control.queue_downlink_packet(queued_context->up_ctrl_fteid.teid, 49,
                                            ipv4(inet_addr("198.51.100.1"), queued_context->ue_ipv4)));

  auto restricted = request(600000000000002ULL, 51, srsran::ACCESS_POLICY_RESTRICTED, 51);
  auto* restricted_context = control.create_gtpc_ctx(restricted);
  TESTASSERT(restricted_context != nullptr);
  const unsigned before_restricted = control.sends;
  for (unsigned i = 0; i < 4; ++i) {
    user.handle_sgi_pdu(ipv4(inet_addr("198.51.100.1"), restricted_context->ue_ipv4));
  }
  TESTASSERT(control.sends == before_restricted);
  TESTASSERT(!restricted_context->paging_pending && restricted_context->paging_queue.empty());
  TESTASSERT(control.delete_gtpc_ctx(queued_context->up_ctrl_fteid.teid));
  TESTASSERT(control.delete_gtpc_ctx(restricted_context->up_ctrl_fteid.teid));
  return 0;
}

int pipe_socket_test()
{
  spgw_test_access::gtpu user;
  const in_addr_t normal_ip = inet_addr("172.16.0.2");
  const in_addr_t restricted_ip = inet_addr("172.16.0.250");
  int pipefd[2];
  TESTASSERT(pipe(pipefd) == 0 && fcntl(pipefd[0], F_SETFL, O_NONBLOCK) == 0);
  user.m_sgi = pipefd[1];
  const int receiver = socket(AF_INET, SOCK_DGRAM, 0);
  const int sender = socket(AF_INET, SOCK_DGRAM, 0);
  TESTASSERT(receiver >= 0 && sender >= 0);
  // Do not co-bind a running EPC's port: a busy loopback endpoint fails the
  // test rather than sharing its traffic. No radio or long-lived service runs.
  sockaddr_in address{};
  address.sin_family = AF_INET;
  address.sin_port = htons(GTPU_RX_PORT);
  address.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
  TESTASSERT(bind(receiver, reinterpret_cast<sockaddr*>(&address), sizeof(address)) == 0);
  TESTASSERT(fcntl(receiver, F_SETFL, O_NONBLOCK) == 0);
  user.m_s1u = sender;
  srsran::gtp_fteid_t normal_down{};
  normal_down.teid = 3001;
  normal_down.ipv4 = htonl(INADDR_LOOPBACK);
  srsran::gtp_fteid_t restricted_down = normal_down;
  restricted_down.teid = 3002;
  TESTASSERT(user.install_gtpc_tunnel(normal_ip, 101, 201, 1001, srsran::ACCESS_POLICY_NORMAL));
  TESTASSERT(user.install_gtpc_tunnel(restricted_ip, 102, 202, 1002, srsran::ACCESS_POLICY_RESTRICTED));
  TESTASSERT(user.modify_gtpu_tunnel(normal_ip, normal_down, 101, 201, 1001, srsran::ACCESS_POLICY_NORMAL));
  TESTASSERT(user.modify_gtpu_tunnel(restricted_ip, restricted_down, 102, 202, 1002,
                                     srsran::ACCESS_POLICY_RESTRICTED));

  uint8_t bytes[128]{};
  auto pdu = uplink(normal_ip, 201);
  user.handle_s1u_pdu(pdu.get());
  TESTASSERT(read(pipefd[0], bytes, sizeof(bytes)) == 20);
  // Public, upstream LAN, local SGi service, peer UE and DNS destinations all
  // encounter the same pre-TUN deny gate, without sending to those addresses.
  for (const char* target : {"198.51.100.1", "10.0.0.1", "172.16.0.1", "172.16.0.2", "192.0.2.53"}) {
    pdu = uplink(restricted_ip, 202, inet_addr(target));
    user.handle_s1u_pdu(pdu.get());
    TESTASSERT(read(pipefd[0], bytes, sizeof(bytes)) == -1 && (errno == EAGAIN || errno == EWOULDBLOCK));
  }

  user.handle_sgi_pdu(ipv4(inet_addr("198.51.100.1"), normal_ip));
  TESTASSERT(readable(receiver, 500));
  TESTASSERT(recv(receiver, bytes, sizeof(bytes), 0) == 28);
  user.handle_sgi_pdu(ipv4(inet_addr("198.51.100.1"), restricted_ip));
  TESTASSERT(!readable(receiver, 50));
  auto stale = ipv4(inet_addr("198.51.100.1"), normal_ip);
  TESTASSERT(!user.send_s1u_pdu(normal_ip, 1000, stale.get()) && !readable(receiver, 50));
  std::queue<srsran::unique_byte_buffer_t> queue;
  for (unsigned i=0; i<2; ++i) queue.push(ipv4(inet_addr("198.51.100.1"),normal_ip));
  user.send_all_queued_packets(normal_ip,1001,queue);
  TESTASSERT(queue.empty());
  for (unsigned i=0; i<2; ++i) {
    TESTASSERT(readable(receiver,500) && recv(receiver,bytes,sizeof(bytes),0)==28);
  }
  queue.push(ipv4(inet_addr("198.51.100.1"),normal_ip));
  user.send_all_queued_packets(normal_ip,1000,queue);
  TESTASSERT(queue.empty() && !readable(receiver,50));
  queue.push(ipv4(inet_addr("198.51.100.1"),restricted_ip));
  user.send_all_queued_packets(restricted_ip,1002,queue);
  TESTASSERT(queue.empty() && !readable(receiver,50));

  TESTASSERT(user.delete_gtpu_tunnel(restricted_ip));
  TESTASSERT(user.modify_gtpu_tunnel(restricted_ip, restricted_down, 102, 202, 1002,
                                     srsran::ACCESS_POLICY_RESTRICTED));
  pdu = uplink(restricted_ip, 202);
  user.handle_s1u_pdu(pdu.get());
  TESTASSERT(read(pipefd[0], bytes, sizeof(bytes)) == -1 && (errno == EAGAIN || errno == EWOULDBLOCK));
  TESTASSERT(user.delete_gtpc_tunnel(normal_ip));
  TESTASSERT(user.delete_gtpc_tunnel(restricted_ip));
  close(receiver);
  close(sender);
  close(pipefd[0]);
  close(pipefd[1]);
  return 0;
}

// MME cancellation/retry coverage lives in restricted_mme_lifecycle_test.cc.
} // namespace

int main(int argc, char** argv)
{
  srsran::test_init(argc, argv);
  TESTASSERT(pool_and_queue_test() == 0);
  TESTASSERT(pipe_socket_test() == 0);
  return 0;
}
