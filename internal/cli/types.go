package cli

import (
	"context"
	"errors"
	"time"
)

var ErrServiceUnconfigured = errors.New("Pushman API transport is not configured in this scaffold")

type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

type PairRequest struct {
	Platform      string
	SuggestedName string
	OnChallenge   func(PairChallenge) error
}

type PairResult struct {
	Nickname string
}

type LoginRequest struct {
	Platform      string
	SuggestedName string
	OnChallenge   func(LoginChallenge) error
}

type LoginChallenge struct {
	UserCode        string
	VerificationURL string
	CompleteURL     string
	ExpiresIn       time.Duration
}

type PairChallenge struct {
	UserCode        string
	VerificationURL string
	ExpiresAt       time.Time
}

type StatusResult struct {
	Paired   bool
	Nickname string
	Method   string
}

type Device struct {
	Nickname string
	Status   string
}

type PushRequest struct {
	Body     string   `json:"body"`
	Title    string   `json:"title,omitempty"`
	Subtitle string   `json:"subtitle,omitempty"`
	URL      string   `json:"url,omitempty"`
	Group    string   `json:"group,omitempty"`
	Image    string   `json:"image,omitempty"`
	Sound    string   `json:"sound"`
	Key      string   `json:"key,omitempty"`
	Format   string   `json:"format"`
	Devices  []string `json:"devices,omitempty"`
}

type PushResult struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	DeviceCount int       `json:"deviceCount"`
	AcceptedAt  time.Time `json:"acceptedAt"`
}

type HistoryItem struct {
	ID            string
	Title         string
	UpdatedAt     time.Time
	UpdateCount   int
	DeliveryState string
}

type HistoryDetail struct {
	LogicalMessageID string
	Read             bool
	Revisions        []HistoryRevision
}

type HistoryRevision struct {
	ID         string
	Title      string
	Subtitle   string
	Body       string
	SenderName string
	URL        string
	Image      string
	Sound      string
	Format     string
	UpdatedAt  time.Time
	Deliveries []HistoryDelivery
}

type HistoryDelivery struct {
	DeviceName string
	State      string
	Failure    string
}

type UsageResult struct {
	Used         int
	Limit        int
	ResetsAt     time.Time
	Plan         string
	BillingState string
}

type DoctorCheck struct {
	Name    string
	OK      bool
	Message string
}

// Service is the command-facing boundary for the future generated API client and
// credential store. Its concrete HTTP implementation follows the shared OpenAPI contract.
type Service interface {
	Pair(context.Context, PairRequest) (PairResult, error)
	Login(context.Context, LoginRequest) (PairResult, error)
	Status(context.Context) (StatusResult, error)
	Rename(context.Context, string) error
	Logout(context.Context) error
	Push(context.Context, PushRequest) (PushResult, error)
	Devices(context.Context) ([]Device, error)
	History(context.Context) ([]HistoryItem, error)
	HistoryShow(context.Context, string) (HistoryDetail, error)
	Usage(context.Context) (UsageResult, error)
	Doctor(context.Context) ([]DoctorCheck, error)
}

type UnconfiguredService struct{}

func (UnconfiguredService) Pair(context.Context, PairRequest) (PairResult, error) {
	return PairResult{}, ErrServiceUnconfigured
}
func (UnconfiguredService) Login(context.Context, LoginRequest) (PairResult, error) {
	return PairResult{}, ErrServiceUnconfigured
}
func (UnconfiguredService) Status(context.Context) (StatusResult, error) {
	return StatusResult{}, ErrServiceUnconfigured
}
func (UnconfiguredService) Rename(context.Context, string) error { return ErrServiceUnconfigured }
func (UnconfiguredService) Logout(context.Context) error         { return ErrServiceUnconfigured }
func (UnconfiguredService) Push(context.Context, PushRequest) (PushResult, error) {
	return PushResult{}, ErrServiceUnconfigured
}
func (UnconfiguredService) Devices(context.Context) ([]Device, error) {
	return nil, ErrServiceUnconfigured
}
func (UnconfiguredService) History(context.Context) ([]HistoryItem, error) {
	return nil, ErrServiceUnconfigured
}
func (UnconfiguredService) HistoryShow(context.Context, string) (HistoryDetail, error) {
	return HistoryDetail{}, ErrServiceUnconfigured
}
func (UnconfiguredService) Usage(context.Context) (UsageResult, error) {
	return UsageResult{}, ErrServiceUnconfigured
}
func (UnconfiguredService) Doctor(context.Context) ([]DoctorCheck, error) {
	return nil, ErrServiceUnconfigured
}
