output "alb_dns_name" {
  description = "ALB's own DNS name — usable directly (plain HTTP) when var.domain_name is unset, e.g. for a first k6 load-test pass. See ../RUNBOOK.md step 6."
  value       = aws_lb.main.dns_name
}

output "api_url" {
  description = "The URL to actually point clients (and k6's BASE_URL) at."
  value       = var.domain_name != "" ? "https://${var.domain_name}" : "http://${aws_lb.main.dns_name}"
}

output "ecr_repository_url" {
  value = aws_ecr_repository.api.repository_url
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  value = aws_ecs_service.api.name
}

output "db_endpoint" {
  value     = aws_db_instance.main.endpoint
  sensitive = true
}

output "redis_endpoint" {
  value     = aws_elasticache_replication_group.main.primary_endpoint_address
  sensitive = true
}

output "media_bucket_name" {
  value = aws_s3_bucket.media.bucket
}

output "cloudwatch_log_group" {
  value = aws_cloudwatch_log_group.api.name
}
