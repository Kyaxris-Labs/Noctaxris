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
