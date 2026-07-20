# ACM

**Status:** shipped (lab core)

Certificate Manager lite with lab self-signed certificates (stdlib `crypto/x509`). No public CA.

## Implemented

| Area | Actions |
|------|---------|
| Certificates | `RequestCertificate`, `DescribeCertificate`, `ListCertificates`, `DeleteCertificate` |

RequestCertificate issues a self-signed PEM for DomainName and stores it as Status ISSUED.

### Authz notes

Identity `EvaluateFull` on `acm:*`.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification).

```bash
aws acm request-certificate --domain-name lab.example.com --endpoint-url "$EP"
aws acm list-certificates --endpoint-url "$EP"
aws acm describe-certificate --certificate-arn "$ARN" --endpoint-url "$EP"
aws acm delete-certificate --certificate-arn "$ARN" --endpoint-url "$EP"
```

Lab protocol is JSON (`CertificateManager.*` X-Amz-Target). Live Compose smoke skipped when Docker is unavailable.

## Not yet / deferred

- Public CA issuance, DNS/email validation against real Route 53 propagation
- ImportCertificate workflows beyond stored PEM
- Certificate renewals and ACM PCA
