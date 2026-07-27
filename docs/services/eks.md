# EKS (Elastic Kubernetes Service)

**Status:** shipped (lab core; control-plane metadata only)

Cluster CRUD for Amazon EKS over REST-JSON (`/clusters`). Clusters become `ACTIVE` immediately with a nested-network endpoint string. There is no nested k3s container and no live Kubernetes API server: `kubectl`, `update-kubeconfig`, and CA data are not supported.

## Implemented

| Area | Actions |
|------|---------|
| CRUD | `CreateCluster`, `DescribeCluster`, `ListClusters`, `DeleteCluster` |
| Nodegroups | `ListNodegroups` empty stub (`nodegroups: []`) |
| Protocol | REST-JSON `POST/GET/DELETE /clusters`, `GET /clusters/{name}`, `GET /clusters/{name}/node-groups` |
| Status | `ACTIVE` on create (metadata-only); delete returns `DELETING` then removes the row |
| Endpoint | `https://noctaxris-eks-<name>:6443` (nested-network string only; not reachable, not host-published) |

### Authz notes

Identity `EvaluateFull` on `eks:*`. Sign as SigV4 service `eks`.

### Why not nested k3s

The DinD dataplane keeps `Privileged: false` and drops capabilities by default. `rancher/k3s` needs a privileged container and a live API on `:6443`. Extending privileged nested starts for EKS would weaken the secure DinD posture, so this cut ships honest control-plane metadata instead.

## How to verify / CLI smoke

Shared Compose and env setup: [index.md](index.md#shared-verification). No `noctaxris-engine` required for EKS metadata.

```bash
aws eks create-cluster \
  --name "noctaxris-eks-$RANDOM" \
  --role-arn arn:aws:iam::000000000001:role/eks \
  --resources-vpc-config subnetIds=subnet-1,securityGroupIds= \
  --kubernetes-version 1.29 \
  --endpoint-url "$EP"

aws eks list-clusters --endpoint-url "$EP"
aws eks describe-cluster --name noctaxris-eks-... --endpoint-url "$EP"
aws eks list-nodegroups --cluster-name noctaxris-eks-... --endpoint-url "$EP"
aws eks delete-cluster --name noctaxris-eks-... --endpoint-url "$EP"
```

DescribeCluster shows `ACTIVE` and a nested endpoint string. `certificateAuthority` has no `data`. Do not run `aws eks update-kubeconfig` or `kubectl` against this endpoint; there is no live API server.

Unit tests cover control-plane CRUD without Docker. SDK Go/Node/Python rows soft-skip when the API is down (`NOCTAXRIS_SKIP_IF_DOWN=1`). Terraform: `STACK=lab-parity-compute` (`TF_PARITY=1` / `NOCTAXRIS_ADVANCED=1`).

## Not yet / deferred

- Nested `rancher/k3s` (or equivalent) inside DinD
- Live kubectl / IAM authenticator webhook / CA extraction
- CreateNodegroup / DescribeNodegroup / DeleteNodegroup (beyond empty ListNodegroups)
- Fargate profiles, add-ons, access entries, UpdateClusterConfig / UpdateClusterVersion
- Host or WAN publish of Kubernetes API ports (forbidden)
