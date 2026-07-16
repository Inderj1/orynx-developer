variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "instance_type" {
  description = "EC2 instance type. t3.large (8GB) builds the stack with the swapfile the bootstrap adds; t3.xlarge (16GB) is safer for heavy parallel agent work."
  type        = string
  default     = "t3.large"
}

variable "disk_gb" {
  description = "Root volume size. Docker images + node_modules + agent workdirs need headroom."
  type        = number
  default     = 40
}
