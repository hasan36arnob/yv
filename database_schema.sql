-- ========================================
-- TodoPro SaaS Database Schema
-- PostgreSQL 12+
-- ========================================

-- Enable UUID extension if needed for future features
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ========================================
-- USERS TABLE
-- ========================================
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    role VARCHAR(50) DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    subscription_plan VARCHAR(50) DEFAULT 'free' CHECK (subscription_plan IN ('free', 'personal', 'pro', 'team')),
    stripe_customer_id VARCHAR(255) UNIQUE,
    stripe_subscription_id VARCHAR(255) UNIQUE,
    is_active BOOLEAN DEFAULT TRUE,
    last_login_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_subscription ON users(subscription_plan, is_active);

-- ========================================
-- WORKSPACES TABLE
-- ========================================
CREATE TABLE workspaces (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_tier VARCHAR(50) DEFAULT 'free' CHECK (plan_tier IN ('free', 'personal', 'pro', 'team')),
    is_active BOOLEAN DEFAULT TRUE,
    max_members INT DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_workspaces_owner ON workspaces(owner_id);
CREATE INDEX idx_workspaces_plan ON workspaces(plan_tier);

-- ========================================
-- TEAM MEMBERS TABLE
-- ========================================
CREATE TABLE team_members (
    id BIGSERIAL PRIMARY KEY,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    is_active BOOLEAN DEFAULT TRUE,
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workspace_id, user_id)
);

CREATE INDEX idx_team_members_workspace ON team_members(workspace_id);
CREATE INDEX idx_team_members_user ON team_members(user_id);
CREATE INDEX idx_team_members_role ON team_members(workspace_id, role);

-- ========================================
-- TASKS TABLE
-- ========================================
CREATE TABLE tasks (
    id BIGSERIAL PRIMARY KEY,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    status VARCHAR(50) DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed')),
    priority VARCHAR(50) DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    assignee_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    due_date TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tasks_workspace ON tasks(workspace_id);
CREATE INDEX idx_tasks_assignee ON tasks(assignee_id);
CREATE INDEX idx_tasks_status ON tasks(workspace_id, status);
CREATE INDEX idx_tasks_due_date ON tasks(due_date);
CREATE INDEX idx_tasks_created_by ON tasks(created_by);

-- ========================================
-- SUBSCRIPTIONS TABLE
-- ========================================
CREATE TABLE subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    plan VARCHAR(50) NOT NULL CHECK (plan IN ('personal', 'pro', 'team')),
    stripe_subscription_id VARCHAR(255) UNIQUE NOT NULL,
    stripe_customer_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'active' CHECK (status IN ('active', 'canceled', 'past_due', 'unpaid', 'incomplete')),
    current_period_start TIMESTAMP NOT NULL,
    current_period_end TIMESTAMP NOT NULL,
    cancel_at_period_end BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_subscriptions_user ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);
CREATE INDEX idx_subscriptions_stripe ON subscriptions(stripe_subscription_id);

-- ========================================
-- STRIPE EVENTS TABLE (for audit & debugging)
-- ========================================
CREATE TABLE stripe_events (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(255) UNIQUE NOT NULL,
    type VARCHAR(100) NOT NULL,
    data JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_stripe_events_id ON stripe_events(event_id);
CREATE INDEX idx_stripe_events_type ON stripe_events(type);
CREATE INDEX idx_stripe_events_processed ON stripe_events(processed, created_at);

-- ========================================
-- ACTIVITY LOG TABLE (for audit trail)
-- ========================================
CREATE TABLE activity_logs (
    id BIGSERIAL PRIMARY KEY,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(50) NOT NULL, -- 'task', 'workspace', 'member'
    entity_id BIGINT,
    changes JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_activity_logs_workspace ON activity_logs(workspace_id);
CREATE INDEX idx_activity_logs_user ON activity_logs(user_id);
CREATE INDEX idx_activity_logs_created ON activity_logs(created_at DESC);

-- ========================================
-- FEATURE FLAGS TABLE (for gradual feature rollouts)
-- ========================================
CREATE TABLE feature_flags (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT FALSE,
    rollout_percentage INT DEFAULT 0 CHECK (rollout_percentage >= 0 AND rollout_percentage <= 100),
    plan_filter VARCHAR(255), -- comma-separated plan names
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO feature_flags (name, description, enabled, rollout_percentage) VALUES
('advanced_analytics', 'Show advanced analytics dashboard', true, 100),
('custom_branding', 'Allow custom branding in Team plan', true, 100),
('api_access', 'Enable API access for Pro/Team', true, 100),
('priority_support', 'Priority support ticket system', true, 100),
('automation_rules', 'Custom automation rules', false, 0);

-- ========================================
-- TRIGGERS
-- ========================================

-- Update updated_at timestamp automatically
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_workspaces_updated_at BEFORE UPDATE ON workspaces
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_tasks_updated_at BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_subscriptions_updated_at BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ========================================
-- FUNCTIONS
-- ========================================

-- Get workspace statistics
CREATE OR REPLACE FUNCTION get_workspace_stats(p_workspace_id BIGINT)
RETURNS JSON AS $$
DECLARE
    result JSON;
    total INT;
    completed INT;
    pending INT;
    overdue INT;
    completion_rate DECIMAL(5,2);
BEGIN
    SELECT COUNT(*) INTO total FROM tasks WHERE workspace_id = p_workspace_id;
    SELECT COUNT(*) INTO completed FROM tasks WHERE workspace_id = p_workspace_id AND status = 'completed';
    SELECT COUNT(*) INTO pending FROM tasks WHERE workspace_id = p_workspace_id AND status = 'pending';
    SELECT COUNT(*) INTO overdue FROM tasks 
    WHERE workspace_id = p_workspace_id 
    AND status != 'completed' 
    AND due_date < CURRENT_TIMESTAMP;
    
    IF total > 0 THEN
        completion_rate := ROUND((completed::DECIMAL / total::DECIMAL) * 100, 2);
    ELSE
        completion_rate := 0;
    END IF;
    
    result := json_build_object(
        'total', total,
        'completed', completed,
        'pending', pending,
        'overdue', overdue,
        'completion_rate', completion_rate
    );
    
    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- Get user productivity stats
CREATE OR REPLACE FUNCTION get_user_productivity(p_workspace_id BIGINT, p_user_id INT DEFAULT NULL)
RETURNS JSON AS $$
DECLARE
    result JSON;
    rows RECORD;
    json_array JSON DEFAULT '[]'::JSON;
BEGIN
    FOR rows IN 
        SELECT 
            u.id as user_id,
            u.first_name,
            u.last_name,
            u.email,
            COUNT(t.id) as total_tasks,
            COUNT(CASE WHEN t.status = 'completed' THEN 1 END) as completed_tasks,
            COUNT(CASE WHEN t.status = 'in_progress' THEN 1 END) as in_progress_tasks,
            COUNT(CASE WHEN t.status = 'pending' THEN 1 END) as pending_tasks,
            MAX(t.completed_at) as last_completed
        FROM users u
        LEFT JOIN tasks t ON t.assignee_id = u.id AND t.workspace_id = p_workspace_id
        INNER JOIN team_members tm ON tm.user_id = u.id AND tm.workspace_id = p_workspace_id
        WHERE tm.is_active = TRUE
        AND (p_user_id IS NULL OR u.id = p_user_id)
        GROUP BY u.id, u.first_name, u.last_name, u.email
        ORDER BY completed_tasks DESC
    LOOP
        json_array := json_array || json_build_object(
            'user_id', rows.user_id,
            'name', rows.first_name || ' ' || rows.last_name,
            'email', rows.email,
            'total', rows.total_tasks,
            'completed', rows.completed_tasks,
            'in_progress', rows.in_progress_tasks,
            'pending', rows.pending_tasks,
            'last_completed', rows.last_completed
        );
    END LOOP;
    
    RETURN json_array;
END;
$$ LANGUAGE plpgsql;

-- ========================================
-- VIEWS
-- ========================================

-- Active workspace members view
CREATE VIEW active_workspace_members AS
SELECT 
    w.id as workspace_id,
    w.name as workspace_name,
    u.id as user_id,
    u.email,
    u.first_name,
    u.last_name,
    u.subscription_plan,
    tm.role,
    tm.joined_at
FROM workspaces w
INNER JOIN team_members tm ON tm.workspace_id = w.id AND tm.is_active = TRUE
INNER JOIN users u ON u.id = tm.user_id AND u.is_active = TRUE
WHERE w.is_active = TRUE;

-- Task completion metrics view
CREATE VIEW task_completion_metrics AS
SELECT 
    workspace_id,
    DATE(created_at) as date,
    COUNT(*) as tasks_created,
    COUNT(CASE WHEN status = 'completed' THEN 1 END) as tasks_completed,
    COUNT(CASE WHEN status = 'completed' AND DATE(completed_at) = DATE(created_at) THEN 1 END) as same_day_completion
FROM tasks
WHERE created_at >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY workspace_id, DATE(created_at)
ORDER BY workspace_id, date;

-- ========================================
-- ROW LEVEL SECURITY (optional, for multi-tenant isolation)
-- ========================================
-- ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
-- CREATE POLICY task_isolation ON tasks 
--     USING (workspace_id IN (
--         SELECT workspace_id FROM team_members 
--         WHERE user_id = current_setting('app.current_user_id')::INT
--     ));

-- ========================================
-- SAMPLE DATA (for development)
-- ========================================

-- Insert demo user (password: "password123" hashed)
-- INSERT INTO users (email, password_hash, first_name, last_name, subscription_plan)
-- VALUES (
--     'demo@todopro.com',
--     '$2a$10$N.zmdr9k7UOCG3aZHWeC.uJVxW36aZbfkJOG5c1bJO7sVo5S8sMPm',
--     'Demo',
--     'User',
--     'pro'
-- );
