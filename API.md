# TodoPro API Documentation

## Base URL

```
Production: https://api.todopro.com
Development: http://localhost:5000
```

All endpoints are prefixed with `/api`

## Authentication

All protected endpoints require a Bearer token in the Authorization header:

```
Authorization: Bearer <your_jwt_token>
```

### Obtaining a Token

**POST** `/api/register` - Create account

**POST** `/api/login` - Sign in

Both return:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "refresh_token_string",
  "user": {
    "id": 1,
    "email": "user@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "role": "member",
    "subscription_plan": "free"
  }
}
```

### Refresh Token

**POST** `/api/refresh`

```json
{
  "refresh_token": "your_refresh_token"
}
```

Response:
```json
{
  "token": "new_jwt_token",
  "refresh_token": "new_refresh_token"
}
```

---

## Error Responses

All endpoints return consistent error format:

```json
{
  "error": "Human readable error message",
  "code": "ERROR_CODE", // optional
  "details": {} // optional additional info
}
```

HTTP Status Codes:
- `200` - Success
- `201` - Created
- `400` - Bad request (validation)
- `401` - Unauthorized (invalid/missing token)
- `403` - Forbidden (insufficient permissions)
- `404` - Not found
- `429` - Rate limited
- `500` - Server error

---

## Endpoints

### Authentication

#### Register User

```
POST /api/register
```

**Request Body:**
```json
{
  "email": "user@company.com",
  "password": "securePassword123",
  "first_name": "John",
  "last_name": "Doe"
}
```

**Response (201):**
```json
{
  "token": "...",
  "refresh_token": "...",
  "user": {
    "id": 1,
    "email": "user@company.com",
    "first_name": "John",
    "last_name": "Doe",
    "role": "member",
    "subscription_plan": "free"
  }
}
```

#### Login

```
POST /api/login
```

**Request Body:**
```json
{
  "email": "user@company.com",
  "password": "securePassword123"
}
```

**Response (200):** Same as register

#### Get Profile

```
GET /api/profile
```

**Headers:** `Authorization: Bearer <token>`

**Response (200):**
```json
{
  "id": 1,
  "email": "user@company.com",
  "first_name": "John",
  "last_name": "Doe",
  "role": "member",
  "subscription_plan": "free",
  "workspaces": [
    {
      "id": 1,
      "name": "Acme Corp",
      "plan_tier": "pro"
    }
  ]
}
```

---

### Workspaces

Workspaces are containers for tasks and team members. Each user can belong to multiple workspaces.

#### List Workspaces

```
GET /api/workspaces
```

**Response (200):**
```json
[
  {
    "id": 1,
    "name": "Acme Corp",
    "description": "Main company workspace",
    "owner_id": 1,
    "plan_tier": "pro",
    "is_active": true,
    "created_at": "2026-01-01T00:00:00Z"
  }
]
```

#### Create Workspace

```
POST /api/workspaces/create
```

**Request Body:**
```json
{
  "name": "New Project",
  "description": "Optional workspace description"
}
```

**Response (201):** Full workspace object

#### Invite Member

```
POST /api/workspaces/invite?workspace_id=1
```

**Request Body:**
```json
{
  "email": "newmember@company.com",
  "role": "member" // "owner", "admin", "member", "viewer"
}
```

**Response (201):**
```json
{
  "message": "Member invited successfully"
}
```

---

### Tasks

#### List Tasks

```
GET /api/tasks?workspace_id=1
```

**Query Parameters:**
- `workspace_id` (required) - Workspace to fetch tasks from
- `assignee_id` (optional) - Filter by assignee
- `status` (optional) - Filter by status

**Response (200):**
```json
[
  {
    "id": 1,
    "workspace_id": 1,
    "title": "Design homepage",
    "description": "Create homepage mockups",
    "status": "pending",
    "priority": "high",
    "assignee_id": 2,
    "assignee": {
      "id": 2,
      "first_name": "Jane",
      "last_name": "Smith",
      "email": "jane@company.com"
    },
    "due_date": "2026-01-15T00:00:00Z",
    "created_by": 1,
    "created_at": "2026-01-01T10:00:00Z",
    "updated_at": "2026-01-01T10:00:00Z"
  }
]
```

#### Create Task

```
POST /api/tasks/create?workspace_id=1
```

**Request Body:**
```json
{
  "title": "New Task",
  "description": "Task details",
  "priority": "high", // low, medium, high, urgent
  "due_date": "2026-01-20T17:00:00Z", // optional RFC3339
  "assignee_id": 2 // optional
}
```

**Response (201):** Task object

#### Update Task

```
PUT /api/tasks/update?id=1
```

**Request Body:** (all fields optional)
```json
{
  "title": "Updated title",
  "description": "Updated description",
  "status": "in_progress", // pending, in_progress, completed
  "priority": "medium",
  "due_date": "2026-01-25T00:00:00Z",
  "assignee_id": 3
}
```

**Response (200):** Updated task object

#### Delete Task

```
DELETE /api/tasks/delete?id=1
```

**Response (200):**
```json
{
  "message": "Task deleted successfully"
}
```

---

### Analytics

#### Get Workspace Analytics

```
GET /api/analytics?workspace_id=1
```

**Permissions:** Requires admin or owner role

**Response (200):**
```json
{
  "summary": {
    "total": 50,
    "completed": 32,
    "pending": 15,
    "overdue": 3,
    "completion_rate": 64.0
  },
  "by_assignee": [
    {
      "user_id": 2,
      "user": {
        "id": 2,
        "first_name": "Jane",
        "last_name": "Smith",
        "email": "jane@company.com"
      },
      "total": 22,
      "completed": 15,
      "in_progress": 4,
      "pending": 3
    }
  ],
  "trend": [
    {
      "date": "2026-01-01",
      "completed": 5,
      "created": 7
    },
    {
      "date": "2026-01-02",
      "completed": 8,
      "created": 3
    }
  ]
}
```

---

### Stripe Payments

#### Create Checkout Session

```
POST /api/checkout/create
```

**Request Body:**
```json
{
  "plan_id": "pro" // personal, pro, team
}
```

**Response (200):**
```json
{
  "session_id": "cs_test_...",
  "url": "https://checkout.stripe.com/pay/cs_test_..."
}
```

#### Stripe Webhook

```
POST /api/stripe/webhook
```

**Headers:**
```
Stripe-Signature: t=...,v1=...
```

**Body:** Raw Stripe event payload

**Response (200):** Empty (Stripe expects 2xx)

---

## WebSocket API

Connect to real-time task updates:

### Connection URL

```
ws://localhost:5000/ws?workspace_id=1
```

**Authorization:** Include JWT in Sec-WebSocket-Protocol header or as query param

### Message Format

**Incoming (Server → Client):**
```json
{
  "type": "task_created",
  "payload": {
    "id": 123,
    "title": "New task",
    "status": "pending",
    // ... task fields
  },
  "timestamp": 1706635200
}
```

**Outgoing (Client → Server):**
```json
{
  "type": "ping",
  "timestamp": 1706635200
}
```

### Message Types

| Type | Description |
|------|-------------|
| `task_created` | New task added |
| `task_updated` | Task modified |
| `task_deleted` | Task removed |
| `user_joined` | Team member joined workspace |
| `user_left` | Team member left |
| `connected` | Connection established |

### Heartbeat

Server sends ping every 54 seconds, respond with pong:

**Ping:**
```json
{"type": "ping", "timestamp": 1706635200}
```

**Pong:**
```json
{"type": "pong", "timestamp": 1706635200}
```

---

## Rate Limits

| Endpoint | Limit |
|----------|-------|
| All API endpoints | 100 requests/minute |
| Auth endpoints | 5 attempts/minute (IP) |
| WebSocket | Unlimited (per workspace) |

Rate limit headers:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1706635200
```

---

## Subscription Plan Limits

| Feature | Free | Personal ($4) | Pro ($9) | Team ($15/user) |
|---------|------|---------------|----------|-----------------|
| Max Tasks | 100 | Unlimited | Unlimited | Unlimited |
| Workspaces | 1 | 1 | 3 | Unlimited |
| Team Members | 1 | 1 | 5 | Unlimited |
| Priority Support | ❌ | ❌ | ✅ | ✅ |
| Custom Branding | ❌ | ❌ | ❌ | ✅ |
| Advanced Analytics | ❌ | ❌ | ✅ | ✅ |

API returns `403` with `{"error": "Feature locked behind Pro plan"}` when accessing paid features on free plan.

---

## Data Models

### Task Schema

```typescript
interface Task {
  id: number;
  workspace_id: number;
  title: string;          // Required, max 500 chars
  description: string;    // Optional
  status: 'pending' | 'in_progress' | 'completed';
  priority: 'low' | 'medium' | 'high' | 'urgent';
  assignee_id?: number;   // null if unassigned
  created_by: number;
  due_date?: string;      // RFC3339
  completed_at?: string;  // RFC3339, set when status=completed
  created_at: string;     // RFC3339
  updated_at: string;     // RFC3339
}
```

### Workspace Schema

```typescript
interface Workspace {
  id: number;
  name: string;
  description?: string;
  owner_id: number;
  plan_tier: 'free' | 'personal' | 'pro' | 'team';
  is_active: boolean;
  created_at: string;
}
```

### User Schema

```typescript
interface User {
  id: number;
  email: string;
  first_name?: string;
  last_name?: string;
  role: 'owner' | 'admin' | 'member';
  subscription_plan: 'free' | 'personal' | 'pro' | 'team';
  customer_id?: string;      // Stripe customer ID
  subscription_id?: string;  // Stripe subscription ID
  is_active: boolean;
  created_at: string;
}
```

---

## SDKs & Libraries

### JavaScript SDK (future)

```javascript
import { TodoPro } from '@todopro/sdk';

const client = new TodoPro({
  apiKey: 'your_api_key',
  workspaceId: 1
});

// Use SDK methods
const tasks = await client.tasks.list();
```

### Go SDK (future)

```go
import "github.com/todopro/go-sdk"

client := todopro.New("your_api_key")
tasks, _ := client.Tasks.List(context.Background(), &todopro.TaskListParams{
    WorkspaceID: 1,
})
```

---

## Webhooks (from TodoPro to your server)

For enterprise customers, we can send webhooks on:

- `task.created`
- `task.updated`
- `task.deleted`
- `user.joined_workspace`
- `subscription.created`
- `subscription.canceled`

Configure webhook URL in dashboard.

---

## Changelog

### v1.0.0 (Current)
- Initial SaaS launch
- JWT authentication
- Multi-workspace support
- Stripe subscriptions
- Real-time WebSocket
- Basic analytics

### Planned v1.1.0
- Email invitations
- Custom fields
- Recurring tasks
- Time tracking

### Planned v1.2.0
- Calendar sync
- Slack integration
- Advanced filtering
- Bulk operations

---

**Questions?** Open an issue on GitHub or email api@todopro.com
