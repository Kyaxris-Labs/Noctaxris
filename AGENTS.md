# AGENTS.md — Noctaxris

Guidance for coding agents in this repository. Follow unless a maintainer says otherwise.

## Product bar

- Go AWS-faithful local emulator. Secure by default. Loopback `:4566` (and IoT TLS `:8443` when enabled).
- No host `docker.sock`. Nested DinD only via Compose engine. Nested port publish default-off.
- Authn: SigV4 at the mux (fail-closed). Authz: `EvaluateFull` deny-by-default. Do not cache Allow/Deny decisions or Authz inputs.
- Prefer lab-complete cores over stubs. Honest theatre only for true stubs.
- Public docs under `docs/` (especially `docs/services/`). No roadmap labels (stage, campaign, tranche, phase, Track, set-N) in public surfaces.
- Module path: `github.com/Kyaxris-Labs/Noctaxris`. Hub image: `kyaxris/noctaxris`.

## JWT and crypto

- Use `internal/kernel/jwtutil` backed by `github.com/go-jose/go-jose/v4`.
- Alg allowlists only (RS256 / HS256 as needed). Reject `alg=none`. No new hand-rolled compact JWT sign/verify.
- Cognito, STS federation, and lab OIDC must go through jwtutil.

## Authz attach (DRY)

- Role attach at configure: call `CheckPassRole` (or the thin server helper wrapping it) with the correct service principal and source ARN.
- Do not invent a second PassRole engine. Do not skip PassRole because the caller is root unless existing root short-circuit applies.
- Cross-service delivery (IoT rules, Edge→S3, orchestration→CodeBuild, etc.) must EvaluateFull for the target action under the delivery role or caller identity. No direct store bypass for marketed paths.
- Object Lock governance bypass requires catalog action `s3:BypassGovernanceRetention` when the bypass header is true.

## Service-add checklist (auditability)

When adding or deepening a service, complete all of the following:

1. **Authn** — SigV4 service scope, or a documented public/alternate path (Cognito IdP, STS federation, anonymous S3 env, OpenDataPlane, webhook secret, mTLS).
2. **Authz** — concrete action name(s) in the catalog; resource ARN/name when AWS-shaped (avoid resource `*` unless intentional and documented).
3. **PassRole** — if the API accepts a RoleArn / instance profile / delivery role, call PassRole at configure (and delivery-time checks where AWS does).
4. **Secrets** — never echo secrets in List/Describe; listKeys-class actions only for key material.
5. **Data plane** — if a wire protocol exists, document authn (PLAINTEXT nested-only vs AUTH) and default publish posture.
6. **Docs** — `docs/services/<svc>.md` with deferred depth + verification/CLI smoke; update `docs/services/index.md` and `docs/security-defaults.md` public-route table when routes change.
7. **Tests** — at least: positive allow, negative deny (missing perm / wrong principal), boundary (empty/malformed), one error-guessing case (wrong service scope, missing PassRole, bypass header without perm). Soft-skip SDK/TF when endpoint unset.
8. **Coverage** — keep `./internal/...` statement coverage at or above 75%. Feature-oriented `*_test.go` names. Do not commit `coverage.out` or root coverage HTML.

## Libraries

- Prefer `go-jose`, `golang.org/x/crypto`, official AWS protocol shapes. Do not replace `EvaluateFull` with OPA/Casbin/Cedar.
- Nested Engine clients: `github.com/moby/moby/client` (not `github.com/docker/docker`).
- Latest dependency majors; pin GitHub Actions `uses:` to supported major tags.

## Docs and release

- CHANGELOG / README / `docs/release.md` stay aligned with siblings. No VERSION bump or tag/push unless asked.
- When not shipping: park under CHANGELOG Unreleased; leave tree dirty for a later cut.
- CI `ci-required` must pass before Hub publish. Failed tests must not push images.

## Privacy

- Do not put personal machine paths, home directories, or private workspace names in commits, docs, tests, comments, or CI logs.
- Keep public prose limited to this product and its cloud APIs.
