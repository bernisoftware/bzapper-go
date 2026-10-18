package bzapper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// This file covers the OFFICIAL rail — a project that sends through Meta's
// WhatsApp Cloud API instead of a paired number.
//
// The send verbs are the same on both rails: SendText is SendText. What lives
// here is what only exists when Meta is the carrier — approved templates, the
// 24-hour service window, Embedded Signup, the numbers of a WhatsApp Business
// Account and their business profile. GetOfficialAccount,
// ConnectOfficialAccount and DisconnectOfficialAccount live in pools.go.

// --- templates ----------------------------------------------------------------

// TemplateHeader is the optional header of a template. A text header takes at
// most ONE variable; a media header needs a sample file, which Meta wants to see
// before approving.
type TemplateHeader struct {
	// Type is text, image, video or document.
	Type string `json:"type"`
	// Text is the text header, up to 60 characters.
	Text string `json:"text,omitempty"`
	// Example is the sample value for the header variable. Meta rejects a
	// variable with no example.
	Example string `json:"example,omitempty"`
	// MediaURL is a public URL to the SAMPLE file, for media headers — or the
	// URL returned by UploadTemplateMedia, so you do not host it anywhere.
	MediaURL string `json:"media_url,omitempty"`
}

// TemplateBody is the body — the only required component.
type TemplateBody struct {
	// Text uses {{1}}, {{2}}… for the values that change, numbered from 1 with
	// no gaps (up to 1024 characters). It cannot start or end with a variable,
	// and two variables cannot sit side by side: Meta rejects all three, and
	// the API checks before submitting.
	Text string `json:"text"`
	// Example carries one sample value per variable, in order. Omitting these is
	// the most common reason for rejection.
	Example []string `json:"example,omitempty"`
}

// TemplateButton is a button of the template.
type TemplateButton struct {
	// Type is quick_reply, url or phone_number.
	Type string `json:"type"`
	// Text is the button label, up to 25 characters.
	Text string `json:"text"`
	// URL is for url buttons (up to 2 per template); it may end with a {{1}}.
	URL string `json:"url,omitempty"`
	// Phone is for phone_number buttons (at most 1 per template).
	Phone string `json:"phone,omitempty"`
	// Example is the filled-in sample URL, required when the url has a variable.
	Example string `json:"example,omitempty"`
}

// TemplateSpec is the simple shape you fill in (CreateTemplate). The API turns
// it into Meta's component array — the nesting, the example format and the
// unwritten rules are not your problem.
type TemplateSpec struct {
	// Name takes lowercase letters, numbers and _. It is the name you send by.
	Name string `json:"name"`
	// Language is e.g. pt_BR (pt-BR is accepted too).
	Language string `json:"language"`
	// Category is MARKETING, UTILITY or AUTHENTICATION. It decides what the
	// conversation COSTS and what Meta accepts in the content — it is not an
	// administrative label. Promotional wording in a UTILITY template gets
	// reclassified, and the price changes with it.
	Category string          `json:"category"`
	Header   *TemplateHeader `json:"header,omitempty"`
	Body     TemplateBody    `json:"body"`
	// Footer is up to 60 characters and accepts no variables.
	Footer string `json:"footer,omitempty"`
	// Buttons are at most 10.
	Buttons []TemplateButton `json:"buttons,omitempty"`
	// CodeExpiryMinutes applies to AUTHENTICATION only (1 to 90). In that
	// category Meta writes the text itself and adds the copy-code button; this
	// is the only thing you set.
	CodeExpiryMinutes int `json:"code_expiry_minutes,omitempty"`
	// LimitedTimeOffer (MARKETING) turns the template into a limited-time
	// offer: a badge of up to 16 characters and, with HasExpiration, a
	// countdown to the deadline sent in SendTemplateParams.OfferExpiresAt.
	LimitedTimeOffer *TemplateOffer `json:"limited_time_offer,omitempty"`
	// Cards (MARKETING or UTILITY, 2 to 10) turns it into a carousel: the
	// template keeps only the opening Body; each card has its own media, text
	// and 1–2 buttons, with the same media type and button types on all cards.
	Cards []TemplateCard `json:"cards,omitempty"`
}

// TemplateOffer is the limited-time offer part of a template.
type TemplateOffer struct {
	Text          string `json:"text"`
	HasExpiration bool   `json:"has_expiration"`
}

// TemplateCard is one card of a carousel template.
type TemplateCard struct {
	Header  *TemplateHeader  `json:"header"`
	Body    TemplateBody     `json:"body"`
	Buttons []TemplateButton `json:"buttons,omitempty"`
}

// TemplateCardValues are the values of one carousel card on a send.
type TemplateCardValues struct {
	// HeaderURL is the media of the card for this send (empty: the sample).
	HeaderURL       string   `json:"header_url,omitempty"`
	Variables       []string `json:"variables,omitempty"`
	ButtonVariables []string `json:"button_variables,omitempty"`
}

// Template is a template as bZapper holds it. Meta owns the truth about Status;
// this is our copy, so the panel and sending by name do not need a call to Meta
// per click.
type Template struct {
	ID        string `json:"id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Language  string `json:"language,omitempty"`
	// Category is MARKETING, UTILITY or AUTHENTICATION.
	Category string `json:"category,omitempty"`
	// Status is PENDING, APPROVED, REJECTED, PAUSED or DISABLED. The decision is
	// asynchronous and can take up to 24 hours — it arrives through the
	// template.approved / template.rejected webhooks, so you never have to poll.
	Status string `json:"status,omitempty"`
	// RejectedReason is what Meta said when it refused.
	RejectedReason string `json:"rejected_reason,omitempty"`
	// Quality is GREEN, YELLOW or RED. RED means Meta is about to pause it.
	Quality string `json:"quality,omitempty"`
	// Variables is how many values the body expects, and therefore how many you
	// must pass in SendTemplateParams.Variables.
	Variables int           `json:"variables,omitempty"`
	Spec      *TemplateSpec `json:"spec,omitempty"`
	CreatedAt string        `json:"created_at,omitempty"`
	UpdatedAt string        `json:"updated_at,omitempty"`
}

// TemplateList is the response of ListTemplates (newest first).
type TemplateList struct {
	Templates []Template `json:"templates"`
}

// CreateTemplateResult is the response of CreateTemplate.
type CreateTemplateResult struct {
	Template Template `json:"template"`
	// Advice are warnings that did NOT block the submission but change the
	// outcome — promotional wording in a UTILITY template, for instance, which
	// Meta reclassifies on its own, and reclassification changes the price.
	Advice []string `json:"advice,omitempty"`
}

// SyncTemplatesResult is the response of SyncTemplates.
type SyncTemplatesResult struct {
	// Imported is how many templates were imported from Meta.
	Imported int `json:"imported"`
}

// FallbackTemplate is the response of GetFallbackTemplate.
type FallbackTemplate struct {
	// Template is nil when no fallback is configured.
	Template *Template `json:"template"`
}

// OTPEligibility is what Meta requires to allow Authentication templates and
// the state of each criterion.
type OTPEligibility struct {
	BusinessVerification  string `json:"business_verification"` // verified, pending, not_verified…
	MessagingLimit        string `json:"messaging_limit"`
	RequiredLimit         string `json:"required_limit"`
	VerificationOK        bool   `json:"verification_ok"`
	LimitOK               bool   `json:"limit_ok"`
	ApprovedAuthTemplates int    `json:"approved_auth_templates"`
}

// OTPTemplate is the response of GetOTPTemplate.
type OTPTemplate struct {
	// Template is nil when none was chosen (the first approved Authentication
	// template is used).
	Template    *Template      `json:"template"`
	Eligibility OTPEligibility `json:"eligibility"`
}

// TemplateMedia is the response of UploadTemplateMedia: pass URL as
// TemplateHeader.MediaURL in CreateTemplate.
type TemplateMedia struct {
	URL string `json:"url"`
}

// SendTemplateParams is the request for SendTemplate.
type SendTemplateParams struct {
	SendBase
	// Template is the template NAME (required). The id Meta assigned to it
	// never appears.
	Template string `json:"template"`
	// Language is e.g. pt_BR. Optional when the name exists in a single
	// language; if it exists in more than one, the send fails with 409
	// template_ambiguous — picking the wrong language for someone is worse than
	// asking.
	Language string `json:"language,omitempty"`
	// Variables are the body values, in order ({{1}}, {{2}}, …). The count must
	// match the approved template exactly — a mismatch (422) is the number one
	// cause of Meta error 132000, so the API checks it before spending the call.
	Variables []string `json:"variables,omitempty"`
	// HeaderVariables is the value for the header variable, when the template
	// has one.
	HeaderVariables []string `json:"header_variables,omitempty"`
	// ButtonVariables are the values for dynamic URL buttons, by button index.
	// Empty entries are skipped.
	ButtonVariables []string `json:"button_variables,omitempty"`
	// Code is for AUTHENTICATION templates only: Meta requires it both in the
	// body and in the copy-code button, and the API places it in both.
	Code string `json:"code,omitempty"`
	// OfferExpiresAt (RFC 3339) is the deadline of a limited-time offer with
	// expiration — the countdown runs to it.
	OfferExpiresAt string `json:"offer_expires_at,omitempty"`
	// Cards are the values of each carousel card, in order.
	Cards []TemplateCardValues `json:"cards,omitempty"`
}

// ListTemplates lists the project templates, newest first. GET /templates.
func (c *Client) ListTemplates(ctx context.Context) (*TemplateList, error) {
	return call[TemplateList](ctx, c, apiRequest{method: http.MethodGet, path: "/templates"})
}

// CreateTemplate submits a template to Meta for review. POST /templates.
//
// Validation happens BEFORE submission: Meta takes up to 24 hours to answer,
// rejects with a terse reason, and caps how many templates you may submit. A
// variable at the start or end of the body, numbering that skips, two variables
// side by side, a missing example — each comes back as 422 with a message that
// says what to fix. 409 = a template with this name and language already
// exists, or the account is not connected. The decision arrives later through
// the template.approved / template.rejected webhooks.
func (c *Client) CreateTemplate(ctx context.Context, spec TemplateSpec) (*CreateTemplateResult, error) {
	return call[CreateTemplateResult](ctx, c, apiRequest{method: http.MethodPost, path: "/templates", body: spec})
}

// GetTemplate returns one template. GET /templates/{id}.
func (c *Client) GetTemplate(ctx context.Context, id string) (*Template, error) {
	return call[Template](ctx, c, apiRequest{method: http.MethodGet, path: "/templates/" + seg(id)})
}

// DeleteTemplate removes a template here and at Meta. Meta deletes ALL
// languages of a name at once — so when another language of the same name is
// still registered here, the API leaves Meta alone rather than delete something
// you did not ask to delete. DELETE /templates/{id}.
func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/templates/" + seg(id)})
}

// EditTemplate changes the content of a template that already exists at Meta.
// Name and language are its identity and do not change. Meta allows editing an
// APPROVED template once every 24 hours and ten times in 30 days, sends it back
// to review (it returns to PENDING and does not send until the decision) and
// keeps the category of an approved template. PUT /templates/{id}.
func (c *Client) EditTemplate(ctx context.Context, id string, spec TemplateSpec) (*CreateTemplateResult, error) {
	return call[CreateTemplateResult](ctx, c, apiRequest{method: http.MethodPut, path: "/templates/" + seg(id), body: spec})
}

// LibraryQuery filters Meta's Template Library. Everything is optional.
type LibraryQuery struct {
	Search   string
	Topic    string
	Usecase  string
	Industry string
	Language string
}

// LibraryTemplate is a ready-made template from Meta's Template Library.
type LibraryTemplate struct {
	ID         string           `json:"id,omitempty"`
	Name       string           `json:"name"`
	Language   string           `json:"language"`
	Category   string           `json:"category"`
	Topic      string           `json:"topic,omitempty"`
	Usecase    string           `json:"usecase,omitempty"`
	Industry   []string         `json:"industry,omitempty"`
	Header     string           `json:"header,omitempty"`
	Body       string           `json:"body"`
	BodyParams []string         `json:"body_params,omitempty"`
	Buttons    []map[string]any `json:"buttons,omitempty"`
}

// LibraryTemplateList is the answer of ListLibraryTemplates.
type LibraryTemplateList struct {
	Templates []LibraryTemplate `json:"templates"`
}

// ListLibraryTemplates browses Meta's Template Library: utility and
// authentication texts Meta has already approved. A template created from one
// without changing the text is approved at once. GET /templates/library.
func (c *Client) ListLibraryTemplates(ctx context.Context, q LibraryQuery) (*LibraryTemplateList, error) {
	return call[LibraryTemplateList](ctx, c, apiRequest{method: http.MethodGet, path: "/templates/library",
		query: newQuery().str("search", q.Search).str("topic", q.Topic).str("usecase", q.Usecase).
			str("industry", q.Industry).str("language", q.Language).values()})
}

// LibraryButtonInput is what a URL or phone button of a library template needs.
type LibraryButtonInput struct {
	Type             string `json:"type"` // url | phone_number
	URL              string `json:"url,omitempty"`
	URLSuffixExample string `json:"url_suffix_example,omitempty"`
	Phone            string `json:"phone,omitempty"`
}

// LibraryCreate creates a template from the Template Library.
type LibraryCreate struct {
	LibraryTemplateName string               `json:"library_template_name"`
	Name                string               `json:"name,omitempty"`
	Language            string               `json:"language"`
	ButtonInputs        []LibraryButtonInput `json:"button_inputs,omitempty"`
	CodeExpiryMinutes   int                  `json:"code_expiry_minutes,omitempty"`
}

// CreateTemplateFromLibrary creates a template from a Template Library item.
// The text is Meta's, unchanged — that is what makes it approved at once; you
// only provide what the buttons need. POST /templates/library.
func (c *Client) CreateTemplateFromLibrary(ctx context.Context, req LibraryCreate) (*CreateTemplateResult, error) {
	return call[CreateTemplateResult](ctx, c, apiRequest{method: http.MethodPost, path: "/templates/library", body: req})
}

// TemplateStats is the performance of one template in the period.
type TemplateStats struct {
	Name         string  `json:"name"`
	Sent         int     `json:"sent"`
	Delivered    int     `json:"delivered"`
	Read         int     `json:"read"`
	Failed       int     `json:"failed"`
	DeliveryRate float64 `json:"delivery_rate"`
	ReadRate     float64 `json:"read_rate"`
}

// TemplateAnalytics is the answer of GetTemplateAnalytics.
type TemplateAnalytics struct {
	Days      int             `json:"days"`
	Templates []TemplateStats `json:"templates"`
}

// GetTemplateAnalytics counts sent, delivered, read and failed by template in
// the last `days` days (1–90; 0 = 30). GET /templates/analytics.
func (c *Client) GetTemplateAnalytics(ctx context.Context, days int) (*TemplateAnalytics, error) {
	return call[TemplateAnalytics](ctx, c, apiRequest{method: http.MethodGet, path: "/templates/analytics",
		query: newQuery().int("days", days).values()})
}

// SyncTemplates imports templates that exist at Meta and not here — the ones you
// created in WhatsApp Manager before using bZapper, or a webhook that never
// arrived. Without it those templates would be invisible here and sending by
// name would say they do not exist. POST /templates/sync.
func (c *Client) SyncTemplates(ctx context.Context) (*SyncTemplatesResult, error) {
	return call[SyncTemplatesResult](ctx, c, apiRequest{method: http.MethodPost, path: "/templates/sync"})
}

// UploadTemplateMedia uploads the header sample of a template (multipart,
// JPEG/PNG/MP4/PDF up to 10 MB). Pass the returned URL as
// TemplateHeader.MediaURL in CreateTemplate — no need to host the file anywhere.
// It is only the sample Meta reviews, not the media that goes out on each send;
// samples never used by a template are cleaned up. POST /templates/media.
func (c *Client) UploadTemplateMedia(ctx context.Context, file UploadFile) (*TemplateMedia, error) {
	r, err := multipartRequest(http.MethodPost, "/templates/media", file, nil)
	if err != nil {
		return nil, err
	}
	return call[TemplateMedia](ctx, c, r)
}

// GetFallbackTemplate returns the template sent when the 24-hour window is
// closed; FallbackTemplate.Template is nil when none is configured.
// GET /templates/fallback.
func (c *Client) GetFallbackTemplate(ctx context.Context) (*FallbackTemplate, error) {
	return call[FallbackTemplate](ctx, c, apiRequest{method: http.MethodGet, path: "/templates/fallback"})
}

// GetOTPTemplate returns the template that carries the verification code of
// SendOTP outside the 24-hour window, plus whether Meta already allows
// Authentication templates on the account. GET /templates/otp.
func (c *Client) GetOTPTemplate(ctx context.Context) (*OTPTemplate, error) {
	return call[OTPTemplate](ctx, c, apiRequest{method: http.MethodGet, path: "/templates/otp"})
}

// SetOTPTemplate chooses the APPROVED template that carries the verification
// code outside the window: Authentication or, until Meta releases it, a
// provisional Utility template with a copy-code button (422
// template_not_otp_capable otherwise). An empty templateID clears the choice.
// PUT /templates/otp.
func (c *Client) SetOTPTemplate(ctx context.Context, templateID string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPut, path: "/templates/otp",
		body: struct {
			TemplateID string `json:"template_id"`
		}{templateID}})
}

// SetFallbackTemplate chooses the template sent when the 24-hour window is
// closed. PUT /templates/fallback.
//
// This is what makes the same SendText work inside and outside the window:
// when the contact has not written for over 24 hours, Meta refuses free text;
// with a fallback configured the platform sends this approved template instead
// and reports the detour in Message.Delivery. Without one, that send fails with
// official_needs_template. Only an APPROVED template is accepted (409
// otherwise). An empty templateID clears it.
func (c *Client) SetFallbackTemplate(ctx context.Context, templateID string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPut, path: "/templates/fallback",
		body: struct {
			TemplateID string `json:"template_id"`
		}{templateID}})
}

// SendTemplate sends an approved template by name — the ONLY way to start a
// conversation outside the 24-hour service window. POST /messages/template.
//
// 404 = no template with that name in the project; 409 = not approved yet,
// ambiguous language (template_ambiguous) or a project on the unofficial rail;
// 422 = the number of Variables does not match the approved template.
// SendBase.IdempotencyKey, when set, goes in the Idempotency-Key header.
func (c *Client) SendTemplate(ctx context.Context, p SendTemplateParams) (*Message, error) {
	return c.sendMessage(ctx, "/messages/template", p)
}

// --- connection (Embedded Signup) and readiness --------------------------------

// OfficialReadiness reports what is ready and what is missing for the project
// to send on the official rail, checked live against Meta.
type OfficialReadiness struct {
	Connected bool `json:"connected"`
	// Status is PENDENTE, AGUARDANDO_PAGAMENTO, ATIVA or SUSPENSA.
	Status        string `json:"status,omitempty"`
	StatusReason  string `json:"status_reason,omitempty"`
	DisplayNumber string `json:"display_number,omitempty"`
	VerifiedName  string `json:"verified_name,omitempty"`
	// NumberVerified is true when the number completed SMS/call verification.
	NumberVerified bool `json:"number_verified"`
	// NameStatus is the display-name review (APPROVED, PENDING_REVIEW,
	// REJECTED). A rejection does not block sending — the number shows
	// instead of the name.
	NameStatus string `json:"name_status,omitempty"`
	// BusinessReview is the business verification (APPROVED, PENDING) — what
	// unlocks higher volume tiers.
	BusinessReview string `json:"business_review,omitempty"`
	// Currency is what Meta bills the account in. A Brazilian account still in
	// USD has to move to BRL (deadline 2027-06-30).
	Currency       string `json:"currency,omitempty"`
	QualityRating  string `json:"quality_rating,omitempty"`
	MessagingLimit string `json:"messaging_limit,omitempty"`
	// CanSend is the only question that matters: can this project send now?
	CanSend bool `json:"can_send"`
	// Live false means Meta could not be reached and the values are the last
	// known ones — the difference between "this is how it is" and "we could not
	// check" matters.
	Live bool `json:"live"`
	// Problem is invalid_token (reconnect required) or meta_unavailable.
	Problem string `json:"problem,omitempty"`
}

// OfficialSignupConfig is what the browser needs to open Meta's Embedded
// Signup. Both ids are public by nature — the customer's browser sees them;
// the app secret never leaves the server.
type OfficialSignupConfig struct {
	// Enabled false means this installation has no Embedded Signup configured:
	// offer the manual path (ConnectOfficialAccount) instead of a button that
	// cannot work.
	Enabled      bool   `json:"enabled"`
	AppID        string `json:"app_id"`
	ConfigID     string `json:"config_id"`
	GraphVersion string `json:"graph_version"`
}

// CompleteOfficialSignupParams is the request for CompleteOfficialSignup.
type CompleteOfficialSignupParams struct {
	// Code is the one-time code from the Embedded Signup callback. It expires
	// about 30 seconds after the popup closes — send it straight through.
	Code   string `json:"code"`
	WabaID string `json:"waba_id"`
	// PhoneNumberID is optional — discovered from the WABA when the popup does
	// not provide it.
	PhoneNumberID string `json:"phone_number_id,omitempty"`
}

// OfficialSignupResult is the response of CompleteOfficialSignup.
type OfficialSignupResult struct {
	Account *OfficialAccount `json:"account,omitempty"`
	// Warning is webhooks_not_subscribed or phone_not_registered when the
	// account was stored but a step needs attention (retry it); "" otherwise.
	Warning string `json:"warning,omitempty"`
}

// GetOfficialStatus reports what is ready and what is missing, checked live
// against Meta: whether the number is verified, whether the display name and
// the business passed review, quality and volume tier. GET /official/status.
func (c *Client) GetOfficialStatus(ctx context.Context) (*OfficialReadiness, error) {
	return call[OfficialReadiness](ctx, c, apiRequest{method: http.MethodGet, path: "/official/status"})
}

// GetOfficialSignupConfig returns the app and configuration ids the Meta JS SDK
// needs to open Embedded Signup. GET /official/signup-config.
func (c *Client) GetOfficialSignupConfig(ctx context.Context) (*OfficialSignupConfig, error) {
	return call[OfficialSignupConfig](ctx, c, apiRequest{method: http.MethodGet, path: "/official/signup-config"})
}

// CompleteOfficialSignup finishes the connection with the code returned by
// Meta's popup: exchanges it for a lasting token, subscribes the app to the
// WABA's webhooks and registers the number on the Cloud API. The customer never
// sees or types a credential. A non-fatal step that fails comes back in
// Warning instead of losing the whole connection. 409 multiple_phone_numbers =
// the WABA has more than one number (pass PhoneNumberID); 503 signup_disabled =
// Embedded Signup is not configured. POST /official/embedded-signup.
func (c *Client) CompleteOfficialSignup(ctx context.Context, p CompleteOfficialSignupParams) (*OfficialSignupResult, error) {
	return call[OfficialSignupResult](ctx, c, apiRequest{method: http.MethodPost, path: "/official/embedded-signup", body: p})
}

// --- connected numbers --------------------------------------------------------

// OfficialNumberList is the response of ListOfficialNumbers.
type OfficialNumberList struct {
	Numbers []OfficialAccount `json:"numbers"`
}

// MetaPhoneNumber is a number of the business account as Meta reports it.
type MetaPhoneNumber struct {
	// ID is Meta's id of the number (the phone_number_id AddOfficialNumber takes).
	ID                 string `json:"id,omitempty"`
	DisplayPhoneNumber string `json:"display_phone_number,omitempty"`
	VerifiedName       string `json:"verified_name,omitempty"`
	QualityRating      string `json:"quality_rating,omitempty"`
	// NameStatus is the display-name review: APPROVED, PENDING_REVIEW, REJECTED.
	NameStatus string `json:"name_status,omitempty"`
	// CodeVerificationStatus is VERIFIED, NOT_VERIFIED or EXPIRED.
	CodeVerificationStatus string `json:"code_verification_status,omitempty"`
	MessagingLimitTier     string `json:"messaging_limit_tier,omitempty"`
}

// MetaPhoneNumberList is the response of ListAvailableOfficialNumbers.
type MetaPhoneNumberList struct {
	Numbers []MetaPhoneNumber `json:"numbers"`
}

// ListOfficialNumbers lists the numbers connected to the project, default first.
// GET /official/numbers.
//
// Meta allows several numbers on one business account. Each has an InstanceID —
// the same value you pass in a send to choose which number it goes out from;
// the one flagged IsDefault sends when the request does not pick one.
func (c *Client) ListOfficialNumbers(ctx context.Context) (*OfficialNumberList, error) {
	return call[OfficialNumberList](ctx, c, apiRequest{method: http.MethodGet, path: "/official/numbers"})
}

// MetaWebhookEvent is one event Meta delivered to bZapper, raw, with how it
// was processed.
type MetaWebhookEvent struct {
	ID         int64     `json:"id"`
	ReceivedAt time.Time `json:"received_at"`
	// Fields lists the Meta change fields in the event, comma-separated.
	Fields string `json:"fields"`
	// Status is pending, running, done or failed.
	Status   string          `json:"status"`
	Attempts int             `json:"attempts"`
	Error    string          `json:"error,omitempty"`
	Body     json.RawMessage `json:"body"`
}

// MetaWebhookEventList is the page returned by ListMetaWebhookEvents.
type MetaWebhookEventList struct {
	Data []MetaWebhookEvent `json:"data"`
}

// ListMetaWebhookEvents returns the raw events Meta sent for the project's
// accounts, newest first — the "Meta → bZapper" side of webhooks. limit <= 0
// uses the server default (50); before > 0 pages back from that id.
// GET /official/webhook-events.
func (c *Client) ListMetaWebhookEvents(ctx context.Context, limit int, before int64) (*MetaWebhookEventList, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if before > 0 {
		q.Set("before", strconv.FormatInt(before, 10))
	}
	return call[MetaWebhookEventList](ctx, c, apiRequest{method: http.MethodGet, path: "/official/webhook-events", query: q})
}

// OfficialInsights is the official-rail dashboard: what Meta will charge the
// customer (estimated per category, with the month projection) and the
// rail's KPIs. bZapper charges only the subscription.
type OfficialInsights struct {
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	RateCard struct {
		Market    string             `json:"market"`
		Currency  string             `json:"currency"`
		Effective string             `json:"effective"`
		Source    string             `json:"source"`
		Rates     map[string]float64 `json:"rates"`
		Tiers     []struct {
			From int     `json:"from"`
			Rate float64 `json:"rate"`
		} `json:"tiers"`
	} `json:"rate_card"`
	Cost struct {
		Currency       string   `json:"currency"`
		Period         float64  `json:"period"`
		PreviousPeriod float64  `json:"previous_period"`
		ChangePct      *float64 `json:"change_pct"`
		MonthToDate    float64  `json:"month_to_date"`
		ProjectedMonth float64  `json:"projected_month"`
		ByCategory     []struct {
			Category string  `json:"category"`
			Messages int     `json:"messages"`
			Billable int     `json:"billable"`
			Cost     float64 `json:"cost"`
		} `json:"by_category"`
		Confirmed      int `json:"confirmed"`
		Estimated      int `json:"estimated"`
		OtherCountries int `json:"other_countries"`
	} `json:"cost"`
	Messages struct {
		Sent         int     `json:"sent"`
		Delivered    int     `json:"delivered"`
		Read         int     `json:"read"`
		Failed       int     `json:"failed"`
		Received     int     `json:"received"`
		DeliveryRate float64 `json:"delivery_rate"`
		ReadRate     float64 `json:"read_rate"`
		FreeMessages int     `json:"free_messages"`
		Contacts     int     `json:"contacts"`
	} `json:"messages"`
	Templates []struct {
		Name     string  `json:"name"`
		Category string  `json:"category"`
		Sent     int     `json:"sent"`
		Failed   int     `json:"failed"`
		Cost     float64 `json:"cost"`
	} `json:"templates"`
	Failures []struct {
		Reason string `json:"reason"`
		Count  int    `json:"count"`
	} `json:"failures"`
	AdReferrals      int `json:"ad_referrals"`
	MarketingOptOuts int `json:"marketing_opt_outs"`
	Numbers          []struct {
		DisplayNumber string `json:"display_number"`
		Quality       string `json:"quality"`
		Limit         string `json:"limit"`
		Status        string `json:"status"`
	} `json:"numbers"`
}

// InsightsParams picks the period: Days (ending now) or From/To. Zero values
// use the server default (last 30 days).
type InsightsParams struct {
	Days int
	From time.Time
	To   time.Time
}

// GetOfficialInsights returns the estimated Meta cost and the official-rail
// KPIs for a period. GET /official/insights.
func (c *Client) GetOfficialInsights(ctx context.Context, p InsightsParams) (*OfficialInsights, error) {
	q := url.Values{}
	if p.Days > 0 {
		q.Set("days", strconv.Itoa(p.Days))
	}
	if !p.From.IsZero() {
		q.Set("from", p.From.Format(time.RFC3339))
	}
	if !p.To.IsZero() {
		q.Set("to", p.To.Format(time.RFC3339))
	}
	return call[OfficialInsights](ctx, c, apiRequest{method: http.MethodGet, path: "/official/insights", query: q})
}

// OfficialMedia is a file stored at Meta (UploadOfficialMedia). Send it as
// MediaInput.ID from the same number for 30 days.
type OfficialMedia struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // image | sticker | video | audio | document
	MimeType   string    `json:"mime_type"`
	FileSize   int64     `json:"file_size"`
	InstanceID string    `json:"instance_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// OfficialMediaInfo is what Meta knows about a stored file (GetOfficialMedia).
type OfficialMediaInfo struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	FileSize int64  `json:"file_size"`
}

func mediaQuery(instanceID string) url.Values {
	if instanceID == "" {
		return nil
	}
	return url.Values{"instance_id": {instanceID}}
}

// UploadOfficialMedia uploads a file to Meta once, through the default number
// (or instanceID), and returns its id to reuse as MediaInput.ID in many sends —
// the same image for a whole campaign without uploading it per message.
// POST /official/media.
func (c *Client) UploadOfficialMedia(ctx context.Context, file UploadFile, instanceID string) (*OfficialMedia, error) {
	r, err := multipartRequest(http.MethodPost, "/official/media", file, nil)
	if err != nil {
		return nil, err
	}
	r.query = mediaQuery(instanceID)
	return call[OfficialMedia](ctx, c, r)
}

// GetOfficialMedia checks that a media id is still valid (404 media_not_found
// once it expires). GET /official/media/{media_id}.
func (c *Client) GetOfficialMedia(ctx context.Context, mediaID, instanceID string) (*OfficialMediaInfo, error) {
	return call[OfficialMediaInfo](ctx, c, apiRequest{method: http.MethodGet,
		path: "/official/media/" + seg(mediaID), query: mediaQuery(instanceID)})
}

// DeleteOfficialMedia deletes a stored file from Meta before it expires.
// DELETE /official/media/{media_id}.
func (c *Client) DeleteOfficialMedia(ctx context.Context, mediaID, instanceID string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete,
		path: "/official/media/" + seg(mediaID), query: mediaQuery(instanceID)})
}

// ListAvailableOfficialNumbers lists the business account's numbers that are
// not connected to the project yet — what feeds "add a number", without
// opening Meta's dashboard. GET /official/numbers/available.
func (c *Client) ListAvailableOfficialNumbers(ctx context.Context) (*MetaPhoneNumberList, error) {
	return call[MetaPhoneNumberList](ctx, c, apiRequest{method: http.MethodGet, path: "/official/numbers/available"})
}

// AddOfficialNumber connects one more number of the same business account,
// reusing the authorization already stored (it belongs to the account, not to
// the number). phoneNumberID is the ID from ListAvailableOfficialNumbers. The
// number is checked against Meta before it is saved, so a wrong id fails here
// (404 number_not_in_waba) and not on the first send. POST /official/numbers.
func (c *Client) AddOfficialNumber(ctx context.Context, phoneNumberID string) (*OfficialAccount, error) {
	return call[OfficialAccount](ctx, c, apiRequest{method: http.MethodPost, path: "/official/numbers",
		body: struct {
			PhoneNumberID string `json:"phone_number_id"`
		}{phoneNumberID}})
}

// SetDefaultOfficialNumber chooses the number that sends when a request does not
// pick one. PUT /official/numbers/{id}/default.
func (c *Client) SetDefaultOfficialNumber(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPut, path: "/official/numbers/" + seg(id) + "/default"})
}

// NumberHealthPoint is one change of quality or messaging limit of a number.
type NumberHealthPoint struct {
	QualityRating  string `json:"quality_rating"`
	MessagingLimit string `json:"messaging_limit"`
	At             string `json:"at"`
}

// NumberHealth is the current state and the change history of a number.
type NumberHealth struct {
	QualityRating  string              `json:"quality_rating"`
	MessagingLimit string              `json:"messaging_limit"`
	History        []NumberHealthPoint `json:"history"`
}

// GetOfficialNumberHealth returns every change of quality rating and messaging
// limit of a number, newest first — what explains why the limit dropped or when
// quality worsened. GET /official/numbers/{id}/health.
func (c *Client) GetOfficialNumberHealth(ctx context.Context, id string) (*NumberHealth, error) {
	return call[NumberHealth](ctx, c, apiRequest{method: http.MethodGet, path: "/official/numbers/" + seg(id) + "/health"})
}

// DisconnectOfficialNumber disconnects one number. The conversation history
// stays (the mirror instance is archived, not deleted); removing the default
// promotes the oldest remaining number. DELETE /official/numbers/{id}.
func (c *Client) DisconnectOfficialNumber(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodDelete, path: "/official/numbers/" + seg(id)})
}

// --- business profile ---------------------------------------------------------

// BusinessProfile is the business card of a number on the official rail — what
// the end customer sees when opening the chat. It lives on the NUMBER, not on
// the account: with several numbers, each has its own.
//
// On update, send the WHOLE profile: an empty (omitted) field is not
// "unchanged", it clears that field on Meta. Read it first, edit, send it back.
type BusinessProfile struct {
	// About is the short line under the name (up to 139 characters).
	About string `json:"about,omitempty"`
	// Description is up to 512 characters.
	Description string `json:"description,omitempty"`
	// Address is up to 256 characters.
	Address string `json:"address,omitempty"`
	Email   string `json:"email,omitempty"`
	// Vertical is the business sector, from Meta's closed list (UNDEFINED,
	// OTHER, AUTO, BEAUTY, APPAREL, EDU, ENTERTAIN, EVENT_PLAN, FINANCE,
	// GROCERY, GOVT, HOTEL, HEALTH, NONPROFIT, PROF_SERVICES, RETAIL, TRAVEL,
	// RESTAURANT, NOT_A_BIZ).
	Vertical string `json:"vertical,omitempty"`
	// Websites are at most 2.
	Websites []string `json:"websites,omitempty"`
	// ProfilePictureURL is the current photo, served by Meta. Read-only: change
	// it with UploadOfficialProfilePhoto / UploadOfficialNumberProfilePhoto.
	ProfilePictureURL string `json:"profile_picture_url,omitempty"`
}

// GetOfficialProfile reads the business profile of the project's default
// number, as Meta reports it. GET /official/profile.
func (c *Client) GetOfficialProfile(ctx context.Context) (*BusinessProfile, error) {
	return call[BusinessProfile](ctx, c, apiRequest{method: http.MethodGet, path: "/official/profile"})
}

// UpdateOfficialProfile updates the business profile of the project's default
// number and returns it normalized as sent to Meta. PUT /official/profile.
//
// Validated against Meta's published limits BEFORE sending, so a 422
// (profile_about_long, profile_description_long, profile_address_long,
// profile_email_invalid, profile_websites_max, profile_vertical_invalid) names
// the field and by how much it overflowed instead of Meta's generic "Invalid
// parameter". Send the WHOLE profile (see BusinessProfile).
func (c *Client) UpdateOfficialProfile(ctx context.Context, p BusinessProfile) (*BusinessProfile, error) {
	return call[BusinessProfile](ctx, c, apiRequest{method: http.MethodPut, path: "/official/profile", body: p})
}

// UploadOfficialProfilePhoto replaces the profile photo of the project's
// default number (multipart, JPEG/PNG up to 5 MB; Meta recommends 640x640).
// WhatsApp Business has only the round profile photo — there is no cover
// image. Returns the profile read back (ProfilePictureURL is the new one), or
// nil when the photo was saved but the profile could not be read back right
// away (204). POST /official/profile/photo.
func (c *Client) UploadOfficialProfilePhoto(ctx context.Context, file UploadFile) (*BusinessProfile, error) {
	r, err := multipartRequest(http.MethodPost, "/official/profile/photo", file, nil)
	if err != nil {
		return nil, err
	}
	return call[BusinessProfile](ctx, c, r)
}

// ReregisterOfficialProfile re-registers the project's default number on the
// Cloud API — the step people forget after Meta approves a new display name:
// until then the number keeps showing the old name. Requesting the name change
// itself is done in WhatsApp Manager; this is the half that is an API call.
// Already registered counts as success. 502 reregister_failed usually means
// two-step verification with a PIN chosen by the customer.
// POST /official/profile/reregister.
func (c *Client) ReregisterOfficialProfile(ctx context.Context) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/official/profile/reregister"})
}

// GetOfficialNumberProfile reads the business profile of one connected number
// (id from ListOfficialNumbers). GET /official/numbers/{id}/profile.
func (c *Client) GetOfficialNumberProfile(ctx context.Context, id string) (*BusinessProfile, error) {
	return call[BusinessProfile](ctx, c, apiRequest{method: http.MethodGet, path: "/official/numbers/" + seg(id) + "/profile"})
}

// UpdateOfficialNumberProfile updates the business profile of one connected
// number — same rules as UpdateOfficialProfile (validated before sending; send
// the WHOLE profile). PUT /official/numbers/{id}/profile.
func (c *Client) UpdateOfficialNumberProfile(ctx context.Context, id string, p BusinessProfile) (*BusinessProfile, error) {
	return call[BusinessProfile](ctx, c, apiRequest{method: http.MethodPut, path: "/official/numbers/" + seg(id) + "/profile", body: p})
}

// UploadOfficialNumberProfilePhoto replaces the profile photo of one connected
// number — same rules as UploadOfficialProfilePhoto (JPEG/PNG up to 5 MB; nil
// result on 204). POST /official/numbers/{id}/profile/photo.
func (c *Client) UploadOfficialNumberProfilePhoto(ctx context.Context, id string, file UploadFile) (*BusinessProfile, error) {
	r, err := multipartRequest(http.MethodPost, "/official/numbers/"+seg(id)+"/profile/photo", file, nil)
	if err != nil {
		return nil, err
	}
	return call[BusinessProfile](ctx, c, r)
}

// ReregisterOfficialNumberProfile re-registers one connected number on the
// Cloud API — same as ReregisterOfficialProfile, for the number given.
// POST /official/numbers/{id}/profile/reregister.
func (c *Client) ReregisterOfficialNumberProfile(ctx context.Context, id string) error {
	return c.exec(ctx, apiRequest{method: http.MethodPost, path: "/official/numbers/" + seg(id) + "/profile/reregister"})
}
