# 版本与镜像发布 / Versioning and image releases

## 1. 当前状态 / Current state

运行/镜像版本仍为 **2.1**。当前已有源码 CI 和服务器测试部署记录，**尚无镜像上传或 GitHub Release 自动发布工作流**。普通 `master` push 只触发现有 CI，不部署服务器、不启动小区。Docker Hub 已决定先用 **Private**；命名空间、仓库和凭据仍需配置。

Runtime/image version remains **2.1**. Source CI and server test-deployment records exist; **image publication and GitHub Release automation do not yet exist**. An ordinary `master` push triggers CI only, not server deployment or cell startup. Docker Hub visibility is now selected as **Private**; namespace, repository and credentials still need configuration.

2026-09-09 的 Web 测试镜像来自构建时工作树快照，并非之后的 Git commit。LTE 容器随后按要求正常停止，保留镜像与数据。完整来源和停机证据见 [验收记录](WEB_UI_RELEASE_2026-09-09.md)。本地 Image ID 不是注册表 manifest digest，也不代表镜像已经上传。

The 2026-09-09 Web test image came from a build-time working-tree snapshot, not a later Git commit. LTE was subsequently stopped as requested, retaining its image/data. See the [acceptance record](WEB_UI_RELEASE_2026-09-09.md). A local image ID is neither a registry manifest digest nor evidence of publication.

## 2. 绑定私有 Docker Hub / Connect private Docker Hub

“绑定”指 GitHub Actions 使用 Docker Hub 专用令牌，不要求两个平台同名，也不需要把本地 Docker 配置上传到 GitHub。

Connecting means letting GitHub Actions use a dedicated Docker Hub token. The platform usernames need not match; do not upload local Docker credential configuration to GitHub.

1. 登录 Docker Hub → **My Hub / Repositories → Create repository**。选择自己的 namespace，仓库名例如 `lte-system`，Visibility 选择 **Private**，先创建再上传，避免误建公开仓库。确认账户的私有仓库额度足够。
   Sign in to Docker Hub, create a repository under your namespace, and explicitly select **Private** before uploading. Confirm the account has sufficient private-repository entitlement.
2. Docker 账号设置 → **Personal access tokens → Generate new token**。用途例如 `github-lte-release`，只授予所需读写权限，不授予删除权限，设置过期时间。Token 只在生成时显示，保存到密码管理器。
   Create a dedicated PAT with the necessary read/write permissions, no delete permission, and an expiration date. Store the one-time-displayed token in a password manager.
3. GitHub 仓库 → **Settings → Secrets and variables → Actions**，按下表配置。Token 不发聊天、不提交文件、不写入 Dockerfile、构建参数或工作流输出。
   In GitHub repository Settings → Secrets and variables → Actions, configure the following. Never put tokens in chat, tracked files, Dockerfiles, build arguments or workflow output.

| 类型 / Type | 名称 / Name | 值 / Value |
|---|---|---|
| Repository secret | `DOCKERHUB_USERNAME` | Docker Hub 用户名，不是邮箱 / Docker Hub username, not email |
| Repository secret | `DOCKERHUB_TOKEN` | 专用读写 PAT / Dedicated read/write PAT |
| Repository variable | `DOCKERHUB_IMAGE` | `namespace/lte-system`，不含 tag 或 URL scheme / No tag or URL scheme |

可以使用已登录正确 GitHub 账号的 CLI **交互输入**秘密，不把值写进命令历史：

With the CLI authenticated as the correct GitHub account, enter secrets **interactively**, without putting their values in command history:

```bash
gh secret set DOCKERHUB_USERNAME --repo OWNER/REPO
gh secret set DOCKERHUB_TOKEN --repo OWNER/REPO
gh variable set DOCKERHUB_IMAGE --repo OWNER/REPO --body 'namespace/lte-system'
# Lists names, never the stored secret values.
gh secret list --repo OWNER/REPO
```

这些配置本身不会上传镜像。后续发布 job 才读取它们并登录 Hub；本地 `docker login` 也不会自动给 GitHub runner 授权。服务器日后只拉镜像时使用另一个 **Read-only** Token，不复制发布用读写 Token。令牌轮换时同步更新 GitHub Secret，并撤销旧令牌。

Configuration alone does not publish images. A future release job must read these values and authenticate to Hub; local `docker login` does not authenticate a GitHub runner. Use a separate **read-only** token for future server pulls. Rotate the GitHub secret and revoke the previous PAT together.

官方参考 / Official references: [创建仓库 / Create repository](https://docs.docker.com/docker-hub/repos/create/), [Docker PAT](https://docs.docker.com/security/access-tokens/personal-access-tokens/), [GitHub Secrets](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets).

## 3. 未来版本规则 / Future version policy

保留当前 2.1，不补造历史 tag，不覆盖历史正式版本。之后经确认采用三段 SemVer，例如修复 `2.1.1`、兼容新功能 `2.2.0`、破坏性变化 `3.0.0`；预发布可用 `2.2.0-rc.1`。以下均是未来规则，不表示这些版本已经发布。

Keep current 2.1, without inventing historical tags or overwriting formal versions. Future approved releases use three-part SemVer, e.g. fix `2.1.1`, compatible feature `2.2.0`, breaking change `3.0.0`, or prerelease `2.2.0-rc.1`. These are policy examples, not released versions.

| 标识 / Identifier | 规则 / Rule |
|---|---|
| `VERSION` | 单一版本来源，例如 `2.1.1` / Single source, e.g. `2.1.1` |
| Git tag | `v` + `VERSION`，例如 `v2.1.1` / `v` prefix plus VERSION |
| Docker version tag | `namespace/lte-system:2.1.1`，正式发布后不覆盖 / Never overwrite after formal publication |
| Docker source tag | `sha-<full-commit-sha>`，指向同一构建 / Same build, full Git SHA |
| 部署引用 / Deployment | `namespace/lte-system@sha256:<manifest-digest>`，从远端读取并记录 / Record the verified remote digest |
| 可选别名 / Optional aliases | `2.1`、`latest` 只在稳定发布完成后显式更新；预发布不更新 / Update explicitly only after stable publication |

未来实现须让 UI 元数据、Compose、构建参数及 OCI version/revision/source 标签由 `VERSION` 和完整 Git SHA 驱动，校验 Git tag 的去前缀版本一致。**当前部分值仍硬编码，这项统一尚未实现。** 当前完整 Dockerfile/CI 使用 Go 1.22，Web 升级使用 Go 1.26.8；正式发布前需确定并统一经测试的工具链策略。

Future implementation must drive UI metadata, Compose, build parameters and OCI version/revision/source labels from `VERSION` and the full Git SHA, validating the stripped Git tag. **Some current values remain hard-coded; this unification is not implemented.** Full builds/CI currently use Go 1.22 while Web upgrades use Go 1.26.8; settle and validate a consistent toolchain policy before formal publication.

## 4. 发布流水线方案（待实现） / Publication pipeline (planned)

```text
reviewed master commit + VERSION + bilingual notes
                 ↓
manual approval / protected version tag
                 ↓
Go + race + Python/JS + shell + version checks
                 ↓
clean full Dockerfile build + seven CTests + management-only smoke
                 ↓
secret/license/vulnerability review + SBOM/provenance
                 ↓
private Docker Hub push → verify remote digest
                 ↓
GitHub Release: notes + source/build materials + checksums + image digest
```

- **触发与权限**：使用手动发布入口或受保护的版本 tag，不在每次 master push/PR 上传。发布前核实 tag 属于已审核 master 历史；任务按仓库和版本串行，拒绝覆盖已有正式标签。测试 job 不拿 Hub 凭据，发布 job 才使用 Secrets。按当前私有仓库套餐支持情况配置受保护 Environment/审批；不假定该能力已开启。使用审核过的固定 Actions commit SHA。
  **Triggers/permissions:** manual release or protected version tags, never every branch push/PR. Verify tag ancestry, serialize releases and reject overwrites. Keep registry secrets out of test jobs. Add protected environments/approval where supported by the private-repository plan; do not assume they are enabled. Pin reviewed action commit SHAs.
- **构建**：从干净、已提交的准确 tag，使用 `deploy/docker/Dockerfile` 完整构建；先只支持 `linux/amd64`。在无 SDR/USB 的专用 Linux 构建节点运行，不在开发机或正在运行 GSM 的硬件服务上构建发布；管理 smoke 跳过正常入口硬件探测，不启动小区。`Dockerfile.web-upgrade` 是已验证基础环境的局部升级工具，不替代正式全量构建。
  **Build:** use clean committed source and the full Dockerfile, initially `linux/amd64` only, on a dedicated Linux builder without SDR/USB. Do not use the development machine or active GSM hardware service. Management smoke must skip hardware probes and cell startup. The trusted-base Web upgrade is not a substitute for an official full build.
- **测试与证据**：现有 Go/vet/race、Python/JS 和 shell 检查全部通过；完整构建运行七项 CTest。增加镜像级默认 API 关闭、双口/鉴权/元数据测试，再做漏洞和秘密检查。CPU/mock 结果不冒充 RF 或手机验收。
  **Tests/evidence:** pass Go/vet/race, Python/JS and shell checks, plus seven full-build CTests. Add image-level listener/auth/metadata tests and secret/vulnerability review. CPU/mock results never substitute for RF or handset acceptance.
- **上传与记录**：构建一次，将版本/SHA 标签指向同一结果，读取远端 digest，记录源码 commit、构建运行链接、工具链、SBOM、provenance 和校验和。基础镜像、apt/pip、固件输入目前仍可能变化；可追溯不等于逐位可复现，后续逐项锁定并验证。私有镜像的证明材料也要检查是否暴露敏感构建信息。
  **Publish/record:** build once, use version/SHA tags for that result, verify the remote digest and record source/build/toolchain/SBOM/provenance/checksums. Current base-image, apt/pip and firmware inputs may drift: traceable is not bit-for-bit reproducible. Review private-image attestations for sensitive build metadata.
- **失败与 Release**：任何门禁失败就不完成 Release；Hub 上传成功但后续记录失败时，保留草稿和已验证 digest，从原产物恢复，不重新覆盖正式版本。先准备全部附件，再发布 Release；若启用 GitHub immutable releases，发布后关联 tag/附件有额外不可变约束。`latest`/次版本别名只在成功后更新。
  **Failure/release:** leave the Release incomplete on gate failure. If upload succeeds but release recording fails, retain the draft and verified digest and resume from the same artifact. Attach all files before publishing, respecting optional GitHub immutable-release constraints. Update moving aliases only after success.

方案基于 [Docker GitHub Actions](https://docs.docker.com/build/ci/github-actions/)、[GitHub Releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases) 和 [immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases)。这些工作流、门禁与证明产物目前尚未交付。

The plan follows those official references. The publication workflow, extra gates and attestation artifacts are **not yet delivered**.

## 5. 分发与敏感数据检查 / Distribution and private-data checks

发布前检查最终镜像及构建上下文，不只检查最终文件系统：不得包含真实订户 Ki/OPc、Token、密码、私有 YAML、运行数据、抓包或服务器凭据；删掉上层文件不会抹掉旧镜像层。使用例子配置，不直接把服务器旧镜像 retag 成可公开分发的正式镜像。

Review build context and image layers, not only the final filesystem: exclude real subscriber secrets, tokens/passwords, private YAML, runtime state, captures and server credentials. Deleting an upper-layer file does not erase old layers. Use example configuration, not a blindly retagged server image.

项目自身 MIT 标识不覆盖整镜像所有组件。发布前逐项核对 srsRAN/pySIM 等第三方许可、匹配版本的修改后对应源码、补丁、构建材料与 notices，以及兼容固件的再分发授权。私有发布也要评估接收者的许可/源码获取要求；公开 Hub 接收者未必能访问私有 GitHub。将来转公开应单独确认授权和源码交付，不由工作流自动改变可见性。

The project's MIT label does not cover every image component. Review third-party licenses, matching modified source, patches, build materials/notices and firmware redistribution rights before publication. Assess recipient/source-access requirements for private distribution too; public Hub recipients may lack private GitHub access. A future switch to public needs separate review, never an automatic visibility change.

## 6. 发版记录与部署分离 / Release records versus deployment

每次正式版本新增 `docs/releases/<VERSION>.md`，使用 [双语模板](releases/TEMPLATE.md)，并更新 [CHANGELOG](../CHANGELOG.md)。记录变化、兼容性/迁移、已做与未做测试、commit、Git tag、镜像 digest、已知问题及数据恢复条件。历史测试部署记录保持日期和真实来源，不改写成当时不存在的 Release。

Add `docs/releases/<VERSION>.md` from the [bilingual template](releases/TEMPLATE.md) and update the [changelog](../CHANGELOG.md). Record changes, compatibility, migration, performed/omitted tests, Git identity, image digest, known issues and data recovery conditions. Preserve historical test-deployment evidence without inventing releases.

发布成功不自动部署、不启动 RF、不改 GSM。现场部署仍需单独确认，核对实际数据卷并备份。遵从当前要求，不重新保留服务器本地回滚镜像；注册表历史版本可用于追溯或日后按 digest 拉取，但旧二进制不保证兼容新数据，恢复前必须核对数据格式与备份。

Publication never automatically deploys, starts RF or modifies GSM. Deployment requires separate confirmation and verified data mounts/backups. Do not reintroduce local rollback images against the current retention request. Registry history can support traceability or future digest pulls, but an older binary is not guaranteed compatible with newer data.
