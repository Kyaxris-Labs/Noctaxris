"""CUR report definitions and IoT thing/cert/shadow lab APIs."""

from __future__ import annotations

from conftest import json_target


def test_cur_put_describe_delete(unique_prefix):
    report_name = f"sdk-{unique_prefix}"
    put = json_target(
        "AWSOrigamiServiceGatewayService.PutReportDefinition",
        "cur",
        {
            "ReportDefinition": {
                "ReportName": report_name,
                "TimeUnit": "MONTHLY",
                "Format": "textORcsv",
                "Compression": "GZIP",
                "S3Bucket": f"cur-sdk-{unique_prefix}",
                "S3Prefix": "reports",
                "S3Region": "us-east-1",
                "AdditionalSchemaElements": ["RESOURCES"],
                "ReportVersioning": "OVERWRITE_REPORT",
            }
        },
    )
    assert put.get("ReportName"), f"PutReportDefinition missing ReportName: {put}"

    desc = json_target(
        "AWSOrigamiServiceGatewayService.DescribeReportDefinitions",
        "cur",
        {},
    )
    assert desc.get("ReportDefinitions"), f"DescribeReportDefinitions empty: {desc}"

    json_target(
        "AWSOrigamiServiceGatewayService.DeleteReportDefinition",
        "cur",
        {"ReportName": report_name},
    )


def test_iot_thing_cert_shadow(unique_prefix):
    thing_name = f"sdk-iot-{unique_prefix}"
    json_target(
        "AWSIotService.CreateThing",
        "iot",
        {
            "thingName": thing_name,
            "attributePayload": {"attributes": {"env": "sdk"}},
        },
    )

    cert = json_target(
        "AWSIotService.CreateKeysAndCertificate",
        "iot",
        {"setAsActive": True},
    )
    cert_arn = cert.get("certificateArn")
    assert cert_arn, f"missing certificateArn: {cert}"

    json_target(
        "AWSIotService.CreatePolicy",
        "iot",
        {
            "policyName": f"{thing_name}-pol",
            "policyDocument": (
                '{"Version":"2012-10-17","Statement":[{"Effect":"Allow",'
                '"Action":"iot:*","Resource":"*"}]}'
            ),
        },
    )

    json_target(
        "AWSIotService.AttachThingPrincipal",
        "iot",
        {"thingName": thing_name, "principal": cert_arn},
    )

    json_target(
        "AWSIotDataService.UpdateThingShadow",
        "iot-data",
        {
            "thingName": thing_name,
            "state": {"desired": {"color": "green"}},
        },
    )

    shadow = json_target(
        "AWSIotDataService.GetThingShadow",
        "iot-data",
        {"thingName": thing_name},
    )
    assert shadow.get("state"), f"GetThingShadow missing state: {shadow}"
