# DynamoDB table with hash+range and one LSI (Projection ALL).
# CloudWatch metric alarm omitted: AWS provider uses Query protocol for
# aws_cloudwatch_metric_alarm; Noctaxris monitoring is Granite JSON only.
# Use SDK smoke (tests/sdk) for PutMetricAlarm / DescribeAlarms.

resource "aws_dynamodb_table" "lab" {
  name         = "${var.name_prefix}-observe"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "pk"
  range_key    = "sk"

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  attribute {
    name = "status"
    type = "S"
  }

  local_secondary_index {
    name            = "ByStatus"
    range_key       = "status"
    projection_type = "ALL"
  }
}
