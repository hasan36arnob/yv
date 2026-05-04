-- ========================================
-- TodoPro Bangladesh - Payment Schema Update
-- ========================================

-- Add payments table for manual payment verification
CREATE TABLE IF NOT EXISTS payments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id BIGINT REFERENCES workspaces(id) ON DELETE SET NULL,
    
    -- Payment details
    amount DECIMAL(10,2) NOT NULL CHECK (amount > 0),
    currency VARCHAR(3) DEFAULT 'BDT',
    method VARCHAR(20) NOT NULL CHECK (method IN ('bkash', 'nagad', 'rocket', 'bank_transfer')),
    transaction_id VARCHAR(255) UNIQUE NOT NULL,
    phone_number VARCHAR(20), -- Customer's phone used for payment
    
    -- Status tracking
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'refunded')),
    notes TEXT, -- Admin notes for rejection/approval
    
    -- Plan details (what they paid for)
    plan_type VARCHAR(50) NOT NULL CHECK (plan_type IN ('personal', 'pro', 'team')),
    duration_months INT DEFAULT 1, -- 1 for monthly, 12 for yearly
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    approved_at TIMESTAMP,
    approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL
);

-- Indexes for fast queries
CREATE INDEX idx_payments_user ON payments(user_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_transaction ON payments(transaction_id);
CREATE INDEX idx_payments_created ON payments(created_at DESC);
CREATE INDEX idx_payments_workspace ON payments(workspace_id);

-- Add plan_tier to workspace if not exists (already in main schema, but ensure)
-- ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS plan_tier VARCHAR(50) DEFAULT 'free';

-- ========================================
-- FUNCTIONS
-- ========================================

-- Function to get user's active subscription
CREATE OR REPLACE FUNCTION get_active_subscription(p_user_id BIGINT)
RETURNS TABLE (
    plan VARCHAR(50),
    expires_at TIMESTAMP,
    is_active BOOLEAN
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        p.plan_type as plan,
        (p.created_at + (p.duration_months * INTERVAL '1 month')) as expires_at,
        p.status = 'approved' 
            AND (p.created_at + (p.duration_months * INTERVAL '1 month')) > CURRENT_TIMESTAMP
            as is_active
    FROM payments p
    WHERE p.user_id = p_user_id
      AND p.status = 'approved'
    ORDER BY p.created_at DESC
    LIMIT 1;
END;
$$ LANGUAGE plpgsql;

-- Function to check if workspace can add more members
CREATE OR REPLACE FUNCTION check_workspace_limits(p_workspace_id BIGINT)
RETURNS JSON AS $$
DECLARE
    workspace_record RECORD;
    member_count INT;
    max_members INT;
BEGIN
    SELECT * INTO workspace_record FROM workspaces WHERE id = p_workspace_id;
    
    IF workspace_record.plan_tier = 'free' THEN
        max_members := 1;
    ELSIF workspace_record.plan_tier = 'personal' THEN
        max_members := 1;
    ELSIF workspace_record.plan_tier = 'pro' THEN
        max_members := 5;
    ELSIF workspace_record.plan_tier = 'team' THEN
        max_members := -1; -- unlimited
    ELSE
        max_members := 1;
    END IF;
    
    SELECT COUNT(*) INTO member_count 
    FROM team_members 
    WHERE workspace_id = p_workspace_id AND is_active = TRUE;
    
    RETURN json_build_object(
        'current_members', member_count,
        'max_members', max_members,
        'can_add', max_members = -1 OR member_count < max_members
    );
END;
$$ LANGUAGE plpgsql;

-- ========================================
-- TRIGGERS
-- ========================================

-- Auto-update workspace plan when payment approved
CREATE OR REPLACE FUNCTION update_workspace_plan_on_payment()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'approved' AND OLD.status != 'approved' THEN
        -- Update user's subscription plan
        UPDATE users 
        SET subscription_plan = NEW.plan_type 
        WHERE id = NEW.user_id;
        
        -- Update workspace plan if this payment is for a workspace
        IF NEW.workspace_id IS NOT NULL THEN
            UPDATE workspaces 
            SET plan_tier = NEW.plan_type 
            WHERE id = NEW.workspace_id;
        END IF;
        
        -- Log activity
        INSERT INTO activity_logs (workspace_id, user_id, action, entity_type, entity_id, changes)
        VALUES (
            COALESCE(NEW.workspace_id, 0),
            NEW.user_id,
            'payment_approved',
            'payment',
            NEW.id,
            jsonb_build_object('plan', NEW.plan_type, 'amount', NEW.amount)
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_payment_approved
    AFTER UPDATE ON payments
    FOR EACH ROW
    WHEN (OLD.status IS DISTINCT FROM NEW.status AND NEW.status = 'approved')
    EXECUTE FUNCTION update_workspace_plan_on_payment();

-- ========================================
-- VIEWS
-- ========================================

-- Payment summary view for admin
CREATE VIEW payment_summary AS
SELECT 
    p.id,
    p.amount,
    p.method,
    p.transaction_id,
    p.phone_number,
    p.status,
    p.plan_type,
    p.created_at,
    p.approved_at,
    u.id as user_id,
    u.email,
    u.first_name,
    u.last_name,
    w.id as workspace_id,
    w.name as workspace_name,
    CASE 
        WHEN p.status = 'approved' THEN '✅ Approved'
        WHEN p.status = 'pending' THEN '⏳ Pending'
        WHEN p.status = 'rejected' THEN '❌ Rejected'
        ELSE '🏦 Refunded'
    END as status_label
FROM payments p
JOIN users u ON u.id = p.user_id
LEFT JOIN workspaces w ON w.id = p.workspace_id
ORDER BY p.created_at DESC;

-- User payment history view
CREATE VIEW user_payment_history AS
SELECT 
    p.*,
    u.email as user_email,
    w.name as workspace_name
FROM payments p
JOIN users u ON u.id = p.user_id
LEFT JOIN workspaces w ON w.id = p.workspace_id
WHERE p.user_id = CURRENT_SETTING('app.current_user_id')::INT
ORDER BY p.created_at DESC;
