resource "aws_lb" "main" {
  name               = "chatplatform-${var.environment}"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id

  # Long-lived WebSocket connections (/ws) need a longer idle timeout
  # than the ALB's 60s default — the app's own 30s ping/pong keepalive
  # (architecture doc section 6) should keep a healthy connection well
  # under this anyway, but 60s is exactly how long a brief GC pause or a
  # slow client could go quiet and get killed for no real reason.
  idle_timeout = 120

  tags = { Name = "chatplatform-${var.environment}-alb" }
}

resource "aws_lb_target_group" "api" {
  name        = "chatplatform-${var.environment}-api"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip" # required for Fargate

  health_check {
    path                = "/healthz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 15
    timeout             = 5
    matcher             = "200"
  }

  # Deregistration delay is the other half of "WS survives LB failover":
  # when a task is drained (deploy, scale-in, or an unhealthy task being
  # replaced), the ALB stops sending it *new* connections immediately but
  # keeps existing ones open for this long, giving already-connected WS
  # clients a graceful window to notice (via ping/pong) and reconnect
  # elsewhere rather than having every open socket cut instantly.
  deregistration_delay = 30

  # NOT using stickiness here on purpose. The architecture doc's WS
  # design is explicitly built to NOT require it: every gateway node is
  # stateless with respect to message storage, and nodes fan messages
  # out to each other over Redis pub/sub (section 1, "cross-node
  # fan-out") specifically so a client can land on *any* healthy node
  # and still receive messages addressed to it. Turning on ALB
  # stickiness would paper over a real fan-out bug by accident (it would
  # keep "working" in a one-task test and then fail the moment a client
  # reconnects to a different task) — this target group is deliberately
  # configured to fail loudly instead, during the failover drill in
  # ../RUNBOOK.md step 7, if that fan-out ever breaks.
  stickiness {
    type    = "lb_cookie"
    enabled = false
  }

  tags = { Name = "chatplatform-${var.environment}-api-tg" }
}

locals {
  has_domain = var.domain_name != ""
}

# No domain set (local.has_domain == false): plain HTTP straight to the
# target group, so a first load-test deployment can be reached at the
# ALB's own DNS name with nothing else to configure. This is for
# k6/internal testing only — the Android app and any real user traffic
# need the HTTPS listener below, which requires var.domain_name.
resource "aws_lb_listener" "http_plain" {
  count             = local.has_domain ? 0 : 1
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}

resource "aws_lb_listener" "http_redirect" {
  count             = local.has_domain ? 1 : 0
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  count             = local.has_domain ? 1 : 0
  load_balancer_arn = aws_lb.main.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.api[0].certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}
