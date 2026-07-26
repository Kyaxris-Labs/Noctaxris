variable "live" {
  type        = bool
  description = "When true, DesiredCount uses desired_count (needs DinD). When false, DesiredCount is 0."
  default     = false
}

variable "desired_count" {
  type        = number
  description = "Service DesiredCount when live=true"
  default     = 1
}

variable "container_image" {
  type        = string
  description = "Task container image. Empty uses alpine:3.20 unless use_ecr_image=true."
  default     = ""
}

variable "use_ecr_image" {
  type        = bool
  description = "When true and container_image is empty, use lab ECR URI (push image first for live runs)."
  default     = false
}
