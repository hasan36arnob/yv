# TodoPro Quick Start Guide

## Option 1: Docker (Easiest)

### Prerequisites
- Docker Desktop installed
- Git (optional)

### Steps

1. **Clone/Download the project**
   ```bash
   cd D:\yv  # or wherever you saved the files
   ```

2. **Configure environment**
   ```bash
   # Edit .env file (created automatically)
   # Set your database password and Stripe keys if needed
   ```

3. **Start everything**
   ```bash
   # Using the deploy script (Linux/macOS/WSL)
   chmod +x deploy.sh
   ./deploy.sh setup
   ./deploy.sh run
   
   # Or using docker-compose directly
   docker-compose up -d
   ```

4. **Access the app**
   - App: http://localhost:5000
   - API: http://localhost:5000/api
   - Database: localhost:5432 (user: todopro, pass: from .env)

---

## Option 2: Native Go (Development)

### Prerequisites
- Go 1.21+
- PostgreSQL 12+

### Steps

1. **Install dependencies**
   ```bash
   go mod tidy
   go mod download
   ```

2. **Setup database**
   ```bash
   # Create database
   createdb todopro -U postgres
   
   # Or use psql
   psql -U postgres -c "CREATE DATABASE todopro;"
   
   # Run schema
   psql -U postgres -d todopro -f database_schema.sql
   ```

3. **Configure environment**
   ```bash
   # Create .env file
   cp .env.example .env
   
   # Edit with your settings
   nano .env  # or use any editor
   ```

   Minimum required:
   ```env
   DB_HOST=localhost
   DB_PORT=5432
   DB_USER=postgres
   DB_PASSWORD=your_postgres_password
   DB_NAME=todopro
   JWT_SECRET=your-random-secret-key-min-32-chars
   APP_URL=http://localhost:5000
   ```

4. **Run the server**
   ```bash
   go run main.go
   ```

5. **Open browser**
   - http://localhost:5000

---

## First Run Checklist

After starting the server:

- [ ] Visit http://localhost:5000 - landing page loads
- [ ] Click "Start Free Trial" - registration modal appears
- [ ] Create account with email/password
- [ ] After login, you should see "My Tasks" dashboard
- [ ] Add a task - it appears in the list
- [ ] Mark complete - strikethrough appears
- [ ] Create a second workspace from user menu
- [ ] Switch between workspaces
- [ ] Check `/health` endpoint returns JSON

---

## Testing the API

### Using curl

```bash
# Health check
curl http://localhost:5000/health

# Register
curl -X POST http://localhost:5000/api/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@test.com","password":"Test12345","first_name":"Test","last_name":"User"}'

# Login
curl -X POST http://localhost:5000/api/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@test.com","password":"Test12345"}'

# Get profile (with token from login)
curl http://localhost:5000/api/profile \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Using the built-in test script

```bash
chmod +x deploy.sh
./deploy.sh test
```

---

## Database Management

### Connect via psql

```bash
# Using docker-compose
docker-compose exec postgres psql -U todopro -d todopro

# Or native
psql -h localhost -U todopro -d todopro
```

### Useful queries

```sql
-- List all users
SELECT id, email, subscription_plan FROM users;

-- List tasks in workspace 1
SELECT * FROM tasks WHERE workspace_id = 1;

-- Get workspace stats
SELECT * FROM get_workspace_stats(1);

-- See team members
SELECT u.email, tm.role FROM team_members tm
JOIN users u ON u.id = tm.user_id
WHERE workspace_id = 1;
```

---

## Common Issues & Solutions

### Issue: "database is locked" or connection errors

**Solution:** Check PostgreSQL is running:
```bash
pg_isready
# or
docker-compose ps postgres
```

### Issue: "relation does not exist" errors

**Solution:** Run database migrations:
```bash
psql -U postgres -d todopro -f database_schema.sql
```

### Issue: JWT token invalid after restart

**Solution:** JWT uses a secret key. If you changed `JWT_SECRET` in .env, old tokens become invalid. Just re-login.

### Issue: CORS errors in browser console

**Solution:** Frontend runs on different port than backend. The backend has CORS enabled for all origins (development). If issues occur, check browser console for specific blocked headers.

### Issue: WebSocket won't connect

**Solution:**
- Ensure you're using `ws://` (not `http://`) for localhost
- Check firewall isn't blocking port 5000
- In production, use `wss://` with SSL

### Issue: Stripe checkout not loading

**Solution:**
- Set `STRIPE_PUBLIC_KEY` in .env
- For local dev, use Stripe test keys from dashboard
- Use `stripe listen` CLI to forward webhooks:
  ```bash
  stripe listen --forward-to localhost:5000/api/stripe/webhook
  ```

---

## Development Tips

### Hot Reload (Go)

Install air for live reload:
```bash
go install github.com/cosmtrek/air@latest
air
```

Or use CompileDaemon:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### Database Changes

1. Update `database_schema.sql`
2. For development, drop and recreate DB:
   ```bash
   dropdb todopro -U postgres
   createdb todopro -U postgres
   psql -U postgres -d todopro -f database_schema.sql
   ```

### Frontend Changes

Static files are embedded via `go:embed`. Go rebuilds binary when code changes, but not CSS/JS automatically:

**Option A:** Rebuild binary on every change (development):
```bash
# In one terminal:
go run main.go

# In another, watch for CSS/JS changes and restart:
# Use entr or similar tool
ls style.css script.js | entr -r go run main.go
```

**Option B:** Serve static files externally (better for frontend work):
Edit `main.go` to serve from filesystem not embedded FS during dev:
```go
// Instead of:
http.FileServer(http.FS(staticFiles))

// Use:
http.FileServer(http.Dir("."))  // serves current directory
```

### Add New Endpoint

1. Define handler in `main.go`
2. Add route in `main()` function
3. (Optional) Add middleware: `rateLimit(withCORS(requireAuth(handler)))`
4. Test with curl/Postman

---

## Production Checklist

Before going live:

- [ ] Change `JWT_SECRET` to 64+ random chars
- [ ] Set strong PostgreSQL password
- [ ] Configure HTTPS (nginx reverse proxy with SSL)
- [ ] Set `APP_URL` to real domain
- [ ] Configure real Stripe keys (not test keys)
- [ ] Set up Stripe webhook endpoint
- [ ] Add domain to CORS (restrict origins)
- [ ] Enable rate limiting
- [ ] Set up database backups (daily)
- [ ] Configure logging to file/external service
- [ ] Add monitoring (UptimeRobot, DataDog, etc.)
- [ ] Create privacy policy & terms of service
- [ ] Set up email service (SendGrid/Mailgun)
- [ ] Load test with k6 or similar

---

## Migrating from Demo JSON Version

If you have existing tasks in `tasks.json`:

```bash
# Convert and import (manual process)
# 1. Export tasks.json
# 2. Create migration script
# 3. Insert into PostgreSQL via psql
```

**Example migration script:**

```go
package main

import (
    "encoding/json"
    "io/ioutil"
    "gorm.io/gorm"
)

type OldTask struct {
    ID        int64  `json:"id"`
    Text      string `json:"text"`
    Completed bool   `json:"completed"`
    CreatedAt string `json:"createdAt"`
}

func migrate(db *gorm.DB) error {
    data, _ := ioutil.ReadFile("tasks.json")
    var old struct {
        Tasks []OldTask `json:"tasks"`
    }
    json.Unmarshal(data, &old)
    
    // Get default workspace (create if needed)
    var workspace Workspace
    db.FirstOrCreate(&workspace, Workspace{Name: "Imported Tasks"})
    
    // Convert tasks
    for _, t := range old.Tasks {
        task := Task{
            WorkspaceID: workspace.ID,
            Title:       t.Text,
            Status:      "pending",
            Priority:    "medium",
            CreatedBy:   1, // system user
        }
        if t.Completed {
            task.Status = "completed"
        }
        db.Create(&task)
    }
    
    return nil
}
```

---

## Performance Optimization

For 10k+ users:

1. **Database**
   - Add read replicas
   - Connection pooling (set `max_open_conns`)
   - Partition large tables by date

2. **Caching**
   - Redis for session storage
   - Cache workspace membership
   - Cache analytics aggregates

3. **CDN**
   - Serve static files via CloudFlare
   - Edge caching for public assets

4. **Horizontal Scaling**
   - Multiple backend instances behind load balancer
   - WebSocket with Redis Pub/Sub
   - Database connection pool per instance

---

## Need Help?

- **Docs:** See README.md, API.md, ROADMAP.md
- **Issues:** Create GitHub issue
- **Stripe Docs:** https://stripe.com/docs
- **PostgreSQL Docs:** https://postgresql.org/docs
- **GORM Docs:** https://gorm.io/docs

---

**You're ready to build a SaaS business! 🚀**

Good luck, and feel free to customize this as much as you'd like!
