resource "aws_s3_bucket" "standalone" {
  bucket        = lower(var.bucket_name)
  force_destroy = true
}
