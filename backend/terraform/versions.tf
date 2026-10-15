# Phase 12 — AWS infrastructure for the chat platform, ap-southeast-1
# (Singapore), per the architecture doc's roadmap exit criteria for this
# phase ("IaC applied").
#
# HONEST STATUS: this has been written and reviewed, but never
# `terraform init`'d, `plan`'d, or `apply`'d anywhere. The sandbox this
# project was built in has neither a `terraform` binary nor AWS
# credentials, and its network egress policy would likely block
# registry.terraform.io / releases.hashicorp.com the same way it already
# blocks proxy.golang.org (see internal/platform/metrics's package doc
# and ../loadtest/REPORT.md for that same constraint hit elsewhere in
# Phase 11/12). "IaC applied" in the literal sense has NOT happened —
# this is the deliverable for whoever runs `terraform apply` with real
# AWS credentials. See ../RUNBOOK.md for that procedure end to end.

terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # Uncomment and point at a real bucket+table before the first real
  # `terraform apply` — local state is fine for nothing, including a
  # first solo trial run, but the moment a second person or CI runs this,
  # local state will silently diverge.
  # backend "s3" {
  #   bucket         = "chatplatform-terraform-state"
  #   key            = "phase12/terraform.tfstate"
  #   region         = "ap-southeast-1"
  #   dynamodb_table = "chatplatform-terraform-locks"
  #   encrypt        = true
  # }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "chatplatform"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}
