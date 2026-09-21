package bzapper

import (
	"context"
	"net/http"
)

// --- Billing: plan, entitlements, add-ons (cart), invoices, pricing ---

// AddonKind is a purchasable add-on.
type AddonKind string

const (
	AddonNumber         AddonKind = "number"
	AddonProject        AddonKind = "project"
	AddonStorageGB      AddonKind = "storage_gb"
	AddonRetentionBlock AddonKind = "retention_block"
	AddonCampaigns      AddonKind = "campaigns"
	AddonScheduleYear   AddonKind = "schedule_year"
)

// Entitlements is what the account can use now: plan, add-ons, limits and usage.
type Entitlements struct {
	Currency                 string `json:"currency,omitempty"`
	ProjectFreeCount         int    `json:"project_free_count,omitempty"`
	PlanCode                 string `json:"plan_code,omitempty"`
	PlanName                 string `json:"plan_name,omitempty"`
	Status                   string `json:"status,omitempty"`
	Gated                    bool   `json:"gated,omitempty"`
	Plan                     string `json:"plan,omitempty"`
	PlanRenewsAt             string `json:"plan_renews_at,omitempty"`
	PlanMonthlyCents         int64  `json:"plan_monthly_cents,omitempty"`
	PlanCancelAt             string `json:"plan_cancel_at,omitempty"`
	AddonNumbers             int    `json:"addon_numbers,omitempty"`
	AddonNumbersNext         int    `json:"addon_numbers_next,omitempty"`
	AddonProjects            int    `json:"addon_projects,omitempty"`
	AddonProjectsNext        int    `json:"addon_projects_next,omitempty"`
	AddonStorageGB           int    `json:"addon_storage_gb,omitempty"`
	AddonStorageGBNext       int    `json:"addon_storage_gb_next,omitempty"`
	AddonRetentionBlocks     int    `json:"addon_retention_blocks,omitempty"`
	AddonRetentionBlocksNext int    `json:"addon_retention_blocks_next,omitempty"`
	AddonNumberCents         int64  `json:"addon_number_cents,omitempty"`
	AddonProjectCents        int64  `json:"addon_project_cents,omitempty"`
	AddonStorageGBCents      int64  `json:"addon_storage_gb_cents,omitempty"`
	AddonRetentionBlockCents int64  `json:"addon_retention_block_cents,omitempty"`
	MaxProjects              int    `json:"max_projects,omitempty"`
	MaxNumbersPerProject     int    `json:"max_numbers_per_project,omitempty"`
	MaxUsers                 int    `json:"max_users,omitempty"`
	MaxAPIKeys               int    `json:"max_api_keys,omitempty"`
	StorageMB                int    `json:"storage_mb,omitempty"`
	MediaRetentionDays       int    `json:"media_retention_days,omitempty"`
	MessageRetentionDays     int    `json:"message_retention_days,omitempty"`
	RateLimitRPS             int    `json:"rate_limit_rps,omitempty"`
	SendsIncluded            int64  `json:"sends_included,omitempty"`
	SendsUsed                int64  `json:"sends_used,omitempty"`
	MessagesUsed             int64  `json:"messages_used,omitempty"`
	MessageFreeCount         int    `json:"message_free_count,omitempty"`
	NumberFreeCount          int    `json:"number_free_count,omitempty"`
	StorageFreeMB            int    `json:"storage_free_mb,omitempty"`
	RetentionFreeDays        int    `json:"retention_free_days,omitempty"`
}

// AddonCart is the pending add-on cart (paid in one Checkout).
type AddonCart struct {
	PlanPro         bool   `json:"plan_pro,omitempty"`
	ProMonthlyCents int64  `json:"pro_monthly_cents,omitempty"`
	Numbers         int    `json:"numbers,omitempty"`
	Projects        int    `json:"projects,omitempty"`
	StorageGB       int    `json:"storage_gb,omitempty"`
	RetentionBlocks int    `json:"retention_blocks,omitempty"`
	Campaigns       int    `json:"campaigns,omitempty"`
	ScheduleYear    int    `json:"schedule_year,omitempty"`
	ProratedCents   int64  `json:"prorated_cents,omitempty"`
	Currency        string `json:"currency,omitempty"`
	Empty           bool   `json:"empty,omitempty"`
}

// PlanSummary is the current subscription (GetMySubscription).
type PlanSummary struct {
	Plan       string `json:"plan,omitempty"`
	Status     string `json:"status,omitempty"` // active, past_due, grace, canceling
	RenewsAt   string `json:"renews_at,omitempty"`
	CancelAt   string `json:"cancel_at,omitempty"`
	GraceUntil string `json:"grace_until,omitempty"`
	Recurring  bool   `json:"recurring,omitempty"`
	Currency   string `json:"currency,omitempty"`
}

// InvoiceItem is a line of an Invoice.
type InvoiceItem struct {
	Kind            string `json:"kind,omitempty"` // plan | addon
	AddonKind       string `json:"addon_kind,omitempty"`
	Description     string `json:"description,omitempty"`
	Qty             int    `json:"qty,omitempty"`
	UnitAmountCents int64  `json:"unit_amount_cents,omitempty"`
	AmountCents     int64  `json:"amount_cents,omitempty"`
	Proration       bool   `json:"proration,omitempty"`
	PeriodStart     string `json:"period_start,omitempty"`
	PeriodEnd       string `json:"period_end,omitempty"`
}

// Invoice is an account invoice.
type Invoice struct {
	ID            string        `json:"id,omitempty"`
	Number        int64         `json:"number,omitempty"`
	Currency      string        `json:"currency,omitempty"`
	Status        string        `json:"status,omitempty"` // open, awaiting_payment, paid, failed, void
	Reason        string        `json:"reason,omitempty"` // initial, addon, renewal
	Recurring     bool          `json:"recurring,omitempty"`
	SubtotalCents int64         `json:"subtotal_cents,omitempty"`
	TotalCents    int64         `json:"total_cents,omitempty"`
	PeriodStart   string        `json:"period_start,omitempty"`
	PeriodEnd     string        `json:"period_end,omitempty"`
	IssueDate     string        `json:"issue_date,omitempty"`
	DueDate       string        `json:"due_date,omitempty"`
	PaidAt        string        `json:"paid_at,omitempty"`
	Items         []InvoiceItem `json:"items,omitempty"`
}

// InvoiceList is the response of ListMyInvoices.
type InvoiceList struct {
	Data []Invoice `json:"data"`
}

// PlanLimits are the limits of a plan in Pricing.Plans.
type PlanLimits struct {
	Numbers           int  `json:"numbers,omitempty"`
	Projects          int  `json:"projects,omitempty"`
	Messages          int  `json:"messages,omitempty"`
	StorageMB         int  `json:"storage_mb,omitempty"`
	RetentionDays     int  `json:"retention_days,omitempty"`
	UnlimitedMessages bool `json:"unlimited_messages,omitempty"`
}

// Pricing is the public price list (GetPricing).
type Pricing struct {
	RetentionFreeDays int                   `json:"retention_free_days,omitempty"`
	Plans             map[string]PlanLimits `json:"plans,omitempty"`
	// Currencies: currency → price key → cents.
	Currencies map[string]map[string]int64 `json:"currencies,omitempty"`
}

// BillingConfig is the public billing configuration (GetBillingConfig).
type BillingConfig struct {
	PublishableKey string `json:"publishable_key,omitempty"`
	Enabled        bool   `json:"enabled,omitempty"`
}

// ChangeAddonParams adds (Delta > 0) or removes (Delta < 0) add-on units in the
// cart.
type ChangeAddonParams struct {
	Kind  AddonKind `json:"kind"`
	Delta int       `json:"delta"`
}

// CheckoutAddonCartParams is the request for CheckoutAddonCart.
type CheckoutAddonCartParams struct {
	SaveCard bool `json:"save_card,omitempty"`
}

// CheckoutResult is the payment intent of a checkout (confirm it with Stripe.js).
type CheckoutResult struct {
	ClientSecret string `json:"client_secret,omitempty"`
	InvoiceID    string `json:"invoice_id,omitempty"`
}

// GetMyEntitlements returns plan, add-ons, limits and usage. GET /me/entitlements.
func (c *Client) GetMyEntitlements(ctx context.Context) (*Entitlements, error) {
	return call[Entitlements](ctx, c, apiRequest{method: http.MethodGet, path: "/me/entitlements"})
}

// GetMySubscription returns the current subscription. GET /me/subscription.
func (c *Client) GetMySubscription(ctx context.Context) (*PlanSummary, error) {
	return call[PlanSummary](ctx, c, apiRequest{method: http.MethodGet, path: "/me/subscription"})
}

// UpgradePlan puts the Pro plan in the cart. POST /me/plan/upgrade.
func (c *Client) UpgradePlan(ctx context.Context) (*AddonCart, error) {
	return call[AddonCart](ctx, c, apiRequest{method: http.MethodPost, path: "/me/plan/upgrade"})
}

// CancelPlan schedules the plan cancellation at the period end.
// POST /me/plan/cancel.
func (c *Client) CancelPlan(ctx context.Context) (*Entitlements, error) {
	return call[Entitlements](ctx, c, apiRequest{method: http.MethodPost, path: "/me/plan/cancel"})
}

// UncancelPlan undoes a scheduled cancellation. POST /me/plan/uncancel.
func (c *Client) UncancelPlan(ctx context.Context) (*Entitlements, error) {
	return call[Entitlements](ctx, c, apiRequest{method: http.MethodPost, path: "/me/plan/uncancel"})
}

// ChangeAddon changes add-on units in the cart. POST /me/addons.
func (c *Client) ChangeAddon(ctx context.Context, p ChangeAddonParams) (*AddonCart, error) {
	return call[AddonCart](ctx, c, apiRequest{method: http.MethodPost, path: "/me/addons", body: p})
}

// GetAddonCart returns the pending cart. GET /me/addons/cart.
func (c *Client) GetAddonCart(ctx context.Context) (*AddonCart, error) {
	return call[AddonCart](ctx, c, apiRequest{method: http.MethodGet, path: "/me/addons/cart"})
}

// ClearAddonCart empties the cart. DELETE /me/addons/cart.
func (c *Client) ClearAddonCart(ctx context.Context) (*AddonCart, error) {
	return call[AddonCart](ctx, c, apiRequest{method: http.MethodDelete, path: "/me/addons/cart"})
}

// CheckoutAddonCart creates the payment of the cart (add-ons are only
// effective after paying). POST /me/addons/cart/checkout.
func (c *Client) CheckoutAddonCart(ctx context.Context, p CheckoutAddonCartParams) (*CheckoutResult, error) {
	return call[CheckoutResult](ctx, c, apiRequest{method: http.MethodPost, path: "/me/addons/cart/checkout", body: p})
}

// ListMyInvoices lists the account invoices. GET /me/invoices.
func (c *Client) ListMyInvoices(ctx context.Context) (*InvoiceList, error) {
	return call[InvoiceList](ctx, c, apiRequest{method: http.MethodGet, path: "/me/invoices"})
}

// PayInvoice creates the payment of an open invoice. POST /me/invoices/{id}/pay.
func (c *Client) PayInvoice(ctx context.Context, id string) (*CheckoutResult, error) {
	return call[CheckoutResult](ctx, c, apiRequest{method: http.MethodPost, path: "/me/invoices/" + seg(id) + "/pay"})
}

// GetBillingConfig returns the public billing configuration. GET /billing/config.
func (c *Client) GetBillingConfig(ctx context.Context) (*BillingConfig, error) {
	return call[BillingConfig](ctx, c, apiRequest{method: http.MethodGet, path: "/billing/config"})
}

// GetPricing returns the public price list. GET /pricing.
func (c *Client) GetPricing(ctx context.Context) (*Pricing, error) {
	return call[Pricing](ctx, c, apiRequest{method: http.MethodGet, path: "/pricing"})
}
