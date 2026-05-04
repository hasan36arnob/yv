# TodoPro SaaS - Implementation & Deployment Guide

## Architecture Overview

### Core Components

1. **Go Backend (main.go)**
   - PostgreSQL via GORM ORM
   - JWT Authentication middleware
   - Rate limiting per IP (100 req/min)
   - WebSocket real-time hub
   - Stripe integration (checkout, webhooks)
   - CORS-enabled REST API
   - Embedded static files (HTML/CSS/JS)

2. **PostgreSQL Database**
   - Multi-tenant schema with Row-Level Security ready
   - 6 core tables + audit logs
   - Optimized indexes for performance
   - Stored procedures for analytics

3. **Frontend (Vanilla JS)**
   - Auth flows (login/register)
   - JWT token management
   - WebSocket client for live updates
   - Stripe checkout integration
   - Responsive dashboard with analytics
   - Workspace/team switcher

## Quick Start (Local Development)

### Prerequisites

- Go 1.21+
- PostgreSQL 12+
- Stripe account (test mode)

### 1. Database Setup

```bash
# Create database
psql -U postgres -c "CREATE DATABASE todopro;"

# Run schema
psql -U postgres -d todopro -f database_schema.sql

# (Optional) Insert demo user with hashed password "password123"
# password hash: $2a$10$N.zmdr9k7UOCG3aZHWeC.uJVxW36aZbfkJOG5c1bJO7sVo5S8sMPm
```

### 2. Environment Configuration

```bash
# Copy env template
cp .env.example .env
```

Edit `.env`:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=todopro

# JWT
JWT_SECRET=your-super-secret-jwt-key-min-32-chars

# Stripe
STRIPE_SECRET_KEY=sk_test_your_stripe_secret_key
STRIPE_PUBLIC_KEY=pk_test_your_stripe_public_key
STRIPE_WEBHOOK_SECRET=whsec_your_webhook_signing_secret

# App
APP_URL=http://localhost:5000
```

### 3. Install Dependencies

```bash
go mod tidy
go mod download
```

### 4. Run Server

```bash
go run main.go
```

Open http://localhost:5000

## Stripe Configuration

### Webhook Setup

1. In Stripe Dashboard → Developers → Webhooks
2. Add endpoint: `https://yourdomain.com/api/stripe/webhook`
3. Select events:
   - `checkout.session.completed`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `invoice.payment_failed`
4. Copy webhook signing secret to `.env` as `STRIPE_WEBHOOK_SECRET`

### Checkout Flow

- Frontend calls `/api/checkout/create` with `plan_id` (personal, pro, team)
- Backend creates Stripe checkout session
- User redirected to Stripe-hosted page
- On success, Stripe sends webhook → subscription activated
- Frontend polls `/api/profile` for plan status

## Production Deployment

### Docker

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o todopro .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/todopro .
COPY --from=builder /app/.env .
EXPOSE 5000
CMD ["./todopro"]
```

```bash
docker build -t todopro .
docker run -p 5000:5000 --env-file .env todopro
```

### Docker Compose (Recommended)

```yaml
# docker-compose.yml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_USER: todopro
      POSTGRES_PASSWORD: securepassword
      POSTGRES_DB: todopro
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./database_schema.sql:/docker-entrypoint-initdb.d/init.sql
    ports:
      - "5432:5432"

  backend:
    build: .
    ports:
      - "5000:5000"
    environment:
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: todopro
      DB_PASSWORD: securepassword
      DB_NAME: todopro
      JWT_SECRET: ${JWT_SECRET}
      STRIPE_SECRET_KEY: ${STRIPE_SECRET_KEY}
      STRIPE_PUBLIC_KEY: ${STRIPE_PUBLIC_KEY}
      STRIPE_WEBHOOK_SECRET: ${STRIPE_WEBHOOK_SECRET}
      APP_URL: ${APP_URL}
    depends_on:
      - postgres
    restart: unless-stopped

volumes:
  postgres_data:
```

### Environment Variables Reference

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DB_HOST` | PostgreSQL host | Yes | localhost |
| `DB_PORT` | PostgreSQL port | Yes | 5432 |
| `DB_USER` | DB username | Yes | postgres |
| `DB_PASSWORD` | DB password | Yes | - |
| `DB_NAME` | Database name | Yes | todopro |
| `JWT_SECRET` | JWT signing secret | Yes | dev-key |
| `STRIPE_SECRET_KEY` | Stripe secret key | For payments | - |
| `STRIPE_PUBLIC_KEY` | Stripe public key | For payments | - |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook secret | For payments | - |
| `APP_URL` | Public app URL | Yes | http://localhost:5000 |

### Nginx Reverse Proxy (Production)

```nginx
server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:5000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # WebSocket support
    location /ws {
        proxy_pass http://localhost:5000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_set_header Host $host;
    }
}
```

## Security Best Practices

### 1. Rate Limiting

Already configured at 100 requests/minute per IP. Adjust in `main.go`:

```go
const (
    RateLimitRequests = 100 // Increase for paid users
    RateLimitWindow   = 60 * time.Second
)
```

### 2. JWT Configuration

- Use strong secret (32+ chars)
- Set short expiry (24h currently)
- Store refresh tokens securely (separate table, hashed)
- Implement token rotation

### 3. SQL Injection Prevention

GORM parameterized queries already protect against SQL injection.

### 4. CORS

Currently allowing all origins (`*`). In production, restrict:

```go
w.Header().Set("Access-Control-Allow-Origin", "https://yourdomain.com")
```

### 5. Password Security

- BCrypt with default cost (10)
- Minimum password length: 8 characters
- Consider adding password strength meter

## Scaling Considerations

### Database Optimization

1. **Connection Pooling**
   ```go
   // In main.go, after db connection
   sqlDB, _ := db.DB()
   sqlDB.SetMaxOpenConns(25)
   sqlDB.SetMaxIdleConns(25)
   sqlDB.SetConnMaxLifetime(5 * time.Minute)
   ```

2. **Read Replicas** (for high-traffic)
   - Implement read/write splitting
   - Route analytics queries to replicas

3. **Caching Layer** (Redis)
   ```go
   // Cache frequent queries:
   // - User profile
   // - Workspace membership
   // - Analytics aggregates (precompute)
   ```

### WebSocket Scaling

- Use Redis Pub/Sub for multi-instance WebSocket broadcast
- Deploy behind load balancer with sticky sessions
- Consider managed services (Pusher, Ably) for extreme scale

### File Storage

For attachments/avatars:
- AWS S3
- CloudFlare R2
- Google Cloud Storage

## Monitoring & Logging

### Structured Logging (replace log.Println)

```go
import "go.uber.org/zap"

logger, _ := zap.NewProduction()
defer logger.Sync()

logger.Info("User registered", zap.Uint("user_id", user.ID))
```

### Metrics Collection

- Prometheus for Go metrics
- Grafana dashboards
- Track: request latency, error rates, active connections

### Health Checks

Already have `/health` endpoint. Expand:

```json
{
  "status": "healthy",
  "timestamp": "2026-01-01T00:00:00Z",
  "checks": {
    "database": "healthy",
    "redis": "healthy"
  },
  "version": "1.0.0"
}
```

## Feature Flag System

Feature flags table already in schema. Usage:

```go
func isFeatureEnabled(workspaceID uint, featureName string) bool {
    var flag FeatureFlag
    db.Where("name = ?", featureName).First(&flag)
    
    if !flag.Enabled {
        return false
    }
    
    // Check plan limits
    var workspace Workspace
    db.First(&workspace, workspaceID)
    
    // Check rollout percentage (for gradual rollout)
    // ...
}
```

## Payment & Billing

### Pricing Model

| Plan | Price | Features |
|------|-------|----------|
| Free | $0 | 100 tasks, 1 workspace |
| Personal | $4/mo | Unlimited tasks, 1 workspace |
| Pro | $9/mo | Unlimited tasks, 3 workspaces, API access |
| Team | $15/user/mo | Unlimited everything, admin controls, SSO |

### Stripe Products Setup

1. Create 4 products in Stripe Dashboard
2. Create price IDs for monthly/yearly
3. Store price IDs in config
4. Map `plan_id` to Stripe Price IDs

```go
var stripePriceIDs = map[string]string{
    "personal_monthly": "price_xxx",
    "personal_yearly":  "price_yyy",
    "pro_monthly":      "price_zzz",
    // ...
}
```

### Usage-Based Billing (Optional)

For metered features (API calls, storage), add:

- Usage records table
- Metered billing in Stripe
- Daily aggregation job

## Team Collaboration Features

### Invitation Flow

1. Admin clicks "Invite" → enters email
2. Backend finds user by email
3. Creates `team_members` record with `pending` status
4. Sends email notification (implement with SendGrid/Mailgun)
5. Invited user accepts → membership activated

### Role-Based Access Control (RBAC)

| Role | Permissions |
|------|-------------|
| Owner | Full control, delete workspace, manage billing |
| Admin | Manage members, all task operations |
| Member | Create/edit own tasks |
| Viewer | Read-only access |

Implemented in `requireWorkspaceAccess` middleware.

### Activity Logging

Activity logs table tracks:
- Task create/update/delete
- Member invites
- Workspace changes
- Billing events

## Analytics & Reporting

### Pre-computed Metrics

Daily job aggregates:

```sql
-- Compute daily workspace metrics
INSERT INTO workspace_daily_metrics (workspace_id, date, tasks_created, tasks_completed, active_members)
SELECT 
    workspace_id,
    DATE(created_at) as date,
    COUNT(*) as tasks_created,
    COUNT(*) FILTER (WHERE status = 'completed') as tasks_completed,
    COUNT(DISTINCT created_by) as active_members
FROM tasks
WHERE created_at >= CURRENT_DATE - INTERVAL '1 day'
GROUP BY workspace_id, DATE(created_at);
```

Schedule with cron or pg_cron.

### Real-time Dashboards

WebSocket pushes updates to all connected clients when:
- Task created/updated/deleted
- Member joined/left
- Workspace stats change

## Data Export

Implement CSV/Excel export:

```go
func exportTasksCSV(w http.ResponseWriter, r *http.Request) {
    workspaceID := r.URL.Query().Get("workspace_id")
    // Query tasks
    // Write CSV with encoding/csv
}
```

## API Documentation (OpenAPI/Swagger)

Generate with `swaggo`:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g main.go
```

See `docs/swagger.yaml` for full spec.

## SEO & Performance

### Search Engine Optimization

- Add canonical tags
- Open Graph meta tags
- Sitemap.xml
- Structured data (JSON-LD)

### Performance Optimization

1. **Frontend**
   - Code splitting (if migrate to React/Vue)
   - Lazy load images
   - Bundle minification

2. **Backend**
   - Database query optimization (EXPLAIN ANALYZE)
   - Response compression (gzip)
   - HTTP/2 enable
   - CDN for static assets

### Analytics Integration

Add Google Analytics / Plausible:

```html
<script async defer data-domain="yourdomain.com" src="https://plausible.io/js/plausible.js"></script>
```

## Troubleshooting

### WebSocket Connection Fails

- Check firewall allows port 5000 (or your port)
- Verify WebSocket URL uses correct protocol (ws/wss)
- Ensure CORS headers include `Upgrade` and `Connection`

### Stripe Webhook Not Received

- Use `stripe listen` CLI for local testing: `stripe listen --forward-to localhost:5000/api/stripe/webhook`
- Ensure webhook secret matches
- Check signature verification

### Database Connection Issues

- Verify credentials in `.env`
- Check `pg_hba.conf` allows connections
- Run `SELECT 1;` to test connection

### JWT Token Expired

- Call `/api/refresh` with refresh token
- Implement token rotation
- Clear localStorage on 401 errors

## Support & Maintenance

### Regular Tasks

- Database backups (daily)
- Log rotation (weekly)
- Security updates (monthly)
- Performance monitoring

### Upgrade Path

1. Test migrations on staging
2. Backup production DB
3. Deploy new version
4. Monitor logs for errors
5. Rollback if needed

## Next Steps for Full Commercial Ready

1. **Email Service** - SendGrid/Mailgun for invitations, notifications
2. **File Attachments** - S3 presigned URLs
3. **Mobile Apps** - React Native wrapper with same API
4. **SSO/SAML** - Enterprise authentication
5. **Advanced Permissions** - Fine-grained per-task/per-project
6. **Time Tracking** - Integrate time entries
7. **Calendar Integration** - CalDAV/Google Calendar sync
8. **Zapier/Make Integration** - Connect to 5000+ apps
9. **API for Developers** - Public API with OAuth
10. **White Label** - Custom branding for agencies

---

**For Questions/Support:** Create issue at repository or contact dev team.
