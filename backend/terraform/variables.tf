variable "aws_region" {
  description = "AWS region — ap-southeast-1 (Singapore) per the architecture doc's deployment target."
  type        = string
  default     = "ap-southeast-1"
}

variable "environment" {
  description = "Deployment environment name, used in resource names/tags."
  type        = string
  default     = "staging"
}

variable "vpc_cidr" {
  type    = string
  default = "10.42.0.0/16"
}

variable "az_count" {
  description = "Number of availability zones to spread subnets across. 2 is the minimum for an ALB + Multi-AZ RDS."
  type        = number
  default     = 2
}

variable "api_image" {
  description = "Full ECR image URI for the Go API (built from backend/Dockerfile). Set by CI after a real build — this sandbox never produced one (see ../RUNBOOK.md step 3; go build has not succeeded here even once)."
  type        = string
  default     = "" # intentionally empty — ecs.tf's task definition is not usable until this is set
}

variable "api_task_cpu" {
  type    = number
  default = 512 # 0.5 vCPU — raise for the real load-test target once Phase 12's k6 suite has actually run (see loadtest/)
}

variable "api_task_memory" {
  type    = number
  default = 1024
}

variable "api_desired_count" {
  description = "Number of API tasks behind the ALB. Must be >= 2 for the roadmap's 'WS survives LB failover' exit criterion to even be testable — a single task has nothing to fail over to."
  type        = number
  default     = 2
}

variable "db_instance_class" {
  type    = string
  default = "db.t4g.micro" # cheapest Graviton burstable class; resize once loadtest/REPORT.md's numbers are reproduced against this real instance class, not the sandbox's local Postgres
}

variable "db_name" {
  type    = string
  default = "chatplatform"
}

variable "db_username" {
  type    = string
  default = "chatplatform_app"
}

variable "redis_node_type" {
  type    = string
  default = "cache.t4g.micro"
}

variable "domain_name" {
  description = "Public domain the API will be served on, e.g. api.example.com. Leave empty to skip ACM/Route53 (route53.tf resources become no-ops) and use the ALB's own DNS name instead — fine for a first load-test deployment, not for anything real users see."
  type        = string
  default     = ""
}

variable "route53_zone_id" {
  description = "Existing Route53 hosted zone ID for domain_name, if domain_name is set and the zone already exists outside this stack."
  type        = string
  default     = ""
}

variable "media_bucket_name" {
  description = "S3 bucket name for user media (must be globally unique) — matches the S3_BUCKET env var in backend/.env.example."
  type        = string
  default     = "" # required if set empty, a name is generated in s3.tf from environment + a random suffix
}
