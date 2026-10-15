# Every secret the API needs at runtime (config.go's mustEnv/os.Getenv
# calls) lives in Secrets Manager, never in a .tf file, tfvars, or the
# task definition's plain "environment" block — only as "secrets" entries
# resolved by ECS at container start (see ecs.tf).

resource "random_password" "jwt_access_secret" {
  length  = 64
  special = false
}

resource "random_password" "jwt_refresh_secret" {
  length  = 64
  special = false
}

resource "aws_secretsmanager_secret" "jwt_access" {
  name = "chatplatform/${var.environment}/jwt_access_secret"
}
resource "aws_secretsmanager_secret_version" "jwt_access" {
  secret_id     = aws_secretsmanager_secret.jwt_access.id
  secret_string = random_password.jwt_access_secret.result
}

resource "aws_secretsmanager_secret" "jwt_refresh" {
  name = "chatplatform/${var.environment}/jwt_refresh_secret"
}
resource "aws_secretsmanager_secret_version" "jwt_refresh" {
  secret_id     = aws_secretsmanager_secret.jwt_refresh.id
  secret_string = random_password.jwt_refresh_secret.result
}

resource "aws_secretsmanager_secret" "db_password" {
  name = "chatplatform/${var.environment}/db_password"
}
resource "aws_secretsmanager_secret_version" "db_password" {
  secret_id     = aws_secretsmanager_secret.db_password.id
  secret_string = random_password.db.result
}

# Deliberately NOT pre-populated with a real value, ever, by Terraform.
# The disposable test key used throughout this project's local
# development (see backend/.env, gitignored, never committed, never put
# in .env.example or any source file — backend/README.md's Phase 9
# "Known gaps" section has the full history) stays out of this file and
# out of any real deployment. Set the real value once, out of band,
# with `aws secretsmanager put-secret-value`, by whoever deploys this —
# never by committing it anywhere this repository's history can see.
resource "aws_secretsmanager_secret" "gemini_api_key" {
  name = "chatplatform/${var.environment}/gemini_api_key"
}
resource "aws_secretsmanager_secret_version" "gemini_api_key" {
  secret_id     = aws_secretsmanager_secret.gemini_api_key.id
  secret_string = "SET-ME-MANUALLY-NEVER-COMMIT-A-REAL-KEY-HERE"
  lifecycle {
    ignore_changes = [secret_string] # so a later out-of-band `put-secret-value` isn't clobbered back to the placeholder on the next apply
  }
}
