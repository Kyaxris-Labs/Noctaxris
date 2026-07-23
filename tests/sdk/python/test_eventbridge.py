def test_eventbridge_bus_and_rule_round_trip(events_client, unique_prefix):
    bus_name = f"{unique_prefix}-bus"
    rule_name = f"{unique_prefix}-rule"

    events_client.create_event_bus(Name=bus_name)
    try:
        events_client.put_rule(
            Name=rule_name,
            EventBusName=bus_name,
            EventPattern='{"source":["noctaxris.sdk"]}',
            State="ENABLED",
        )
        try:
            desc = events_client.describe_rule(Name=rule_name, EventBusName=bus_name)
            assert desc["Name"] == rule_name

            events_client.delete_rule(Name=rule_name, EventBusName=bus_name)
        finally:
            try:
                events_client.delete_rule(Name=rule_name, EventBusName=bus_name)
            except Exception:
                pass

        events_client.delete_event_bus(Name=bus_name)
    finally:
        try:
            events_client.delete_event_bus(Name=bus_name)
        except Exception:
            pass
