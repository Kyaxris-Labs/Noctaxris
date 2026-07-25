"""AppConfig StartDeployment + GetConfiguration (lab JSON targets)."""

from __future__ import annotations

import base64

from conftest import json_target


def test_appconfig_start_deployment_get_configuration(unique_prefix):
    content = b'{"feature":true,"src":"py"}'
    app = json_target(
        "AmazonAppConfig.CreateApplication",
        "appconfig",
        {"Name": f"{unique_prefix}-app"},
    )
    app_id = app["Id"]
    env = json_target(
        "AmazonAppConfig.CreateEnvironment",
        "appconfig",
        {"ApplicationId": app_id, "Name": "dev"},
    )
    env_id = env["Id"]
    profile = json_target(
        "AmazonAppConfig.CreateConfigurationProfile",
        "appconfig",
        {
            "ApplicationId": app_id,
            "Name": "flags",
            "LocationUri": "hosted",
        },
    )
    profile_id = profile["Id"]

    hosted = json_target(
        "AmazonAppConfig.CreateHostedConfigurationVersion",
        "appconfig",
        {
            "ApplicationId": app_id,
            "ConfigurationProfileId": profile_id,
            "ContentType": "application/json",
            "Content": base64.b64encode(content).decode("ascii"),
        },
    )
    version = hosted["VersionNumber"]

    dep = json_target(
        "AmazonAppConfig.StartDeployment",
        "appconfig",
        {
            "ApplicationId": app_id,
            "EnvironmentId": env_id,
            "ConfigurationProfileId": profile_id,
            "ConfigurationVersion": str(version),
            "DeploymentStrategyId": "AppConfig.AllAtOnce",
        },
    )
    assert dep.get("State") == "DEPLOYED" or dep.get("Id")

    got = json_target(
        "AmazonAppConfig.GetConfiguration",
        "appconfig",
        {
            "Application": app_id,
            "Environment": env_id,
            "Configuration": profile_id,
            "ClientId": "py-lab",
        },
    )
    # Lab may return Content as base64 or embed the hosted bytes.
    if "Content" in got:
        decoded = base64.b64decode(got["Content"])
        assert decoded == content
    else:
        assert content.decode("utf-8") in str(got)
