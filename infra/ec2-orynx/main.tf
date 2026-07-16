# Orynx self-host EC2 — one isolated, always-on box that runs the whole stack
# (Postgres + backend + frontend) AND is the agent execution runtime (Claude
# Code + Codex + Chrome). Nothing on your laptop is exposed.
#
#   terraform init
#   terraform apply
#   # then: ../deploy.sh   (syncs source, builds, migrates, starts daemon)

terraform {
  required_version = ">= 1.3"
  required_providers {
    aws  = { source = "hashicorp/aws", version = "~> 5.0" }
    tls  = { source = "hashicorp/tls", version = "~> 4.0" }
    http = { source = "hashicorp/http", version = "~> 3.0" }
    local = { source = "hashicorp/local", version = "~> 2.0" }
  }
}

provider "aws" {
  region = var.region
}

# Restrict every ingress rule to the IP running terraform (your Mac), so the box
# is never open to the world.
data "http" "my_ip" {
  url = "https://ipv4.icanhazip.com"
}

locals {
  my_cidr = "${chomp(data.http.my_ip.response_body)}/32"
}

# Latest Ubuntu 22.04 LTS (amd64), from Canonical's published SSM parameter.
data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/22.04/stable/current/amd64/hvm/ebs-gp2/ami-id"
}

# Generate an SSH keypair; private key is written next to the terraform state.
resource "tls_private_key" "orynx" {
  algorithm = "ED25519"
}

resource "aws_key_pair" "orynx" {
  key_name   = "orynx-runtime"
  public_key = tls_private_key.orynx.public_key_openssh
}

resource "local_sensitive_file" "private_key" {
  content         = tls_private_key.orynx.private_key_openssh
  filename        = "${path.module}/orynx-runtime.pem"
  file_permission = "0600"
}

resource "aws_security_group" "orynx" {
  name        = "orynx-runtime"
  description = "Orynx self-host box - access restricted to the operator IP"

  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = distinct(concat([local.my_cidr], var.extra_operator_cidrs))
  }
  ingress {
    description = "Web (Caddy reverse proxy to frontend)"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = distinct(concat([local.my_cidr], var.extra_operator_cidrs))
  }
  egress {
    description = "All outbound (pulls, agent API calls, git)"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "orynx-runtime" }
}

resource "aws_instance" "orynx" {
  ami                    = data.aws_ssm_parameter.ubuntu.value
  instance_type          = var.instance_type
  key_name               = aws_key_pair.orynx.key_name
  vpc_security_group_ids = [aws_security_group.orynx.id]
  user_data              = file("${path.module}/bootstrap.sh")

  root_block_device {
    volume_size = var.disk_gb
    volume_type = "gp3"
  }

  tags = { Name = "orynx-runtime" }
}

output "public_ip" {
  value = aws_instance.orynx.public_ip
}

output "ssh" {
  value = "ssh -i ${path.module}/orynx-runtime.pem ubuntu@${aws_instance.orynx.public_ip}"
}

output "web_url" {
  value = "http://${aws_instance.orynx.public_ip}  (only reachable from your current IP)"
}
