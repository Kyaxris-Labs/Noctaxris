"""CodeBuild control-plane + light CodeCommit via SigV4 JSON; StartBuild nested-gated."""

from __future__ import annotations

import json

import pytest
from conftest import endpoint, json_target, nested_enabled, signed_request

CODEBUILD_TRUST = (
    '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
    '"Principal":{"Service":"codebuild.amazonaws.com"},'
    '"Action":"sts:AssumeRole"}]}'
)


def _json_target_status(
    target: str, service: str, payload: dict | None = None
) -> tuple[int, bytes, dict]:
    """Like json_target but returns status for nested StartBuild gating."""
    body = json.dumps(payload or {}).encode("utf-8")
    status, raw, _ = signed_request(
        "POST",
        endpoint() + "/",
        service=service,
        body=body,
        headers={"X-Amz-Target": target},
        content_type="application/x-amz-json-1.1",
    )
    parsed: dict = {}
    if raw:
        parsed = json.loads(raw.decode("utf-8"))
    return status, raw, parsed


def test_codebuild_control_plane_and_optional_start_build(iam_client, unique_prefix):
    role_name = f"{unique_prefix}-cb-role"
    if len(role_name) > 64:
        role_name = role_name[:64]
    role_out = iam_client.create_role(
        RoleName=role_name,
        AssumeRolePolicyDocument=CODEBUILD_TRUST,
    )
    role_arn = role_out["Role"]["Arn"]
    assert role_arn

    repo_name = f"cb-src-{unique_prefix}".replace("_", "-")
    if len(repo_name) > 100:
        repo_name = repo_name[:100]
    project_name = f"cb-proj-{unique_prefix}".replace("_", "-")
    if len(project_name) > 100:
        project_name = project_name[:100]

    try:
        repo = json_target(
            "CodeCommit_20150413.CreateRepository",
            "codecommit",
            {"repositoryName": repo_name},
        )
        assert repo.get("repositoryMetadata") or repo

        put = json_target(
            "CodeCommit_20150413.PutFile",
            "codecommit",
            {
                "repositoryName": repo_name,
                "branchName": "main",
                "filePath": "README.md",
                "fileContent": "hello-codecommit",
            },
        )
        assert put.get("commitId")

        created = json_target(
            "CodeBuild_20161006.CreateProject",
            "codebuild",
            {
                "name": project_name,
                "serviceRole": role_arn,
                "source": {
                    "type": "CODECOMMIT",
                    "location": repo_name,
                    "buildspec": (
                        '{"version":"0.2","phases":{"build":'
                        '{"commands":["cat README.md"]}}}'
                    ),
                },
                "environment": {
                    "type": "LINUX_CONTAINER",
                    "image": "alpine:3.20",
                },
                "artifacts": {"type": "NO_ARTIFACTS"},
            },
        )
        assert created["project"]["name"] == project_name

        listed = json_target("CodeBuild_20161006.ListProjects", "codebuild", {})
        assert project_name in listed.get("projects", [])

        batch = json_target(
            "CodeBuild_20161006.BatchGetProjects",
            "codebuild",
            {"names": [project_name]},
        )
        projects = batch.get("projects") or []
        assert len(projects) == 1
        assert projects[0]["source"]["type"] == "CODECOMMIT"
        assert projects[0]["source"]["location"] == repo_name

        wh = json_target(
            "CodeBuild_20161006.CreateWebhook",
            "codebuild",
            {
                "projectName": project_name,
                "filterGroups": [[{"type": "EVENT", "pattern": "PUSH"}]],
            },
        )
        assert wh.get("webhook", {}).get("payloadUrl")
        assert wh.get("webhook", {}).get("secret")
        list_wh = json_target(
            "CodeBuild_20161006.ListWebhooks",
            "codebuild",
            {"projectName": project_name},
        )
        assert len(list_wh.get("webhooks") or []) == 1
        json_target(
            "CodeBuild_20161006.DeleteWebhook",
            "codebuild",
            {"projectName": project_name},
        )

        if not nested_enabled():
            return

        status, raw, start = _json_target_status(
            "CodeBuild_20161006.StartBuild",
            "codebuild",
            {"projectName": project_name},
        )
        if status == 503 and b"compute unavailable" in raw:
            pytest.skip(
                "CodeBuild StartBuild skipped: compute unavailable "
                "(nested engine not healthy)"
            )
        assert status == 200, start
        assert start.get("build", {}).get("id")
    finally:
        try:
            json_target(
                "CodeBuild_20161006.DeleteProject",
                "codebuild",
                {"name": project_name},
            )
        except AssertionError:
            pass
        try:
            json_target(
                "CodeCommit_20150413.DeleteRepository",
                "codecommit",
                {"repositoryName": repo_name},
            )
        except AssertionError:
            pass
        try:
            iam_client.delete_role(RoleName=role_name)
        except Exception:
            pass
