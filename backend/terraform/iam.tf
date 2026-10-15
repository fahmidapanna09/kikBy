# The execution role: what ECS itself needs to start the container
# (pull from ECR, write logs, resolve the "secrets" block in ecs.tf's
# task definition). Not the same as the task role below — AWS's
# execution-role/task-role split exists specifically so the running
# application's own AWS permissions (task role) are never broader than
# "needed to boot the container" (execution role).
resource "aws_iam_role" "ecs_execution" {
  name = "chatplatform-${var.environment}-ecs-execution"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_execution_managed" {
  role       = aws_iam_role.ecs_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "ecs_execution_secrets" {
  name = "read-secrets"
  role = aws_iam_role.ecs_execution.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = ["secretsmanager:GetSecretValue"]
      Resource = [
        aws_secretsmanager_secret.jwt_access.arn,
        aws_secretsmanager_secret.jwt_refresh.arn,
        aws_secretsmanager_secret.db_password.arn,
        aws_secretsmanager_secret.gemini_api_key.arn,
      ]
    }]
  })
}

# The task role: what the Go application itself can do with AWS, via
# its own credentials at runtime (the S3 presign calls in
# internal/media/s3.go). Deliberately scoped to exactly one bucket,
# never "s3:*" on "*".
resource "aws_iam_role" "ecs_task" {
  name = "chatplatform-${var.environment}-ecs-task"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy" "ecs_task_s3" {
  name = "media-bucket-access"
  role = aws_iam_role.ecs_task.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"]
      Resource = ["${aws_s3_bucket.media.arn}/*"]
    }, {
      Effect   = "Allow"
      Action   = ["s3:ListBucket"]
      Resource = [aws_s3_bucket.media.arn]
    }]
  })
}
