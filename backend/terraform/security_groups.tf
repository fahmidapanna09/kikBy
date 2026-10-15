resource "aws_security_group" "alb" {
  name_prefix = "chatplatform-${var.environment}-alb-"
  vpc_id      = aws_vpc.main.id
  description = "Public ALB — HTTPS in, nothing else."

  ingress {
    description = "HTTPS from the internet"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    description = "HTTP from the internet, redirected to HTTPS by the listener rule below — never forwarded to a target as plaintext"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "chatplatform-${var.environment}-alb-sg" }
}

resource "aws_security_group" "api" {
  name_prefix = "chatplatform-${var.environment}-api-"
  vpc_id      = aws_vpc.main.id
  description = "API ECS tasks — only reachable from the ALB, never directly from the internet."

  ingress {
    description     = "HTTP_PORT (8080) from the ALB only"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "chatplatform-${var.environment}-api-sg" }
}

# /metrics intentionally has NO separate ingress rule from anywhere
# outside the VPC — see internal/platform/metrics's package doc and
# backend/README.md's Phase 11 section: METRICS_PORT is a second port on
# the *same* task, and this security group's lack of an explicit
# 9090 ingress rule (beyond what's implied by scraping from inside the
# VPC — add a narrow rule scoped to a Prometheus/OTel collector's own
# security group if one is deployed) is deliberate, not an oversight.

resource "aws_security_group" "rds" {
  name_prefix = "chatplatform-${var.environment}-rds-"
  vpc_id      = aws_vpc.main.id
  description = "RDS Postgres — only reachable from API tasks."

  ingress {
    description     = "Postgres from API tasks only"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.api.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "chatplatform-${var.environment}-rds-sg" }
}

resource "aws_security_group" "redis" {
  name_prefix = "chatplatform-${var.environment}-redis-"
  vpc_id      = aws_vpc.main.id
  description = "ElastiCache Redis — only reachable from API tasks."

  ingress {
    description     = "Redis from API tasks only"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.api.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "chatplatform-${var.environment}-redis-sg" }
}
