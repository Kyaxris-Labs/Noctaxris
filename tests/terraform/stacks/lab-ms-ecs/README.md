# lab-ms-ecs live notes

Control-plane (`live=false`): apply creates ECR, IAM roles, task definition, and a service with DesiredCount 0. No nested DinD required.

Live (`live=true`): DesiredCount defaults to 1. The default container image is `alpine:3.20` (DinD pull). Nested tasks need a healthy `noctaxris-engine`.

Optional lab ECR image path:

```bash
# After apply with live=false (or create repo first), push then re-apply with live=true:
STACK=lab-ms-ecs bash tests/terraform/run.sh   # or apply once and note repository_name
bash tests/terraform/stacks/lab-ms-ecs/scripts/push-lab-image.sh \
  "$NOCTAXRIS_ENDPOINT" "<repository_name>"
STACK=lab-ms-ecs TF_MS_LIVE=1 \
  TF_VAR_use_ecr_image=true \
  bash tests/terraform/run.sh
```

For task→API calls from the nested container, opt in to ECS host-gateway:

- `NOCTAXRIS_INJECT_ECS_HOST_GATEWAY=1`, or
- `docker compose -f docker/compose.yaml -f docker/compose.lab-ecs-host-gateway.yaml ...`

Do not widen Compose publish to `0.0.0.0` for WSL.
