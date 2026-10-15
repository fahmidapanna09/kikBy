data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags = { Name = "chatplatform-${var.environment}" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "chatplatform-${var.environment}-igw" }
}

# Public subnets — only the ALB and NAT gateways live here.
resource "aws_subnet" "public" {
  count                   = var.az_count
  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, count.index)
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true
  tags                    = { Name = "chatplatform-${var.environment}-public-${count.index}" }
}

# Private subnets — ECS tasks, RDS, ElastiCache. No direct route to the
# internet gateway; outbound-only via NAT (for pulling container images,
# calling the Gemini API, etc).
resource "aws_subnet" "private" {
  count             = var.az_count
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + 100)
  availability_zone = data.aws_availability_zones.available.names[count.index]
  tags              = { Name = "chatplatform-${var.environment}-private-${count.index}" }
}

resource "aws_eip" "nat" {
  count  = var.az_count
  domain = "vpc"
  tags   = { Name = "chatplatform-${var.environment}-nat-eip-${count.index}" }
}

# One NAT gateway per AZ — costs more than a single shared NAT, but a
# single NAT gateway is a cross-AZ single point of failure, which
# directly undermines the "survives failover" goal this whole phase is
# about. Downgrade to one shared NAT only as a deliberate cost trade-off,
# not a default.
resource "aws_nat_gateway" "main" {
  count         = var.az_count
  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id
  tags          = { Name = "chatplatform-${var.environment}-nat-${count.index}" }
  depends_on    = [aws_internet_gateway.main]
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }
  tags = { Name = "chatplatform-${var.environment}-public-rt" }
}

resource "aws_route_table_association" "public" {
  count          = var.az_count
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  count  = var.az_count
  vpc_id = aws_vpc.main.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main[count.index].id
  }
  tags = { Name = "chatplatform-${var.environment}-private-rt-${count.index}" }
}

resource "aws_route_table_association" "private" {
  count          = var.az_count
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}
