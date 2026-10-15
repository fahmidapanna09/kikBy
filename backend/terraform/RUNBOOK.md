# Phase 12 deployment runbook

**Nothing in this file has been run in this project's sandbox** — no
AWS credentials, no `terraform` binary, no reachable registry (see
`versions.tf`'s header comment and `../loadtest/REPORT.md`). This is the
step-by-step for whoever runs it for real, against a real AWS account.

## 0. Prerequisites
- An AWS account, and credentials for it (`aws configure` or an assumed
  role) with permission to create VPC/RDS/ElastiCache/ECS/ALB/IAM/ACM/
  Route53/Secrets Manager resources.
- Terraform >= 1.5.
- Docker, to build the API image.
- (Optional, recommended before the first real apply) an S3 bucket +
  DynamoDB table for remote state — uncomment the `backend "s3"` block in
  `versions.tf` and fill in the names.

## 1. Configure
```bash
cd backend/terraform
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars — at minimum decide domain_name (or leave empty
# for a first HTTP-only pass against the ALB's own DNS name)
terraform init
terraform plan   # review every resource before applying anything
```

## 2. First apply (creates everything except a usable API — no image yet)
```bash
terraform apply
```
At this point RDS, ElastiCache, the VPC, ALB, and an ECS *service* all
exist, but `aws_ecs_task_definition.api`'s `image` is whatever
`var.api_image` was set to (empty by default) — the service will sit
with 0 running tasks until step 4.

## 3. Build and push the API image
This is also the first time this project's `go build` would actually
succeed — it has never been run to completion anywhere in this project's
history before now, because every sandbox session building it had no
module-fetch network access (see `../README.md`'s Phase 11 section and
`../loadtest/REPORT.md`). From a machine with normal internet access:
```bash
cd backend
go mod tidy    # this is also the first real go.sum this project will ever have
go build ./... # confirm it actually compiles, for the first time for real
go test ./...  # likewise the first real `go test` run, not gofmt-level syntax checking

aws ecr get-login-password --region ap-southeast-1 | \
  docker login --username AWS --password-stdin <account-id>.dkr.ecr.ap-southeast-1.amazonaws.com

docker build -t chatplatform-api:v0.1.0 .
docker tag chatplatform-api:v0.1.0 <ecr_repository_url_from_terraform_output>:v0.1.0
docker push <ecr_repository_url_from_terraform_output>:v0.1.0
```

## 4. Point the service at the real image
```bash
cd terraform
terraform apply -var="api_image=<ecr_repository_url>:v0.1.0"
# or set api_image in terraform.tfvars and re-run `terraform apply`
```
Wait for `aws ecs describe-services` to show `runningCount == desiredCount`
(2, by default) before moving on.

## 5. Run the database migrations
The migration runner referenced throughout `backend/migrations/` has
never had its forward+back correctness checked against *this* schema
with a real network-reachable Postgres until Phase 12's local sandbox
test (`../loadtest/REPORT.md` section 1) — but that was against a local,
throwaway database, not this real RDS instance. Re-run the same
forward+back check against the real `aws_db_instance.main` endpoint
before pointing real traffic at it:
```bash
# from a machine that can reach the VPC (a bastion, or a temporary
# public-subnet EC2, or AWS Systems Manager Session Manager port forwarding)
for f in backend/migrations/*.up.sql; do psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"; done
```

## 6. Seed test data and run the k6 load test for real
```bash
# seed one fixed peer account the k6 suite's PEER_USER_ID points at —
# e.g. reuse backend/README.md's own OTP-login curl walkthrough once
# against the real deployed API to create one
cd ../loadtest/k6
k6 run -e BASE_URL=$(terraform -chdir=../../terraform output -raw api_url) \
       -e WS_URL=wss://<domain or ALB dns>/ws \
       -e PEER_USER_ID=<seeded-user-uuid> \
       chat-load-test.js
```
This is the run `../REPORT.md` section 2 describes as "written, not
executed" — update that section with the real thresholds-pass/fail
output once this has actually run.

## 7. The WS-survives-LB-failover drill
This is the roadmap's actual Phase 12 exit criterion, and it requires a
real multi-task deployment to even attempt:

1. Open a handful of real WebSocket connections against the deployed
   API (the k6 `websocket_send` scenario above already does this, or
   connect a few Android devices / `websocat` sessions by hand) and
   confirm messages are flowing (the AI/chat screens show messages
   arriving).
2. Note which ECS task each connection landed on
   (`aws ecs list-tasks` + checking container IPs, or add a temporary
   debug log line echoing the task's own IP/task ARN on WS connect).
3. Force-stop that task: `aws ecs stop-task --cluster <cluster> --task <task-arn>`.
4. **Expected behavior**, per the architecture doc's design and
   `alb.tf`'s comments: the ALB's health check marks that target
   unhealthy and stops routing new connections to it within
   `health_check.interval * unhealthy_threshold` (~45s); existing
   WebSocket clients notice their connection drop (missed ping/pong,
   architecture doc section 6) and reconnect with the Android client's
   existing exponential-backoff logic; they land on a *different*,
   healthy task; because every gateway node is stateless and fans
   messages out via Redis pub/sub rather than depending on which node
   a client is connected to, a message sent to a user who reconnected
   to a different node should still arrive.
5. **Record what actually happened** — reconnect time, whether any
   message sent during the gap was lost vs. merely delayed (check
   `security_audit_log` / app logs for errors, and the k6
   `ws_message_ack_missing` metric if the load test was running
   concurrently with the drill, which is the most realistic way to run
   this step).
6. This step has never been performed against a real deployment in this
   project's history. Until it has, "WS survives LB failover" is a
   design intent backed by the architecture (stateless nodes + Redis
   fan-out + client backoff, each individually true) and by the
   `deregistration_delay`/health-check tuning in `alb.tf`'s comments —
   not yet an empirically verified fact. Update this runbook and
   `../REPORT.md` with the real result once run.

## 8. Rollback
```bash
# redeploy the previous image tag
terraform apply -var="api_image=<ecr_repository_url>:<previous-tag>"
```
ECS's rolling deployment (`deployment_minimum_healthy_percent = 100`)
means this never drops below `api_desired_count` healthy tasks mid-roll.

## 9. Tearing it all down
```bash
terraform destroy
```
`aws_db_instance.main.skip_final_snapshot` is `true` for any
`environment` other than `"production"` (see `rds.tf`) — a non-prod
`destroy` does not leave a final RDS snapshot behind to pay for by
accident; a `production`-tagged environment does, deliberately.
