#include "srsepc/hdr/spgw/gtpc.h"
#include "srsepc/hdr/spgw/gtpu.h"
#include "srsran/common/test_common.h"
#include <arpa/inet.h>
#include <cstring>
#include <fcntl.h>
#include <unistd.h>

namespace srsepc {
struct spgw_test_access { using gtpc = spgw::gtpc; using gtpu = spgw::gtpu; };
}
using namespace srsepc;
namespace {
class control_spy : public spgw_test_access::gtpc {
public:
  srsran::gtpc_pdu response{};
  bool send_ok = true;
  bool send_s11_pdu(const srsran::gtpc_pdu& pdu) override { response = pdu; return send_ok; }
};
int pool_tests()
{
  spgw_test_access::gtpu user;
  user.m_s1u_addr = {};
  control_spy control;
  control.m_gtpu = &user;
  spgw_args_t args{};
  args.sgi_if_addr = "172.16.0.1";
  TESTASSERT(control.init_ue_ip(&args, {}) == SRSRAN_SUCCESS);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == 253);
  srsran::gtpc_create_session_request request{};
  request.imsi = 100000000000001;
  request.sender_f_teid.teid = 123;
  for (unsigned i = 0; i < 600; ++i) {
    auto context = control.create_gtpc_ctx(request);
    TESTASSERT(context && context->ue_ipv4 != 0);
    const uint32_t teid = context->up_ctrl_fteid.teid;
    TESTASSERT(control.delete_gtpc_ctx(teid));
    TESTASSERT(!control.delete_gtpc_ctx(teid));
    TESTASSERT(control.m_ue_ip_addr_pool.size() == 253 && control.m_dynamic_ip_in_use.empty());
  }
  request.imsi = 300000000000001;
  auto sleeping = control.create_gtpc_ctx(request);
  TESTASSERT(sleeping);
  srsran::gtpc_header control_header{}; control_header.teid = sleeping->up_ctrl_fteid.teid;
  srsran::gtpc_release_access_bearers_request release{};
  srsran::gtp_fteid_t down{};
  user.modify_gtpu_tunnel(sleeping->ue_ipv4, down, sleeping->up_ctrl_fteid.teid, sleeping->up_user_fteid.teid);
  control.handle_release_access_bearers_request(control_header, release);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == 252 && control.m_dynamic_ip_in_use.count(sleeping->ue_ipv4));
  TESTASSERT(user.m_up_teid_to_ip.empty() && user.m_ip_to_ctr_teid.count(sleeping->ue_ipv4));
  srsran::gtpc_modify_bearer_request resume{};
  control.handle_modify_bearer_request(control_header, resume);
  TESTASSERT(user.m_up_teid_to_ip.count(sleeping->up_user_fteid.teid));
  control.delete_gtpc_ctx(control_header.teid);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == 253);
  for (unsigned i = 0; i < 253; ++i) {
    request.imsi = 100000000000001 + i;
    TESTASSERT(control.create_gtpc_ctx(request));
  }
  request.imsi = 200000000000001;
  TESTASSERT(control.create_gtpc_ctx(request) == nullptr);
  control.handle_create_session_request(request);
  TESTASSERT(control.response.header.teid == request.sender_f_teid.teid);
  TESTASSERT(control.response.choice.create_session_response.cause.cause_value ==
             srsran::GTPC_CAUSE_VALUE_ALL_DYNAMIC_ADDRESSES_ARE_OCCUPIED);
  TESTASSERT(!control.response.choice.create_session_response.paa_present);
  while (!control.m_teid_to_tunnel_ctx.empty()) control.delete_gtpc_ctx(control.m_teid_to_tunnel_ctx.begin()->first);
  control.send_ok = false;
  control.handle_create_session_request(request);
  TESTASSERT(control.m_teid_to_tunnel_ctx.empty() && control.m_ue_ip_addr_pool.size() == 253);
  control.send_ok = true;
  const std::map<std::string,uint64_t> reserved{{"172.16.0.20",100000000000001}};
  TESTASSERT(control.init_ue_ip(&args,reserved) == SRSRAN_SUCCESS);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == 252);
  request.imsi = 100000000000001;
  auto fixed = control.create_gtpc_ctx(request);
  TESTASSERT(fixed && fixed->ue_ipv4 == inet_addr("172.16.0.20"));
  control.delete_gtpc_ctx(fixed->up_ctrl_fteid.teid);
  TESTASSERT(control.m_ue_ip_addr_pool.size() == 252);
  for (const char* invalid : {"172.16.0.0", "172.16.0.1", "172.16.0.255", "172.16.1.20"})
    TESTASSERT(control.init_ue_ip(&args, {{invalid,1}}) != SRSRAN_SUCCESS);
  TESTASSERT(control.init_ue_ip(&args, {{"172.16.0.2",1},{"172.16.0.3",1}}) != SRSRAN_SUCCESS);
  args.sgi_if_addr = "172.16.0.2";
  TESTASSERT(control.init_ue_ip(&args,{}) != SRSRAN_SUCCESS);
  return 0;
}
int source_tests()
{
  spgw_test_access::gtpu user;
  srsran::gtp_fteid_t down{};
  const in_addr_t ip = inet_addr("172.16.0.2"), other = inet_addr("172.16.0.3");
  TESTASSERT(user.modify_gtpu_tunnel(ip,down,100,200));
  uint8_t ipv4[20]{}; ipv4[0]=0x45; ipv4[3]=20;
  auto checksum=[&]() {
    ipv4[10]=ipv4[11]=0; uint32_t sum=0;
    for (unsigned i=0;i<20;i+=2) sum+=(uint32_t(ipv4[i])<<8)|ipv4[i+1];
    while(sum>>16) sum=(sum&0xffff)+(sum>>16);
    sum=~sum; ipv4[10]=sum>>8; ipv4[11]=sum&255;
  };
  memcpy(ipv4+12,&ip,4); checksum();
  TESTASSERT(user.valid_uplink(200,ipv4,sizeof(ipv4)));
  TESTASSERT(!user.valid_uplink(100,ipv4,sizeof(ipv4)));
  memcpy(ipv4+12,&other,4); checksum();
  TESTASSERT(!user.valid_uplink(200,ipv4,sizeof(ipv4)));
  memcpy(ipv4+12,&ip,4); checksum();
  ipv4[0]=0x44; TESTASSERT(!user.valid_uplink(200,ipv4,sizeof(ipv4))); ipv4[0]=0x45;
  TESTASSERT(!user.valid_uplink(200,ipv4,19));
  int pipefd[2]; TESTASSERT(pipe(pipefd)==0);
  user.m_sgi=pipefd[1]; TESTASSERT(fcntl(pipefd[0],F_SETFL,O_NONBLOCK)==0);
  auto pdu=srsran::make_byte_buffer(); pdu->N_bytes=28;
  memset(pdu->msg,0,28); pdu->msg[0]=0x30; pdu->msg[1]=0xff; pdu->msg[3]=20;
  uint32_t teid=htonl(200); memcpy(pdu->msg+4,&teid,4); memcpy(pdu->msg+8,ipv4,20);
  user.handle_s1u_pdu(pdu.get()); uint8_t received[32]; TESTASSERT(read(pipefd[0],received,32)==20);
  pdu->msg[0]=0x34; user.handle_s1u_pdu(pdu.get()); TESTASSERT(read(pipefd[0],received,32)==-1);
  pdu->msg[0]=0x30; user.delete_gtpu_tunnel(ip);
  TESTASSERT(!user.valid_uplink(200,ipv4,sizeof(ipv4)));
  user.handle_s1u_pdu(pdu.get()); TESTASSERT(read(pipefd[0],received,32)==-1);
  TESTASSERT(user.modify_gtpu_tunnel(ip,down,101,201));
  TESTASSERT(!user.valid_uplink(200,ipv4,sizeof(ipv4)) && user.valid_uplink(201,ipv4,sizeof(ipv4)));
  user.delete_gtpc_tunnel(ip); TESTASSERT(user.m_up_teid_to_ip.empty() && user.m_ip_to_usr_teid.empty());
  close(pipefd[0]); close(pipefd[1]);
  return 0;
}
}
int main(int argc,char** argv)
{
  srsran::test_init(argc,argv);
  TESTASSERT(pool_tests()==0); TESTASSERT(source_tests()==0);
  return 0;
}
