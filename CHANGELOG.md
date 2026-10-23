# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.1.0 (2026-10-23)


### Features

* add firmware inspection CLI and PostgreSQL evidence store ([15329b1](https://github.com/deploymenttheory/go-ios-vm/commit/15329b10cd90f156454cc63bc78af541546050e3))
* assess exact finding revisions with history ([a11727d](https://github.com/deploymenttheory/go-ios-vm/commit/a11727d48f3afbcf55979e4721cc29c72da0f320))
* assess exact finding revisions with history ([ba27ca1](https://github.com/deploymenttheory/go-ios-vm/commit/ba27ca1d50f3ee8e37ec9106748c02a8eff72b55))
* bound native LZFSE payload decoding ([#61](https://github.com/deploymenttheory/go-ios-vm/issues/61)) ([fe0de30](https://github.com/deploymenttheory/go-ios-vm/commit/fe0de30dc85832ff0f8cb9ae3093a12db3b654e1))
* **cli:** emit versioned structured diagnostics and concise text summaries ([#29](https://github.com/deploymenttheory/go-ios-vm/issues/29)) ([83a9bb2](https://github.com/deploymenttheory/go-ios-vm/commit/83a9bb207201c1087632dc1fdb607148962ca983))
* establish firmware inspection CLI and PostgreSQL evidence store ([4be8892](https://github.com/deploymenttheory/go-ios-vm/commit/4be8892832fe4d557ba024a8b5f6a1c7f87c5d44))
* expose and enforce pinned-image resource budgets ([ac7fac1](https://github.com/deploymenttheory/go-ios-vm/commit/ac7fac16ebc995a4f67be8556a613563d0e09040))
* **findings:** add stable identities and linked evidence revisions ([#28](https://github.com/deploymenttheory/go-ios-vm/issues/28)) ([000af31](https://github.com/deploymenttheory/go-ios-vm/commit/000af31a088f3092483e3dad7e8b10a692ab56c4))
* **firmware:** list and search restore IPSW locators ([#24](https://github.com/deploymenttheory/go-ios-vm/issues/24)) ([e51fc1f](https://github.com/deploymenttheory/go-ios-vm/commit/e51fc1f279812f20cc0462a81cf9d2a440d5b254))
* **firmware:** qualify historical iPhone 11 manifests and resumable acquisition ([#21](https://github.com/deploymenttheory/go-ios-vm/issues/21)) ([1093c8b](https://github.com/deploymenttheory/go-ios-vm/commit/1093c8b2a6dad23b0c74066f8f9560d5edd951a8))
* **inspection:** retain bounded APFS firmware file content ([8be227f](https://github.com/deploymenttheory/go-ios-vm/commit/8be227f60c5aecf60c9d9adec58fb2fd4a930eff))
* **inspection:** retain bounded APFS firmware file content ([590300f](https://github.com/deploymenttheory/go-ios-vm/commit/590300f71f04b926fb1338416e000f5a351e95ea))
* **inspection:** stage ZIP-compressed OS disk members within limits ([#26](https://github.com/deploymenttheory/go-ios-vm/issues/26)) ([78f56ce](https://github.com/deploymenttheory/go-ios-vm/commit/78f56ce387331d9b0a45d9e460c6b73a7407fb2a))
* persist bounded acquisition and inspection progress ([#56](https://github.com/deploymenttheory/go-ios-vm/issues/56)) ([09d4dce](https://github.com/deploymenttheory/go-ios-vm/commit/09d4dcecb586301dce9d1c08413ee50f4cc2a961))
* publish bounded inspection batches with explicit resume ([1f5a328](https://github.com/deploymenttheory/go-ios-vm/commit/1f5a328b8f359172163de4d6a89c05ce584e1855))
* publish bounded inspection batches with explicit resume ([18652e2](https://github.com/deploymenttheory/go-ios-vm/commit/18652e2e706479202ec0a5048eb424b98dec9c25))
* **qualification:** qualify exact iPhone12,1 iOS 14.0 image ([#22](https://github.com/deploymenttheory/go-ios-vm/issues/22)) ([d4035e7](https://github.com/deploymenttheory/go-ios-vm/commit/d4035e717e5efda0df2eb7059040c46e99faa21d))
* reconcile abandoned inspection runs and retry finalization ([59295db](https://github.com/deploymenttheory/go-ios-vm/commit/59295dbdf499cbcff61c37eb18b1d6b676c24914))
* reconcile crashed inspection runs and retry finalization ([c0085dc](https://github.com/deploymenttheory/go-ios-vm/commit/c0085dcea05603f7a314ac56e8c387b858b79371))
* **resources:** expose and enforce pinned image budgets ([f788d0b](https://github.com/deploymenttheory/go-ios-vm/commit/f788d0bb97dd4a668ed4b4da67b976f59bdbcc99))
* **ui:** add bounded acquisition and run read APIs ([11225c3](https://github.com/deploymenttheory/go-ios-vm/commit/11225c38f911069f6eab7dabd64c004f19bf3a0e))
* **ui:** add bounded acquisition and run read APIs ([4a9f025](https://github.com/deploymenttheory/go-ios-vm/commit/4a9f025fa4010edf593a0e78122af3d552008352))
* **ui:** browse findings and retained evidence ([#57](https://github.com/deploymenttheory/go-ios-vm/issues/57)) ([04583c8](https://github.com/deploymenttheory/go-ios-vm/commit/04583c8d54ae9371af91dc854f2c28af9a5c6ff5))
* **ui:** browse targets, findings, and evidence ([#59](https://github.com/deploymenttheory/go-ios-vm/issues/59)) ([12deb0f](https://github.com/deploymenttheory/go-ios-vm/commit/12deb0ffa02d4dadc05ad9a4fdacaa22ae3ad34f))
* **ui:** expose bounded source-backed relationship graph ([#60](https://github.com/deploymenttheory/go-ios-vm/issues/60)) ([57be9a9](https://github.com/deploymenttheory/go-ios-vm/commit/57be9a9d63b3b82f1894042c42c842e257d4b812))
* **ui:** expose durable live progress reads ([8efb63a](https://github.com/deploymenttheory/go-ios-vm/commit/8efb63a877dc1b2cc452b5bd51356108145a9f1a))
* **ui:** expose durable live progress reads ([b9b6182](https://github.com/deploymenttheory/go-ios-vm/commit/b9b6182a72efb737fac0b8b7b9a5f94d8e56a40c))
* **ui:** monitor acquisition and inspection progress ([#62](https://github.com/deploymenttheory/go-ios-vm/issues/62)) ([1cf0c7d](https://github.com/deploymenttheory/go-ios-vm/commit/1cf0c7d8c4f90ed9f2b23ebf442958da0813edd7))
* **ui:** serve local read-only inspection shell ([97d0547](https://github.com/deploymenttheory/go-ios-vm/commit/97d0547440dcd61c62058c187850339c732d5100))
* **ui:** serve local read-only inspection shell ([50bc2fc](https://github.com/deploymenttheory/go-ios-vm/commit/50bc2fce669da5100a3952dd5abeaf71ad80e0fa))


### Bug Fixes

* **ci:** correct service quoting and update Go-compatible linter ([eb87474](https://github.com/deploymenttheory/go-ios-vm/commit/eb874746519af90352a0b7504f4b79604e3aa810))
* **ci:** lint the Go module and resolve enforced findings ([87505a5](https://github.com/deploymenttheory/go-ios-vm/commit/87505a5d05b478e1cf0b54423853d5a505a3b196))
* **ci:** use Super-Linter module allowlist consistently ([1c3cc40](https://github.com/deploymenttheory/go-ios-vm/commit/1c3cc40862f16c02bb4e9473343e5c1528eb40be))
* **inspection:** decode IOKit IDREF kernel metadata ([#27](https://github.com/deploymenttheory/go-ios-vm/issues/27)) ([8fa041d](https://github.com/deploymenttheory/go-ios-vm/commit/8fa041daea991efaea31fa85bac6c91f01476fc0))
* report changed resume archives before identity selection ([8dfed53](https://github.com/deploymenttheory/go-ios-vm/commit/8dfed5301643b3e5c01ffaf0c1d912a087f085de))

## [Unreleased]

### Added

- Added xyz [@your_username](https://github.com/your_username)

### Fixed

- Fixed zyx [@your_username](https://github.com/your_username)

## [1.1.0] - 2021-06-23

### Added

- Added x [@your_username](https://github.com/your_username)

### Changed

- Changed y [@your_username](https://github.com/your_username)

## [1.0.0] - 2021-06-20

### Added

- Inititated y [@your_username](https://github.com/your_username)
- Inititated z [@your_username](https://github.com/your_username)
