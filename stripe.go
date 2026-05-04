package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/customer"
	"github.com/stripe/stripe-go/v76/sub"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Plan pricing (in cents)
var planPrices = map[string]int64{
	"personal": 400,   // $4/month
	"pro":      900,   // $9/month  
	"team":     1500,  // $15/month per user
}

var planLimits = map[string]map[string]int{
	"personal": {
		"max_tasks":     1000,
		"max_workspaces": 1,
		"max_members":   1,
	},
	"pro": {
		"max_tasks":     5000,
		"max_workspaces": 3,
		"max_members":   5,
	},
	"team": {
		"max_tasks":     -1, // Unlimited
		"max_workspaces": -1,
		"max_members":   -1, // Unlimited
	},
}

// Initialize Stripe client
func initStripe() {
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	if stripe.Key == "" {
		log.Println("Warning: STRIPE_SECRET_KEY not set, using test mode")
		stripe.Key = "sk_test_placeholder"
	}
}

// CreateCheckoutSession creates a Stripe checkout session
func CreateCheckoutSession(db *gorm.DB, userID uint, planID string) (string, string, error) {
	// Validate plan
	if _, ok := planPrices[planID]; !ok {
		return "", "", fmt.Errorf("invalid plan: %s", planID)
	}

	// Get user
	var user User
	if err := db.First(&user, userID).Error; err != nil {
		return "", "", fmt.Errorf("user not found")
	}

	// Create or get Stripe customer
	customerID, err := getOrCreateStripeCustomer(&user)
	if err != nil {
		return "", "", fmt.Errorf("failed to create customer: %v", err)
	}

	// Create checkout session
	params := &stripe.CheckoutSessionParams{
		Customer:    stripe.String(customerID),
		PaymentMethodTypes: stripe.StringSlice([]string{stripe.CheckoutSessionCreateParamsPaymentMethodTypesCard}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:    stripe.String("usd"),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String(fmt.Sprintf("TodoPro %s Plan", strings.Title(planID))),
					},
					UnitAmount: stripe.Int64(planPrices[planID]),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:              stripe.String("subscription"),
		SuccessURL:        stripe.String(fmt.Sprintf("%s/success?session_id={CHECKOUT_SESSION_ID}", getAppURL())),
		CancelURL:         stripe.String(fmt.Sprintf("%s/pricing", getAppURL())),
		AllowPromotionCodes: stripe.Bool(true),
		Metadata: map[string]string{
			"user_id":    fmt.Sprintf("%d", userID),
			"plan":       planID,
			"workspace":  "main", // For individual plans
		},
	}

	s, err := session.New(params)
	if err != nil {
		return "", "", fmt.Errorf("failed to create session: %v", err)
	}

	return s.ID, s.URL, nil
}

// GetOrCreateStripeCustomer finds or creates a Stripe customer
func getOrCreateStripeCustomer(user *User) (string, error) {
	if user.StripeCustomerID != "" {
		// Verify customer exists in Stripe
		cust, err := customer.Get(user.StripeCustomerID, nil)
		if err == nil && cust != nil {
			return user.StripeCustomerID, nil
		}
	}

	// Create new customer
	params := &stripe.CustomerParams{
		Email: stripe.String(user.Email),
		Name:  stripe.String(fmt.Sprintf("%s %s", user.FirstName, user.LastName)),
		Metadata: map[string]string{
			"user_id": fmt.Sprintf("%d", user.ID),
			"plan":    user.SubscriptionPlan,
		},
	}

	cust, err := customer.New(params)
	if err != nil {
		return "", err
	}

	// Update user with Stripe customer ID
	db.Model(user).Update("stripe_customer_id", cust.ID)

	return cust.ID, nil
}

// ProcessWebhook handles Stripe webhook events
func ProcessWebhook(payload []byte, signature string) error {
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if webhookSecret == "" {
		return fmt.Errorf("webhook secret not configured")
	}

	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		return fmt.Errorf("failed to construct event: %v", err)
	}

	// Store event for audit
	storeStripeEvent(event)

	// Process based on event type
	switch event.Type {
	case "checkout.session.completed":
		handleCheckoutCompleted(event.Data.Object)
	case "customer.subscription.updated":
		handleSubscriptionUpdated(event.Data.Object)
	case "customer.subscription.deleted":
		handleSubscriptionDeleted(event.Data.Object)
	case "invoice.payment_failed":
		handlePaymentFailed(event.Data.Object)
	case "customer.subscription.trial_will_end":
		handleTrialEnding(event.Data.Object)
	}

	return nil
}

func handleCheckoutCompleted(obj map[string]interface{}) {
	session := obj["session"].(map[string]interface{})
	customerID := session["customer"].(string)
	subscriptionID := session["subscription"].(string)
	metadata := session["metadata"].(map[string]interface{})
	
	userID := uint(parseInt(metadata["user_id"]))
	plan := metadata["plan"]

	// Create subscription record
	sub := &Subscription{
		UserID:            userID,
		Plan:              plan,
		StripeSubID:       subscriptionID,
		StripeCustomerID:  customerID,
		Status:            "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:  time.Now().Add(30 * 24 * time.Hour),
	}
	db.Create(sub)

	// Update user plan
	db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"subscription_plan":      plan,
		"stripe_subscription_id": subscriptionID,
	})
}

func handleSubscriptionUpdated(obj map[string]interface{}) {
	subscription := obj["id"].(string)
	var sub Subscription
	if err := db.Where("stripe_subscription_id = ?", subscription).First(&sub).Error; err != nil {
		return
	}

	stripeSub := obj["object"].(map[string]interface{})
	sub.Status = stripeSub["status"].(string)
	sub.CurrentPeriodStart = parseTime(stripeSub["current_period_start"].(float64))
	sub.CurrentPeriodEnd = parseTime(stripeSub["current_period_end"].(float64))
	db.Save(&sub)

	// Update user plan
	if sub.Status != "active" {
		db.Model(&User{}).Where("id = ?", sub.UserID).Update("subscription_plan", "free")
	}
}

func handleSubscriptionDeleted(obj map[string]interface{}) {
	subscriptionID := obj["id"].(string)
	var sub Subscription
	if err := db.Where("stripe_subscription_id = ?", subscriptionID).First(&sub).Error; err != nil {
		return
	}

	sub.Status = "canceled"
	db.Save(&sub)

	// Downgrade user to free
	db.Model(&User{}).Where("id = ?", sub.UserID).Update("subscription_plan", "free")
}

func handlePaymentFailed(obj map[string]interface{}) {
	// Send email notification to user
	// Log for manual follow-up
	log.Printf("Payment failed for subscription: %v", obj["id"])
}

func handleTrialEnding(obj map[string]interface{}) {
	// Send reminder email to user
	log.Printf("Trial ending soon for subscription: %v", obj["id"])
}

// StoreStripeEvent stores webhook event for audit
func storeStripeEvent(event stripe.Event) {
	eventData, _ := json.Marshal(event.Data.Object)
	stripeEvent := StripeEvent{
		EventID: event.ID,
		Type:    event.Type,
		Data:    eventData,
	}
	db.Create(&stripeEvent)
}

// CheckSubscriptionAccess checks if user has access to a feature based on plan
func CheckSubscriptionAccess(user *User, feature string) bool {
	limits, ok := planLimits[user.SubscriptionPlan]
	if !ok {
		return false
	}

	switch feature {
	case "unlimited_tasks":
		return limits["max_tasks"] == -1
	case "team_collaboration":
		return limits["max_members"] > 1
	case "custom_branding":
		return user.SubscriptionPlan == "team"
	case "advanced_analytics":
		return user.SubscriptionPlan == "pro" || user.SubscriptionPlan == "team"
	case "priority_support":
		return user.SubscriptionPlan == "pro" || user.SubscriptionPlan == "team"
	default:
		return true
	}
}

// CheckWorkspaceLimit checks if user can add more members to workspace
func CheckWorkspaceLimit(workspace *Workspace, newMemberCount int) bool {
	limits, ok := planLimits[workspace.PlanTier]
	if !ok || limits["max_members"] == -1 {
		return true // Unlimited
	}

	var currentCount int64
	db.Model(&TeamMember{}).Where("workspace_id = ? AND is_active = ?", workspace.ID, true).Count(&currentCount)
	return int(currentCount)+newMemberCount <= limits["max_members"]
}

// UpgradeWorkspace upgrades a workspace to a paid plan
func UpgradeWorkspace(workspace *Workspace, planID string, userID uint) error {
	// Create checkout session
	_, _, err := CreateCheckoutSession(db, userID, planID)
	return err
}

// CancelSubscription cancels a user's subscription
func CancelSubscription(userID uint) error {
	var sub Subscription
	if err := db.Where("user_id = ?", userID).First(&sub).Error; err != nil {
		return err
	}

	// Cancel at period end
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(true),
	}
	_, err := sub.Update(params)
	if err != nil {
		return err
	}

	sub.CancelAtPeriodEnd = true
	db.Save(&sub)

	return nil
}

// GetSubscriptionStatus retrieves user subscription
func GetSubscriptionStatus(userID uint) (*Subscription, error) {
	var sub Subscription
	err := db.Where("user_id = ?", userID).First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// GetActivePlan returns user's current plan with limits
func GetActivePlan(userID uint) map[string]interface{} {
	var user User
	if err := db.First(&user, userID).Error; err != nil {
		return nil
	}

	limits := planLimits[user.SubscriptionPlan]
	if limits == nil {
		limits = planLimits["free"]
	}

	return map[string]interface{}{
		"plan":      user.SubscriptionPlan,
		"limits":    limits,
		"customer":  user.CustomerID,
		"active":    user.SubscriptionPlan != "free" && user.IsActive,
	}
}

// Helper functions
func getAppURL() string {
	url := os.Getenv("APP_URL")
	if url == "" {
		url = "http://localhost:5000"
	}
	return url
}

func parseInt(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}

func parseTime(ts float64) time.Time {
	return time.Unix(int64(ts), 0)
}

// --- Dashboard Analytics Functions ---

// GetTeamProductivity returns productivity metrics for all team members
func GetTeamProductivity(workspaceID uint) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	
	query := `
		SELECT 
			u.id as user_id,
			u.first_name,
			u.last_name,
			u.email,
			u.subscription_plan,
			COUNT(t.id) as total_tasks,
			COUNT(CASE WHEN t.status = 'completed' THEN 1 END) as completed_tasks,
			COUNT(CASE WHEN t.status = 'in_progress' THEN 1 END) as in_progress_tasks,
			COUNT(CASE WHEN t.status = 'pending' THEN 1 END) as pending_tasks,
			MAX(t.completed_at) as last_completed_at,
			AVG(EXTRACT(EPOCH FROM (t.completed_at - t.created_at))/3600) as avg_completion_hours
		FROM users u
		INNER JOIN team_members tm ON tm.user_id = u.id
		LEFT JOIN tasks t ON t.assignee_id = u.id AND t.workspace_id = ?
		WHERE tm.workspace_id = ? AND tm.is_active = true
		GROUP BY u.id, u.first_name, u.last_name, u.email, u.subscription_plan
		ORDER BY completed_tasks DESC
	`

	db.Raw(query, workspaceID, workspaceID).Scan(&results)
	return results, nil
}

// GetCompletionTrend returns daily completion trend
func GetCompletionTrend(workspaceID uint, days int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	
	query := `
		SELECT 
			DATE(created_at) as date,
			COUNT(*) as created_count,
			COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed_count,
			COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending_count
		FROM tasks
		WHERE workspace_id = ? 
			AND created_at >= CURRENT_DATE - INTERVAL '? days'
		GROUP BY DATE(created_at)
		ORDER BY date ASC
	`

	db.Raw(query, workspaceID, days).Scan(&results)
	return results, nil
}

// GetPriorityDistribution returns task breakdown by priority
func GetPriorityDistribution(workspaceID uint) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	
	query := `
		SELECT 
			priority,
			COUNT(*) as count,
			COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed
		FROM tasks
		WHERE workspace_id = ?
		GROUP BY priority
		ORDER BY 
			CASE priority 
				WHEN 'urgent' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 4
			END
	`

	db.Raw(query, workspaceID).Scan(&results)
	return results, nil
}

// GetOverdueTasks returns list of overdue tasks
func GetOverdueTasks(workspaceID uint) ([]Task, error) {
	var tasks []Task
	now := time.Now()
	
	err := db.Where("workspace_id = ? AND status != ? AND due_date < ?", 
		workspaceID, "completed", now).Preload("Assignee").Find(&tasks).Error
	return tasks, err
}
