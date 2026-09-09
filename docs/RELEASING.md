# 自动发布 / Automated releases

## 当前契约 / Current contract

当前版本仍为 **2.1**。用户已确认 Docker Hub 仓库 **addxemmm/lte-system 为 Public**，GitHub 源码仓库可继续保持私有。现有 Secrets 为 `DOCKERHUB_USERNAME`、`DOCKERHUB_TOKEN`；Repository variable 为 `DOCKERHUB_IMAGE=addxemmm/lte-system`。发布脚本每次重新核实登录、目标仓库实际授予的 pull/push scope 和公开可见性，不靠截图或 Secrets 名称推断权限。

Current version remains **2.1**. The user has approved **Public** visibility for Docker Hub; the GitHub source repository may remain private. Configure the two named Secrets and image variable above. Every run checks authentication, actual granted pull/push scope and public visibility rather than assuming permissions from secret names.

[`release.yml`](../.github/workflows/release.yml) 是实际工作流；本文不代表任何一次执行已完成。只有工作流成功、Hub 远端 digest 和 GitHub Release 都可核对时，才算发布成功。旧 Web 测试镜像仍保留原工作树来源，不能冒充新 commit 的构建。

The linked workflow is implemented; this document does not assert that any particular run has completed. Success requires a verified remote digest and completed GitHub Release. The old server Web test image retains its original worktree provenance.

**首次正式发布已完成：** [2.1 验收记录](releases/2.1-automation-acceptance.md) 包含成功运行、公开镜像 digest、Release 附件与重复 resume 不修改产物的证据。 / **First formal publication completed:** the linked acceptance record contains successful runs, the public digest, Release assets and no-mutation resume evidence.

## 四种操作 / Four modes

GitHub → **Actions → release → Run workflow**，默认选择 `master`，mode 默认 `verify`。

Open Actions → release → Run workflow on master. The default mode is verify.

| mode | 行为 / Behavior |
|---|---|
| `verify` | 只核对准确源码、版本说明、登录/权限/公开状态；不构建或上传 / Check source, notes, login/scopes/visibility only |
| `build` | verify 后全量测试、完整构建、管理 smoke，保留 Actions 构建产物；不上传 Hub/创建 Release / Test/build/smoke and retain Actions artifacts only |
| `publish` | 完整 build 后上传镜像并完成中英 GitHub Release；已有镜像版本标签则拒绝覆盖 / Full build, publish image and release; existing image tags are not overwritten |
| `resume` | 不重建；从已有 Release 证据和匹配远端镜像恢复中断发布 / Resume matching remote image and original release evidence without rebuilding |

CLI 示例（先确认 CLI 登录正确 GitHub 账号）/ CLI examples:

以下 master 命令用于已准备好新版本、尚未发布的 commit。2.1 已发布；master 后补文档后仍为 2.1 时，版本身份检查会阻止从不同 commit 重发。核对已发布版本请使用原始 tag 的 `resume`。 / The master commands below apply to a prepared, unpublished version. Since 2.1 is published, later documentation commits retaining VERSION=2.1 are intentionally rejected as a different release source. Check an existing release using resume at its original tag.

```bash
gh workflow run release.yml --repo addxemmm/lte-system --ref master -f mode=verify
gh workflow run release.yml --repo addxemmm/lte-system --ref master -f mode=build
# This publishes a PUBLIC Docker image and creates a GitHub Release.
gh workflow run release.yml --repo addxemmm/lte-system --ref master -f mode=publish
gh run list --repo addxemmm/lte-system --workflow release.yml
# Verify legacy 2.1 after its SHA alias migration (read-only by default):
gh workflow run cleanup-legacy-21-tag.yml --repo addxemmm/lte-system --ref master -f delete_alias=false
```

普通 master push 只运行 `ci.yml`。推送符合版本规则的 `v*` tag 会自动运行 publish；请将创建版本 tag 视为公开镜像发布操作。所有 release 执行串行，不取消正在上传的任务。

Ordinary master pushes run CI only. Pushing a valid v-prefixed version tag automatically runs publish: creating that tag is a public-publication action. Release runs serialize and do not cancel in-flight uploads.

## 版本迭代 / Version iteration

`VERSION` 通过 Go embed 成为运行元数据的单一来源。发布 Git tag 必须等于 `v` + VERSION；**Docker Hub 每次发版只创建 VERSION 一个标签**，不创建 `sha-<commit>` 或 latest。完整 commit 继续记录在 OCI revision、Release 元数据及证据包中，镜像内容用 digest 校验。2.1 是唯一两段历史例外；以后使用三段版本，如 `2.1.1`、`2.2.0`，预发布支持 `2.2.0-rc.1`（也支持 alpha/beta）。不覆盖正式标签；历史 2.1 不是将来可移动的 minor alias。新版本不会自动删除旧版本标签。

VERSION is embedded into the Go binary. Git tags are v + VERSION; Docker Hub receives **one version tag per release**, without SHA or latest aliases. Full commit identity remains in OCI labels, Release metadata and evidence, with digest verification. Legacy 2.1 is the only two-component exception. Future versions use three components and optional numbered rc/alpha/beta prereleases. Published version tags are never overwritten or automatically removed when a new version is released.

每次迭代 / Each iteration:
1. 修改 VERSION，并新增 `docs/releases/<VERSION>.md`（含 `## 中文` 和 `## English`），更新 CHANGELOG。
   Update VERSION, bilingual version notes and CHANGELOG.
2. 测试、审查并提交到 master。Compose 本地构建设置 `LTE_VERSION` 与 `LTE_IMAGE`；直接 docker build 显式传 `OCI_VERSION`、`OCI_REVISION`，值必须与 VERSION 匹配。旧 2.1 默认值保留兼容，未来版本遗漏参数会构建失败而非错误标记。
   Review/test/commit to master. For local Compose builds set LTE_VERSION/LTE_IMAGE; pass matching OCI_VERSION/OCI_REVISION for direct builds. The historical 2.1 defaults remain compatible; a future mismatch fails rather than mislabels.
3. 首先手动 verify/build，确认后 publish；或推送准确 commit 上的版本 tag。tag 已存在时不移动。
   Verify/build first, then publish or push the exact version tag. Never move an existing tag.
4. 检查 Actions、Hub Tags、GitHub Release 与附件；部署另行确认。
   Inspect the run, Hub tags, Release and attachments. Deployment is separate.

## 实际门禁 / Implemented gates

- **源码**：只允许 master 或与 VERSION 完全匹配的版本 tag；解析完整 SHA，必须在已获取 master 历史中，已存在 tag 必须指向同一 SHA。源工作区干净；发行说明必须双语。
  **Source:** exact reviewed commit, master ancestry, matching version/tag and bilingual notes.
- **凭据隔离**：verify/build/publish 三个 job。build 无 Hub Secrets，只有 publish 获得 contents:write（下载本次产物另需 actions:read）。固定官方 Actions commit SHA，checkout 不持久保存凭据；禁止凭据请求重定向。
  **Isolation:** separate jobs; no registry secrets in build; scoped publication permissions; pinned actions and no persisted checkout credentials.
- **完整构建**：GitHub 隔离 Linux runner，完整 Dockerfile，不使用服务器旧镜像增量升级。Go/vet/race、Python/JS、shell、FPGA 哈希，以及 Dockerfile 七项 CTest 全部作为门禁。当前仅 linux/amd64。
  **Build:** isolated full source build with all CPU/contract/firmware checks and seven CTests, initially linux/amd64 only.
- **镜像 smoke**：无 USB/特权/宿主端口，network none、只读根目录、临时 data；跳过硬件入口直接启动 Go，核对 18081、默认 8081 关闭、启用后共享鉴权、元数据、版本标签及对应源码。
  **Smoke:** hardware-free, unprivileged, no host ports, read-only root and temporary data, direct Go entrypoint and listener/auth/identity/source checks.
- **同一产物**：测试后的 docker save 压缩产物跨 job 传递，校验归档 SHA-256 和 image ID 后才 push；没有第二次构建。
  **Artifact identity:** transfer the exact tested archive; verify checksum/image ID before pushing, with no second build.
- **发布顺序**：先创建草稿并上传构建证据，再推唯一版本标签；记录远端 digest，版本标签指向准确镜像且匿名拉取可用后才完成 Release。
  **Order:** draft and evidence first, then the single version tag, verified digest and anonymous pull before completing the Release.
- **不自动部署**：整个流程不 SSH 到服务器，不启动小区、不改 GSM、不删除现场镜像或数据。
  **No deployment:** no server SSH, cell startup, GSM changes or on-server cleanup.

## 失败恢复 / Failure recovery

构建/测试失败：没有镜像发布，修复后重新运行。已出现 Hub 版本标签：不要再次 publish 覆盖，选择 `resume`，执行分支选原版本 tag；若尚无 tag，则 master 必须仍为原 commit。恢复要求原构建附件完整、校验和覆盖完整、版本 digest 与记录一致、OCI 与版本/commit 匹配，且远端 image ID 等于原 smoke 结果。已有完整 Release 只核对，不修改。历史证据中的 `source_tag` 只保留为旧记录，新发布不生成该字段或别名。

Before-upload build/test failures can be fixed and retried. If a registry version tag exists, use resume at the original tag/commit instead of overwriting. Resume requires original evidence/checksums, a consistent version digest, matching OCI labels and the exact tested image ID. A completed matching Release is checked without mutation. Legacy source_tag metadata is historical only; new publication creates neither that field nor an alias.

### 2.1 历史标签迁移 / Legacy 2.1 tag migration

`v2.1` 中的旧脚本仍要求两个标签，不移动 Git tag 或覆盖已发布镜像来更新它。删除 SHA 别名后，旧 `v2.1` 的 `resume` 会拒绝缺失的别名；请用 master 的 `cleanup-legacy-21-tag` 工作流只读核对这个已完成版本。该工作流默认 `delete_alias=false`，只允许核对固定的 2.1 Release、commit 和 digest；显式 true 时仅删除准确的旧 SHA 别名，保留 2.1，不删除共享 manifest。若 Token 缺少 Delete 权限，操作会失败，需在 Hub 手动删除该别名或另行配置有删除权限的凭据；不要删除整个镜像。未来版本使用新版单标签 resume。

The immutable v2.1 source still contains the old two-tag workflow. After alias removal its old resume rejects the missing alias; use the master cleanup-legacy-21-tag workflow's default read-only verification for this completed release. It pins the original release/commit/digest; explicit delete_alias=true removes only that exact alias, never the retained version tag or shared manifest. A token without Delete permission causes the operation to fail; remove the alias in Hub or separately provision suitable credentials. Future releases use the updated single-tag resume.

若证据包已上传但首个镜像 push 尚未完成，resume 会从原构建 run 下载同一 tested-image 归档，校验后继续；该传递归档仅保留三天，过期后需人工恢复原产物，不重新构建覆盖。只有部分附件、缺失原始证据或 digest 冲突时，会停止并要求人工核对，不通过删除/覆盖版本来掩盖失败。发布完成前 Release 保持 draft。同一 repo 的工作流并发锁不约束外部手工 docker push；应限制外部写入并保护 master/版本 tag，不宣称这些 GitHub/Hub 账户规则已自动配置。

If evidence exists but no image was pushed, resume downloads and verifies the original run’s tested-image artifact (three-day retention); if expired, restore the original artifact manually rather than rebuilding under the same version. Incomplete evidence or conflicting digests require manual reconciliation, not destructive replacement. Workflow serialization does not prevent external manual pushes; restrict other writers and protect branches/tags separately. Account protection rules are not automatically installed.

## 产物、许可与边界 / Evidence, licensing and limits

不可变 `build-evidence-<SHA>.zip` 在首次 push 前一次上传，内含原始构建证据及完整 SHA256SUMS，不在更新 digest 时覆盖；恢复先验证整个包再读取。最终单独附加 `release.json` 与其校验和。证据内容包括 `release.json`（source SHA、版本、原构建链接、远端 digest）、双语 notes、准确 Git 源码归档、Debian 包清单、smoke 结果、测试 image ID、镜像归档哈希及 SHA256SUMS。GitHub Release/源码可继续私有；公开镜像用户可在镜像内获得匹配 patched srsRAN 源码、pySIM 源码及构建材料，参见 [第三方说明](../THIRD_PARTY_NOTICES.md)。

An immutable build-evidence-<SHA>.zip is uploaded once before the first push, with complete internal checksums; digest updates never overwrite it. Resume validates the whole bundle before using it. Final release.json and its checksum are separate append-only attachments. Evidence includes identity/build metadata, notes, source, inventory, smoke, image ID and checksums. Private GitHub access is not required to extract bundled corresponding srsRAN/pySIM source and build materials from the public image.

不将全部镜像标为 MIT，也不声称厂商固件已自动取得再分发许可。发布者需保留相应许可/授权材料。当前包清单不是标准 SBOM，尚未生成签名/provenance，也尚未接入漏洞数据库扫描。Go 正式构建/CI 使用 1.22，Web 升级辅助文件使用 1.26.8；基础镜像与 apt/pip 仍有浮动输入，可追溯不等于逐位可复现。这些是后续加固项，不伪装为已通过的门禁。

Do not label the whole image MIT or claim automatic vendor-firmware redistribution permission. Retain applicable license/permission records. Package inventory is not a standard SBOM; signing, provenance and vulnerability-database scanning are not yet implemented. Full builds/CI use Go 1.22; the Web-upgrade helper uses 1.26.8. Floating dependencies preclude a bit-for-bit reproducibility claim.

Actions evidence 保留 14 天，大镜像传递归档保留 3 天以控制存储；正式 Release 附件和 Hub 正式标签独立保留，不依赖短期 Actions artifact。服务器继续遵从不保留本地回滚镜像的要求，后续按 digest 拉取或部署需要另行操作。

Actions evidence lasts 14 days and the large transport archive three days. Release attachments and registry tags persist independently. Existing server retention and stopped-service state remain unchanged.

## 官方参考 / Official references

- [Docker GitHub Actions](https://docs.docker.com/build/ci/github-actions/)
- [Test before push](https://docs.docker.com/build/ci/github-actions/test-before-push/)
- [Registry authentication/scopes](https://docs.docker.com/reference/api/registry/auth/)
- [GitHub Releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases)
- [GitHub Secrets](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets)
