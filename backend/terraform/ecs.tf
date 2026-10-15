resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/chatplatform-${var.environment}-api"
  retention_in_days = 30
}

resource "aws_ecs_cluster" "main" {
  name = "chatplatform-${var.environment}"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_ecs_task_definition" "api" {
  family                   = "chatplatform-${var.environment}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.api_task_cpu
  memory                   = var.api_task_memory
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name      = "api"
    image     = var.api_image
    essential = true

    portMappings = [
      { containerPort = 8080, protocol = "tcp" }, # public API — see alb.tf's target group
      { containerPort = 9090, protocol = "tcp" }, # /metrics — never attached to the ALB, see security_groups.tf's comment
    ]

    environment = [
      { name = "APP_ENV", value = var.environment == "production" ? "production" : "staging" },
      { name = "HTTP_PORT", value = "8080" },
      { name = "METRICS_PORT", value = "9090" },
      { name = "POSTGRES_DSN", value = "postgres://${var.db_username}:${random_password.db.result}@${aws_db_instance.main.endpoint}/${var.db_name}?sslmode=require" },
      { name = "REDIS_ADDR", value = "${aws_elasticache_replication_group.main.primary_endpoint_address}:6379" },
      { name = "REDIS_TLS_ENABLED", value = "true" },
      { name = "S3_BUCKET", value = aws_s3_bucket.media.bucket },
      { name = "S3_REGION", value = var.aws_region },
      { name = "GEMINI_MODEL", value = "gemini-2.0-flash" },
    ]

    # Secrets Manager -> environment variable, resolved by ECS at
    # container start, never written into the task definition's plain
    # "environment" block above (that block is a bare JSON string
    # readable by anyone with ecs:DescribeTaskDefinition).
    secrets = [
      { name = "JWT_ACCESS_SECRET", valueFrom = aws_secretsmanager_secret.jwt_access.arn },
      { name = "JWT_REFRESH_SECRET", valueFrom = aws_secretsmanager_secret.jwt_refresh.arn },
      { name = "GEMINI_API_KEY", valueFrom = aws_secretsmanager_secret.gemini_api_key.arn },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.api.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "api"
      }
    }

    healthCheck = {
      command     = ["CMD-SHELL", "wget -q -O- http://localhost:8080/healthz || exit 1"]
      interval    = 15
      timeout     = 5
      retries     = 3
      startPeriod = 20
    }
  }])

  # POSTGRES_DSN above is built from random_password.db.result directly
  # in the task definition rather than via a "secrets" entry — this is a
  # deliberate trade-off, not an oversight: ECS "secrets" entries can
  # only inject a *whole* secret value as one environment variable, and
  # this DSN interpolates the password into a larger connection string.
  # The practical effect is the same exposure as any other
  # DescribeTaskDefinition-readable value. Tightening this to build the
  # DSN inside the container from separate DB_HOST/DB_PASSWORD secrets
  # (requiring a small config.go change) is a reasonable follow-up, not
  # done here to avoid changing config.go's parsing during a phase whose
  # actual scope is deployment, not another backend code change beyond
  # the redisx TLS fix above.
}

resource "aws_ecs_service" "api" {
  name            = "chatplatform-${var.environment}-api"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.api.arn
  launch_type     = "FARGATE"
  desired_count   = var.api_desired_count

  network_configuration {
    subnets         = aws_subnet.private[*].id
    security_groups = [aws_security_group.api.id]
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  # A rolling deploy with minimum healthy 100% / max 200% — never fewer
  # than desired_count tasks serving, so a deploy itself can't look like
  # an unplanned failover. The *drill* in ../RUNBOOK.md step 7 is what
  # intentionally kills a task to test the real failover path; a normal
  # deploy should never need to.
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  depends_on = [aws_lb_listener.https, aws_lb_listener.http_plain]

  lifecycle {
    ignore_changes = [desired_count] # so a future manual/autoscaled scale-out isn't reverted by the next apply
  }
}
