<!-- Short description (max 100 chars):
Run AWS-shaped security labs without a cloud bill or a host Docker socket.
-->

<p align="center">
  <img src="https://raw.githubusercontent.com/Kyaxris-Labs/Noctaxris/main/assets/noctaxris_bg.png" alt="Noctaxris" width="640">
</p>

<p align="center">
  <b>Run AWS-shaped security labs on your laptop without a cloud bill or a host Docker socket.</b>
</p>

```bash
docker pull kyaxris/noctaxris:latest
# Generate unique roots (shipped example pair is refused).
ROOT_AKID="AKIA$(openssl rand -hex 8 | tr '[:lower:]' '[:upper:]')"
ROOT_SECRET="$(openssl rand -hex 32)"
docker run -d --name noctaxris -p 127.0.0.1:4566:4566 \
  -e NOCTAXRIS_LISTEN=0.0.0.0:4566 \
  -e NOCTAXRIS_ALLOW_NONLOOPBACK_LISTEN=1 \
  -e NOCTAXRIS_ROOT_ACCESS_KEY_ID="$ROOT_AKID" \
  -e NOCTAXRIS_ROOT_SECRET_ACCESS_KEY="$ROOT_SECRET" \
  kyaxris/noctaxris:latest
curl http://127.0.0.1:4566/_noctaxris/health
```

Point the AWS CLI or SDK at `http://127.0.0.1:4566`. Tags: `latest`, semver, `nightly`.

Full service matrix and docs: [github.com/Kyaxris-Labs/Noctaxris](https://github.com/Kyaxris-Labs/Noctaxris).

License: [MIT](https://github.com/Kyaxris-Labs/Noctaxris/blob/main/LICENSE)
