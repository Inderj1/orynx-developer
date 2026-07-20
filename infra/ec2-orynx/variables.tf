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
  description = "Root volume size. Docker images + node_modules + agent workdirs need headroom. Expanded 40->80 on 2026-07-17 (live modify-volume + growpart/resize2fs) after the 40GB box hit 100% mid-build; keep >=80 for dockerized-app building."
  type        = number
  default     = 80
}

variable "extra_operator_cidrs" {
  description = "Additional operator IPs (besides the auto-detected current IP) allowed on SSH + web. The operator works from two networks; keep both so an ISP IP reassignment on one doesn't lock the box."
  type        = list(string)
  default     = ["47.233.56.109/32", "142.136.62.204/32"]
}
