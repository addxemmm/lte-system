#include "srsepc/hdr/mme/ue_snapshot.h"
#include "srsepc/hdr/mme/s1ap.h"
#include "srsran/common/test_common.h"
#include <atomic>
#include <cstdlib>
#include <fstream>
#include <iterator>
#include <thread>

using namespace srsepc;

namespace {
void set_normal(nas& n, const char* source, access_reason_t reason, const char* ip)
{
  n.m_emm_ctx.ue_ip.s_addr = inet_addr(ip);
  n.m_requested_apn = reason == ACCESS_REASON_APN_OMITTED ? "" : "test";
  n.m_apn_source = source;
  n.m_selected_apn = "test";
  n.m_apn_validated = true;
  n.m_session_requested = true;
  n.m_access_policy = srsran::ACCESS_POLICY_NORMAL;
  n.m_access_reason = reason;
}

void set_restricted(nas& n, const char* ip)
{
  n.m_emm_ctx.ue_ip.s_addr = inet_addr(ip);
  n.m_requested_apn = "wrong.apn";
  n.m_apn_source = "pdn_request";
  n.m_selected_apn = "test";
  n.m_apn_validated = false;
  n.m_session_requested = true;
  n.m_access_policy = srsran::ACCESS_POLICY_RESTRICTED;
  n.m_access_reason = ACCESS_REASON_APN_MISMATCH;
}

std::string read_file(const std::string& path)
{
  std::ifstream file(path, std::ios::binary);
  return std::string(std::istreambuf_iterator<char>(file), {});
}
} // namespace

int main(int argc, char** argv)
{
  srsran::test_init(argc, argv);
  nas_init_t args{};
  args.apn = "test";
  nas_if_t interfaces{};
  nas normal(args, interfaces), restricted(args, interfaces), denied(args, interfaces);
  normal.m_emm_ctx.imsi = 100000000000001;
  restricted.m_emm_ctx.imsi = 100000000000002;
  denied.m_emm_ctx.imsi = 100000000000003;
  normal.m_emm_ctx.state = restricted.m_emm_ctx.state = denied.m_emm_ctx.state = EMM_STATE_REGISTERED;
  normal.m_ecm_ctx.state = ECM_STATE_IDLE;
  restricted.m_ecm_ctx.state = ECM_STATE_CONNECTED;
  denied.m_ecm_ctx.state = ECM_STATE_IDLE;
  normal.m_esm_ctx[5].state = ERAB_DEACTIVATED;
  restricted.m_esm_ctx[5].state = ERAB_ACTIVE;
  restricted.m_esm_ctx[5].qci = 9;
  set_normal(normal, "pdn_request", ACCESS_REASON_APN_MATCH, "172.16.0.2");
  set_restricted(restricted, "172.16.0.250");

  const std::string run = "unit-run";
  std::set<const nas*> contexts{&normal, &restricted, &denied};
  std::string rows = ue_snapshot::sessions(contexts, run);
  TESTASSERT(!rows.empty());
  TESTASSERT(rows.find("100000000000001") != std::string::npos);
  TESTASSERT(rows.find("100000000000002") != std::string::npos);
  TESTASSERT(rows.find("100000000000003") != std::string::npos);
  TESTASSERT(rows.find("\"access_policy\":\"normal\"") != std::string::npos);
  TESTASSERT(rows.find("\"access_reason\":\"apn_match\"") != std::string::npos);
  TESTASSERT(rows.find("\"access_policy\":\"restricted\"") != std::string::npos);
  TESTASSERT(rows.find("\"access_reason\":\"apn_mismatch\"") != std::string::npos);
  TESTASSERT(rows.find("\"access_policy\":\"deny\"") != std::string::npos);
  TESTASSERT(rows.find("\"access_reason\":\"session_unavailable\"") != std::string::npos);
  TESTASSERT(rows.find("\"selected_apn\":\"test\"") != std::string::npos);
  TESTASSERT(rows.find("\"requested_apn\":\"wrong.apn\"") != std::string::npos);
  TESTASSERT(rows.find("\"ecm_state\":\"idle\"") != std::string::npos);
  TESTASSERT(rows.find("k_asme") == std::string::npos && rows.find("nas_count") == std::string::npos);

  // Real map lifecycle without init(): no sockets, HSS, RF or worker thread.
  auto owner = s1ap::get_instance();
  auto mapped = new nas(args, interfaces);
  mapped->m_emm_ctx.imsi = 100000000000004;
  mapped->m_ecm_ctx.mme_ue_s1ap_id = 10;
  TESTASSERT(owner->add_nas_ctx_to_imsi_map(mapped));
  TESTASSERT(owner->add_nas_ctx_to_mme_ue_s1ap_id_map(mapped));
  mapped->m_ecm_ctx.mme_ue_s1ap_id = 11;
  TESTASSERT(owner->add_nas_ctx_to_mme_ue_s1ap_id_map(mapped));
  TESTASSERT(owner->find_nas_ctx_from_mme_ue_s1ap_id(10) == nullptr);
  mapped->m_ecm_ctx.mme_ue_s1ap_id = 0;
  TESTASSERT(owner->delete_ue_ctx(mapped->m_emm_ctx.imsi));
  TESTASSERT(owner->snapshot_sessions(run) == "[]");
  s1ap::cleanup();

  normal.clear_pdn_session();
  rows = ue_snapshot::sessions(contexts, run);
  TESTASSERT(!normal.m_apn_validated && normal.m_selected_apn.empty());
  TESTASSERT(normal.m_access_policy == srsran::ACCESS_POLICY_DENY);
  TESTASSERT(normal.m_access_reason == ACCESS_REASON_SESSION_UNAVAILABLE);
  TESTASSERT(normal.m_esm_ctx[5].state == ERAB_DEACTIVATED);
  const uint64_t generation = normal.m_session_generation;
  normal.reset();
  TESTASSERT(generation != normal.m_session_generation);

  contexts.erase(&restricted);
  rows = ue_snapshot::sessions(contexts, run);
  TESTASSERT(rows.find("100000000000002") == std::string::npos);
  normal.m_emm_ctx.imsi = 1010000000001ULL;
  rows = ue_snapshot::sessions(contexts, run);
  TESTASSERT(rows.find("001010000000001") != std::string::npos);
  normal.m_emm_ctx.imsi = 1000000000000000ULL;
  TESTASSERT(ue_snapshot::sessions(contexts, run).empty());
  normal.m_emm_ctx.imsi = 0;
  rows = ue_snapshot::sessions(contexts, run);

  TESTASSERT(ue_snapshot::quote("a\"\n\\") == "\"a\\\"\\u000a\\\\\"");
  char directory[] = "/tmp/lte-snapshot-test-XXXXXX";
  TESTASSERT(mkdtemp(directory));
  const std::string path = std::string(directory) + "/sessions.json";
  ue_snapshot::writer writer(path, run);
  TESTASSERT(writer.publish(rows, true, true));
  struct stat st{};
  TESTASSERT(stat(path.c_str(), &st) == 0 && (st.st_mode & 0777) == 0600);
  std::string current = read_file(path);
  TESTASSERT(current.find("\"schema_version\":2") != std::string::npos);
  TESTASSERT(current.find("\"sequence\":1") != std::string::npos);
  TESTASSERT(writer.publish(rows, true));
  TESTASSERT(read_file(path) == current);
  std::this_thread::sleep_for(std::chrono::milliseconds(1100));
  TESTASSERT(writer.publish(rows, true));
  TESTASSERT(read_file(path).find("\"sequence\":2") != std::string::npos);

  std::atomic<bool> reading{true}, complete{true};
  std::thread reader([&]() {
    while (reading.load()) {
      auto bytes = read_file(path);
      if (bytes.empty() || bytes.front() != '{' || bytes.back() != '\n' || bytes[bytes.size() - 2] != '}') {
        complete = false;
      }
    }
  });
  for (unsigned i = 0; i < 40; ++i) TESTASSERT(writer.publish(i % 2 ? rows : "[]", true, true));
  reading = false;
  reader.join();
  TESTASSERT(complete.load());
  TESTASSERT(writer.publish("[]", false, true));
  TESTASSERT(read_file(path).find("\"state\":\"stopped\",\"sessions\":[]") != std::string::npos);

  // Optional cross-language fixture: schema-2 output from the real producer,
  // containing one normal, one restricted and one fail-closed session.
  if (const char* export_path = std::getenv("LTE_TEST_SNAPSHOT_EXPORT")) {
    if (export_path[0] != '\0') {
      std::set<const nas*> contract_contexts{&normal, &restricted, &denied};
      normal.m_emm_ctx.imsi = 100000000000001;
      normal.m_emm_ctx.state = EMM_STATE_REGISTERED;
      set_normal(normal, "omitted", ACCESS_REASON_APN_OMITTED, "172.16.0.2");
      restricted.m_emm_ctx.state = EMM_STATE_REGISTERED;
      set_restricted(restricted, "172.16.0.250");
      denied.clear_pdn_session();
      denied.m_emm_ctx.imsi = 100000000000003;
      std::string contract_rows = ue_snapshot::sessions(contract_contexts, "contract-run");
      TESTASSERT(!contract_rows.empty());
      ue_snapshot::writer exporter(export_path, "contract-run");
      TESTASSERT(exporter.publish(contract_rows, true, true));
      const std::string exported = read_file(export_path);
      TESTASSERT(exported.find("\"schema_version\":2") != std::string::npos);
      TESTASSERT(exported.find("\"access_policy\":\"normal\"") != std::string::npos);
      TESTASSERT(exported.find("\"access_policy\":\"restricted\"") != std::string::npos);
      TESTASSERT(exported.find("\"access_policy\":\"deny\"") != std::string::npos);
    }
  }

  ue_snapshot::writer invalid(path, "invalid\nrun");
  TESTASSERT(!invalid.publish("[]", true, true));
  TESTASSERT(!writer.publish("", true, true));
  unlink(path.c_str());
  rmdir(directory);
  return 0;
}