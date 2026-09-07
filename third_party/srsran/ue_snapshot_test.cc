#include "srsepc/hdr/mme/ue_snapshot.h"
#include "srsepc/hdr/mme/s1ap.h"
#include "srsran/common/test_common.h"
#include <fstream>
#include <iterator>
#include <thread>
#include <atomic>

using namespace srsepc;
int main(int argc,char** argv)
{
  srsran::test_init(argc,argv);
  nas_init_t args{}; args.apn="test"; nas_if_t interfaces{};
  nas one(args,interfaces), two(args,interfaces);
  one.m_emm_ctx.imsi=100000000000001; two.m_emm_ctx.imsi=100000000000002;
  one.m_emm_ctx.state=EMM_STATE_REGISTERED; one.m_ecm_ctx.state=ECM_STATE_IDLE;
  two.m_emm_ctx.state=EMM_STATE_REGISTERED; two.m_ecm_ctx.state=ECM_STATE_CONNECTED;
  one.m_esm_ctx[5].state=ERAB_DEACTIVATED; two.m_esm_ctx[5].state=ERAB_ACTIVE; two.m_esm_ctx[5].qci=9;
  const std::string run="unit-run";
  // Real map lifecycle without init(): no sockets, HSS, RF or worker thread.
  auto owner=s1ap::get_instance();
  auto mapped=new nas(args,interfaces); mapped->m_emm_ctx.imsi=100000000000003; mapped->m_ecm_ctx.mme_ue_s1ap_id=10;
  TESTASSERT(owner->add_nas_ctx_to_imsi_map(mapped));
  TESTASSERT(owner->add_nas_ctx_to_mme_ue_s1ap_id_map(mapped));
  mapped->m_ecm_ctx.mme_ue_s1ap_id=11;
  TESTASSERT(owner->add_nas_ctx_to_mme_ue_s1ap_id_map(mapped));
  TESTASSERT(owner->find_nas_ctx_from_mme_ue_s1ap_id(10)==nullptr);
  mapped->m_ecm_ctx.mme_ue_s1ap_id=0; // emulate an already-cleared SCTP context
  TESTASSERT(owner->delete_ue_ctx(mapped->m_emm_ctx.imsi));
  TESTASSERT(owner->snapshot_sessions(run)=="[]");
  s1ap::cleanup();

  std::set<const nas*> contexts{&one,&two};
  std::string rows=ue_snapshot::sessions(contexts,run);
  TESTASSERT(rows.find("100000000000001")!=std::string::npos && rows.find("100000000000002")!=std::string::npos);
  TESTASSERT(rows.find("\"ecm_state\":\"idle\"")!=std::string::npos);
  one.m_emm_ctx.ue_ip.s_addr=inet_addr("172.16.0.2");
  one.m_apn_validated=true; one.m_selected_apn="test";
  one.clear_pdn_session(); // same operation used by Delete Session / Detach
  two.m_emm_ctx.ue_ip.s_addr=inet_addr("172.16.0.2");
  rows=ue_snapshot::sessions(contexts,run);
  auto address_at=rows.find("172.16.0.2");
  TESTASSERT(address_at!=std::string::npos && rows.find("172.16.0.2",address_at+1)==std::string::npos);
  TESTASSERT(!one.m_apn_validated && one.m_selected_apn.empty() && one.m_esm_ctx[5].state==ERAB_DEACTIVATED);
  const uint64_t generation=one.m_session_generation;
  one.reset(); TESTASSERT(generation!=one.m_session_generation);
  contexts.erase(&two); rows=ue_snapshot::sessions(contexts,run);
  TESTASSERT(rows.find("100000000000002")==std::string::npos);
  TESTASSERT(rows.find("k_asme")==std::string::npos && rows.find("nas_count")==std::string::npos);
  one.m_emm_ctx.imsi=1010000000001ULL;
  rows=ue_snapshot::sessions(contexts,run);
  TESTASSERT(rows.find("001010000000001")!=std::string::npos);
  one.m_emm_ctx.imsi=1000000000000000ULL;
  TESTASSERT(ue_snapshot::sessions(contexts,run).empty());
  one.m_emm_ctx.imsi=0; rows=ue_snapshot::sessions(contexts,run);

  TESTASSERT(ue_snapshot::quote("a\"\n\\")=="\"a\\\"\\u000a\\\\\"");
  char directory[]="/tmp/lte-snapshot-test-XXXXXX"; TESTASSERT(mkdtemp(directory));
  const std::string path=std::string(directory)+"/sessions.json";
  ue_snapshot::writer writer(path,run);
  TESTASSERT(writer.publish(rows,true,true));
  struct stat st{}; TESTASSERT(stat(path.c_str(),&st)==0 && (st.st_mode&0777)==0600);
  auto read=[&]() { std::ifstream f(path); return std::string(std::istreambuf_iterator<char>(f),{}); };
  std::string current=read();
  TESTASSERT(current.find("\"schema_version\":1")!=std::string::npos && current.find("\"sequence\":1")!=std::string::npos);
  TESTASSERT(writer.publish(rows,true)); TESTASSERT(read()==current);
  std::this_thread::sleep_for(std::chrono::milliseconds(1100));
  TESTASSERT(writer.publish(rows,true)); TESTASSERT(read().find("\"sequence\":2")!=std::string::npos);
  std::atomic<bool> reading{true}, complete{true};
  std::thread reader([&]() {
    while(reading.load()) {
      auto bytes=read();
      if (bytes.empty() || bytes.front()!='{' || bytes.back()!='\n' || bytes[bytes.size()-2]!='}') complete=false;
    }
  });
  for (unsigned i=0;i<40;++i) TESTASSERT(writer.publish(i%2 ? rows : "[]",true,true));
  reading=false; reader.join(); TESTASSERT(complete.load());
  TESTASSERT(writer.publish("[]",false,true));
  TESTASSERT(read().find("\"state\":\"stopped\",\"sessions\":[]")!=std::string::npos);
  ue_snapshot::writer invalid(path,"invalid\nrun"); TESTASSERT(!invalid.publish("[]",true,true));
  TESTASSERT(!writer.publish("",true,true));
  unlink(path.c_str()); rmdir(directory);
  return 0;
}
