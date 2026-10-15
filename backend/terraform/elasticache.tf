resource "aws_elasticache_subnet_group" "main" {
  name       = "chatplatform-${var.environment}"
  subnet_ids = aws_subnet.private[*].id
}

# A replication group (not a single aws_elasticache_cluster) even at the
# smallest node size — this is what gives automatic failover to a
# replica, which is the Redis half of this phase's "survives failover"
# exit criterion. Everything Redis holds (presence, the WS connection
# registry, OTP state, rate limits) is explicitly disposable per the
# architecture doc's section 1 ("Why Redis is not the source of truth"),
# so losing it briefly during a failover is an acceptable, documented
# blip — never a reason to skip having failover at all.
resource "aws_elasticache_replication_group" "main" {
  replication_group_id = "chatplatform-${var.environment}"
  description           = "chatplatform ${var.environment} — presence/pubsub/cache/rate-limit, never the source of truth"

  engine         = "redis"
  engine_version = "7.1"
  node_type      = var.redis_node_type
  port           = 6379

  num_cache_clusters         = 2 # primary + 1 replica, minimum for automatic failover
  automatic_failover_enabled = true
  multi_az_enabled           = true

  subnet_group_name  = aws_elasticache_subnet_group.main.name
  security_group_ids = [aws_security_group.redis.id]

  at_rest_encryption_enabled = true
  transit_encryption_enabled = true

  snapshot_retention_limit = 1

  tags = { Name = "chatplatform-${var.environment}-redis" }
}
