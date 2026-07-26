# syntax=docker/dockerfile:1
# Tool-bearing CodeBuild lab image for Noctaxris nested builds.
# Runtime pulls must use the lab registry (127.0.0.1:4566/ACCOUNT/codebuild-lab:tag),
# rewritten for DinD as host.docker.internal:<listenPort>/ACCOUNT/codebuild-lab:tag.
# Do not point CodeBuild environment.image at arbitrary Docker Hub tags.
FROM alpine:3.20

RUN apk add --no-cache \
      ca-certificates \
      curl \
      jq \
      python3 \
      py3-pip \
  && pip3 install --no-cache-dir --break-system-packages \
      'boto3>=1.34,<2' \
      'awscli>=1.32,<2' \
  && addgroup -S -g 1000 codebuild \
  && adduser -S -u 1000 -G codebuild -h /home/codebuild -s /bin/sh codebuild \
  && mkdir -p /home/codebuild \
  && chown -R codebuild:codebuild /home/codebuild

USER codebuild
WORKDIR /home/codebuild

# Nested StartBuild invokes: /bin/sh -c "<buildspec commands>"
CMD ["/bin/sh"]
