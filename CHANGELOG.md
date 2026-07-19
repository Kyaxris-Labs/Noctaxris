# Changelog

## Lab core (phases 0-7)

First shippable lab surface for Docker-first local AWS-shaped labs.

### Included

- Secure Compose defaults: loopback publish, no host `docker.sock`, sealed secrets
- SigV4 (header and query), IAM lab APIs, all 11 STS actions, Organizations CreateAccount
- Lab-complete KMS (keys, policies, crypto, grants, aliases)
- Lab-complete S3 (path-style objects, bucket policy, SSE-S3/SSE-KMS, presigned GET/PUT)
- Lab-complete DynamoDB and SQS (items/messages, resource/queue policies, encryption options)
- Lab-complete Lambda (zip `python3.12`, PassRole + `lambda.amazonaws.com` trust, nested DinD sync Invoke)

### Explicitly later

See [docs/deferred.md](docs/deferred.md) for SAR depth, FIFO, image-based Lambda, microVMs, and condition-key codegen.
