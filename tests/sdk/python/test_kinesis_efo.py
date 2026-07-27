def test_kinesis_efo_and_update_shard_count(kinesis_client, unique_prefix):
    stream = f"{unique_prefix}-kinesis"
    kinesis_client.create_stream(StreamName=stream, ShardCount=1)
    try:
        desc = kinesis_client.describe_stream(StreamName=stream)
        stream_arn = desc["StreamDescription"]["StreamARN"]

        reg = kinesis_client.register_stream_consumer(
            StreamARN=stream_arn,
            ConsumerName="lab-consumer",
        )
        assert reg["Consumer"]["ConsumerARN"]

        kinesis_client.update_shard_count(
            StreamName=stream,
            TargetShardCount=2,
            ScalingType="UNIFORM_SCALING",
        )
        after = kinesis_client.describe_stream(StreamName=stream)
        assert len(after["StreamDescription"]["Shards"]) == 2

        kinesis_client.deregister_stream_consumer(
            ConsumerARN=reg["Consumer"]["ConsumerARN"],
        )
    finally:
        try:
            kinesis_client.delete_stream(StreamName=stream)
        except Exception:
            pass
