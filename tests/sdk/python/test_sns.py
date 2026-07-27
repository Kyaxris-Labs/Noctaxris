import os

import pytest
from botocore.exceptions import ClientError


def test_sns_topic_publish_round_trip(sns_client, unique_prefix):
    name = f"{unique_prefix}-topic"
    created = sns_client.create_topic(Name=name)
    topic_arn = created["TopicArn"]
    assert topic_arn

    try:
        topics = [t["TopicArn"] for t in sns_client.list_topics().get("Topics", [])]
        assert topic_arn in topics, f"ListTopics missing {topic_arn}"

        pub = sns_client.publish(TopicArn=topic_arn, Message="sdk-sns")
        assert pub.get("MessageId"), "Publish missing MessageId"

        sns_client.delete_topic(TopicArn=topic_arn)
    finally:
        try:
            sns_client.delete_topic(TopicArn=topic_arn)
        except Exception:
            pass


@pytest.mark.skipif(
    os.environ.get("NOCTAXRIS_SNS_HTTP_EGRESS") == "1",
    reason="NOCTAXRIS_SNS_HTTP_EGRESS=1; deny-by-default assertion soft-skipped",
)
def test_sns_http_egress_deny_without_opt_in(sns_client, unique_prefix):
    name = f"{unique_prefix}-http-deny"
    created = sns_client.create_topic(Name=name)
    topic_arn = created["TopicArn"]
    assert topic_arn

    try:
        with pytest.raises(ClientError) as exc:
            sns_client.subscribe(
                TopicArn=topic_arn,
                Protocol="https",
                Endpoint="https://example.com/noctaxris-sns-deny",
            )
        code = exc.value.response.get("Error", {}).get("Code", "")
        assert code in ("InvalidParameter", "InvalidParameterException"), exc.value
    finally:
        try:
            sns_client.delete_topic(TopicArn=topic_arn)
        except Exception:
            pass
