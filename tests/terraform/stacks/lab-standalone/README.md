# lab-standalone

Terraform stack for one S3 bucket in a Noctaxris standalone account. It is not the default stack. `tests/run-all.sh` does not call it.

`tests/terraform/run.sh` defaults `STACK` to `lab-core`. Set `STACK=lab-standalone` to run this stack. If `AWS_ACCESS_KEY_ID` or `AWS_SECRET_ACCESS_KEY` is still the example root pair, the script skips and does not apply.

The `.tf` files do not contain access keys. The AWS provider reads `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` from the environment. Those values are the standalone account key: an IAM user key (`is_root` false) returned by `POST /_noctaxris/lab/accounts:seedStandalone` while `NOCTAXRIS_LAB_STANDALONE` is `1` or `true`. That key is not `NOCTAXRIS_ROOT_ACCESS_KEY_ID` on management account `000000000001`. See `docs/configuration.md`.

`bucket_name` defaults to `standalone-lab-bucket`. The bucket argument is `lower(var.bucket_name)`. `endpoint` defaults to `http://127.0.0.1:4566`.
