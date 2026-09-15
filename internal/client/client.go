package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pushmanhq/pushman-cli/internal/api"
	"github.com/pushmanhq/pushman-cli/internal/cli"
	"github.com/pushmanhq/pushman-cli/internal/credential"
)

const DefaultBaseURL = "https://api.pushman.whitekiwi.link/v1"

type WaitFunc func(context.Context, time.Duration) error

type Service struct {
	api             api.ClientWithResponsesInterface
	credentials     credential.Store
	automationToken string
	wait            WaitFunc
	clock           func() time.Time
	baseURL         string
}

func New(baseURL string, credentials credential.Store, automationToken string, httpClient *http.Client) (*Service, error) {
	baseURL, err := ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return fmt.Errorf("Pushman API redirects are not allowed")
			},
		}
	}
	generated, err := api.NewClientWithResponses(baseURL, api.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create Pushman API client: %w", err)
	}
	return &Service{api: generated, credentials: credentials, automationToken: automationToken,
		wait: waitContext, clock: time.Now, baseURL: baseURL}, nil
}

func ValidateBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("PUSHMAN_API_URL must be an absolute API base URL")
	}
	if parsed.Path != "/v1" {
		return "", fmt.Errorf("PUSHMAN_API_URL path must be /v1")
	}
	if parsed.Scheme != "https" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if parsed.Scheme != "http" || !(host == "localhost" || ip != nil && ip.IsLoopback()) {
			return "", fmt.Errorf("PUSHMAN_API_URL must use HTTPS; HTTP is allowed only for loopback development")
		}
	}
	return value, nil
}

func CredentialServiceName(baseURL string) string {
	digest := sha256.Sum256([]byte(baseURL))
	return "com.pushman.cli." + hex.EncodeToString(digest[:6])
}

func (s *Service) Pair(ctx context.Context, request cli.PairRequest) (cli.PairResult, error) {
	if _, err := s.credentials.Get(); err == nil {
		return cli.PairResult{}, &cli.ServiceError{Code: "already_paired", Message: "This CLI is already paired; run pushman logout first."}
	} else if !errors.Is(err, credential.ErrNotFound) {
		return cli.PairResult{}, err
	}
	platform := api.CreatePairingJSONBodyPlatform(request.Platform)
	response, err := s.api.CreatePairingWithResponse(ctx, api.CreatePairingJSONRequestBody{
		Platform: platform, SuggestedName: request.SuggestedName,
	})
	if err != nil {
		return cli.PairResult{}, transportError(err)
	}
	if response.JSON201 == nil {
		return cli.PairResult{}, responseError(response.StatusCode(), response.JSONDefault)
	}
	created := response.JSON201
	if request.OnChallenge != nil {
		if err := request.OnChallenge(cli.PairChallenge{UserCode: created.UserCode,
			VerificationURL: created.VerificationUri, ExpiresAt: created.ExpiresAt}); err != nil {
			return cli.PairResult{}, err
		}
	}
	interval := time.Duration(created.Interval) * time.Second
	for s.clock().Before(created.ExpiresAt) {
		if err := s.wait(ctx, interval); err != nil {
			return cli.PairResult{}, err
		}
		poll, err := s.api.GetPairingWithResponse(ctx, created.Id, &api.GetPairingParams{PairingSecret: created.PairingSecret})
		if err != nil {
			return cli.PairResult{}, transportError(err)
		}
		if poll.JSON200 == nil {
			return cli.PairResult{}, responseError(poll.StatusCode(), poll.JSONDefault)
		}
		switch poll.JSON200.Status {
		case api.PairingStatusPending:
			continue
		case api.PairingStatusApproved:
			if poll.JSON200.Credential == nil || poll.JSON200.Credential.Token == "" {
				return cli.PairResult{}, &cli.ServiceError{Code: "invalid_response", Message: "Pairing approval did not include a credential."}
			}
			if err := s.credentials.Set(poll.JSON200.Credential.Token); err != nil {
				return cli.PairResult{}, err
			}
			return cli.PairResult{Nickname: poll.JSON200.Credential.Name}, nil
		case api.PairingStatusDenied:
			return cli.PairResult{}, &cli.ServiceError{Code: "pairing_denied", Message: "Pairing was denied."}
		case api.PairingStatusExpired:
			return cli.PairResult{}, &cli.ServiceError{Code: "pairing_expired", Message: "Pairing expired; run pushman pair again."}
		default:
			return cli.PairResult{}, &cli.ServiceError{Code: "invalid_response", Message: "The server returned an unknown pairing state."}
		}
	}
	return cli.PairResult{}, &cli.ServiceError{Code: "pairing_expired", Message: "Pairing expired; run pushman pair again."}
}

func (s *Service) Login(ctx context.Context, request cli.LoginRequest) (cli.PairResult, error) {
	if _, err := s.credentials.Get(); err == nil {
		return cli.PairResult{}, &cli.ServiceError{Code: "already_authenticated", Message: "This CLI is already authorized; run pushman logout first."}
	} else if !errors.Is(err, credential.ErrNotFound) {
		return cli.PairResult{}, err
	}
	clientID := api.CreateDeviceAuthorizationFormdataBodyClientIdPushmanCli
	platform := api.CreateDeviceAuthorizationFormdataBodyPlatform(request.Platform)
	response, err := s.api.CreateDeviceAuthorizationWithFormdataBodyWithResponse(ctx, api.CreateDeviceAuthorizationFormdataRequestBody{
		ClientId: clientID, Platform: platform, SuggestedName: request.SuggestedName,
	})
	if err != nil {
		return cli.PairResult{}, transportError(err)
	}
	if response.JSON200 == nil {
		return cli.PairResult{}, responseError(response.StatusCode(), response.JSONDefault)
	}
	created := response.JSON200
	if request.OnChallenge != nil {
		if err := request.OnChallenge(cli.LoginChallenge{
			UserCode: created.UserCode, VerificationURL: created.VerificationUri,
			CompleteURL: created.VerificationUriComplete, ExpiresIn: time.Duration(created.ExpiresIn) * time.Second,
		}); err != nil {
			return cli.PairResult{}, err
		}
	}
	interval := time.Duration(created.Interval) * time.Second
	deadline := s.clock().Add(time.Duration(created.ExpiresIn) * time.Second)
	for s.clock().Before(deadline) {
		if err := s.wait(ctx, interval); err != nil {
			return cli.PairResult{}, err
		}
		tokenResponse, err := s.api.ExchangeDeviceCodeWithFormdataBodyWithResponse(ctx, api.ExchangeDeviceCodeFormdataRequestBody{
			ClientId:   api.ExchangeDeviceCodeFormdataBodyClientIdPushmanCli,
			DeviceCode: created.DeviceCode,
			GrantType:  api.UrnIetfParamsOauthGrantTypeDeviceCode,
		})
		if err != nil {
			return cli.PairResult{}, transportError(err)
		}
		if tokenResponse.JSON200 != nil {
			if tokenResponse.JSON200.AccessToken == "" {
				return cli.PairResult{}, &cli.ServiceError{Code: "invalid_response", Message: "Login approval did not include a credential."}
			}
			if err := s.credentials.Set(tokenResponse.JSON200.AccessToken); err != nil {
				return cli.PairResult{}, err
			}
			return cli.PairResult{Nickname: tokenResponse.JSON200.SenderName}, nil
		}
		if tokenResponse.JSON400 == nil {
			return cli.PairResult{}, responseError(tokenResponse.StatusCode(), tokenResponse.JSONDefault)
		}
		switch tokenResponse.JSON400.Error {
		case api.AuthorizationPending:
			continue
		case api.SlowDown:
			interval += 5 * time.Second
			continue
		case api.AccessDenied:
			return cli.PairResult{}, &cli.ServiceError{Code: "login_denied", Message: "Login was denied."}
		case api.ExpiredToken:
			return cli.PairResult{}, &cli.ServiceError{Code: "login_expired", Message: "Login expired; run pushman login again."}
		default:
			return cli.PairResult{}, &cli.ServiceError{Code: "login_failed", Message: "The login request is no longer valid."}
		}
	}
	return cli.PairResult{}, &cli.ServiceError{Code: "login_expired", Message: "Login expired; run pushman login again."}
}

func (s *Service) Status(ctx context.Context) (cli.StatusResult, error) {
	token, err := s.credentials.Get()
	if errors.Is(err, credential.ErrNotFound) {
		return cli.StatusResult{Paired: false}, nil
	}
	if err != nil {
		return cli.StatusResult{}, err
	}
	response, err := s.api.GetCurrentSenderCredentialWithResponse(ctx, authorize(token))
	if err != nil {
		return cli.StatusResult{}, transportError(err)
	}
	if response.JSON200 == nil {
		return cli.StatusResult{}, responseError(response.StatusCode(), response.JSONDefault)
	}
	method := ""
	if response.JSON200.AuthorizationMethod != nil {
		method = string(*response.JSON200.AuthorizationMethod)
	}
	return cli.StatusResult{Paired: true, Nickname: response.JSON200.Name, Method: method}, nil
}

func (s *Service) Rename(ctx context.Context, name string) error {
	token, err := s.pairedToken()
	if err != nil {
		return err
	}
	response, err := s.api.UpdateCurrentSenderCredentialWithResponse(ctx,
		api.UpdateCurrentSenderCredentialJSONRequestBody{Name: name}, authorize(token))
	if err != nil {
		return transportError(err)
	}
	if response.JSON200 == nil {
		return responseError(response.StatusCode(), response.JSONDefault)
	}
	return nil
}

func (s *Service) Logout(ctx context.Context) error {
	token, err := s.credentials.Get()
	if errors.Is(err, credential.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	response, err := s.api.RevokeCurrentSenderCredentialWithResponse(ctx, authorize(token))
	if err != nil {
		return transportError(err)
	}
	if response.StatusCode() != http.StatusNoContent && response.StatusCode() != http.StatusUnauthorized {
		return responseError(response.StatusCode(), response.JSONDefault)
	}
	return s.credentials.Delete()
}

func (s *Service) Push(ctx context.Context, request cli.PushRequest) (cli.PushResult, error) {
	token := s.automationToken
	if token == "" {
		var err error
		token, err = s.pairedToken()
		if err != nil {
			return cli.PushResult{}, err
		}
	}
	body := api.CreateMessageJSONRequestBody{Body: request.Body}
	if request.Title != "" {
		body.Title = &request.Title
	}
	if request.Subtitle != "" {
		body.Subtitle = &request.Subtitle
	}
	if request.URL != "" {
		body.Url = &request.URL
	}
	if request.Group != "" {
		value := api.RestrictedIdentifier(request.Group)
		body.Group = &value
	}
	if request.Image != "" {
		body.Image = &request.Image
	}
	sound := api.MessageInputSound(request.Sound)
	body.Sound = &sound
	if request.Key != "" {
		value := api.RestrictedIdentifier(request.Key)
		body.Key = &value
	}
	format := api.MessageInputFormat(request.Format)
	body.Format = &format
	if len(request.Devices) != 0 {
		body.Devices = &request.Devices
	}
	response, err := s.api.CreateMessageWithResponse(ctx, body, authorize(token))
	if err != nil {
		return cli.PushResult{}, transportError(err)
	}
	if response.JSON202 == nil {
		return cli.PushResult{}, responseError(response.StatusCode(), response.JSONDefault)
	}
	return cli.PushResult{ID: response.JSON202.Id, Status: "accepted",
		DeviceCount: response.JSON202.TargetCount, AcceptedAt: response.JSON202.AcceptedAt}, nil
}

func (s *Service) Devices(ctx context.Context) ([]cli.Device, error) {
	token, err := s.pairedToken()
	if err != nil {
		return nil, err
	}
	response, err := s.api.ListDevicesWithResponse(ctx, authorize(token))
	if err != nil {
		return nil, transportError(err)
	}
	if response.JSON200 == nil {
		return nil, responseError(response.StatusCode(), response.JSONDefault)
	}
	devices := make([]cli.Device, 0, len(response.JSON200.Items))
	for _, item := range response.JSON200.Items {
		status := string(item.NotificationStatus)
		if item.Eligible {
			status = "eligible"
		}
		devices = append(devices, cli.Device{Nickname: item.Name, Status: status})
	}
	return devices, nil
}

func (s *Service) History(ctx context.Context) ([]cli.HistoryItem, error) {
	token, err := s.pairedToken()
	if err != nil {
		return nil, err
	}
	var items []cli.HistoryItem
	var cursor *string
	limit := 100
	for page := 0; page < 100; page++ {
		response, err := s.api.ListOwnMessagesWithResponse(ctx, &api.ListOwnMessagesParams{Cursor: cursor, Limit: &limit}, authorize(token))
		if err != nil {
			return nil, transportError(err)
		}
		if response.JSON200 == nil {
			return nil, responseError(response.StatusCode(), response.JSONDefault)
		}
		for _, item := range response.JSON200.Items {
			items = append(items, cli.HistoryItem{ID: item.Id, Title: item.Title, UpdatedAt: item.AcceptedAt,
				UpdateCount: item.UpdateCount, DeliveryState: string(item.DeliveryState)})
		}
		cursor = response.JSON200.NextCursor
		if cursor == nil || *cursor == "" {
			return items, nil
		}
	}
	return nil, &cli.ServiceError{Code: "invalid_response", Message: "History pagination did not terminate."}
}

func (s *Service) HistoryShow(ctx context.Context, messageID string) (cli.HistoryDetail, error) {
	token, err := s.pairedToken()
	if err != nil {
		return cli.HistoryDetail{}, err
	}
	response, err := s.api.GetOwnMessageWithResponse(ctx, messageID, authorize(token))
	if err != nil {
		return cli.HistoryDetail{}, transportError(err)
	}
	if response.JSON200 == nil {
		return cli.HistoryDetail{}, responseError(response.StatusCode(), response.JSONDefault)
	}
	detail := cli.HistoryDetail{LogicalMessageID: response.JSON200.LogicalMessageId, Read: response.JSON200.Read}
	for _, item := range response.JSON200.Revisions {
		revision := cli.HistoryRevision{ID: item.Id, Title: item.Title, Body: item.Body, SenderName: item.SenderName,
			Sound: string(item.Sound), Format: string(item.Format), UpdatedAt: item.AcceptedAt}
		if item.Subtitle != nil {
			revision.Subtitle = *item.Subtitle
		}
		if item.Url != nil {
			revision.URL = *item.Url
		}
		if item.Image != nil {
			revision.Image = *item.Image
		}
		for _, value := range item.Deliveries {
			delivery := cli.HistoryDelivery{DeviceName: value.DeviceName, State: string(value.State)}
			if value.FailureCode != nil {
				delivery.Failure = *value.FailureCode
			}
			revision.Deliveries = append(revision.Deliveries, delivery)
		}
		detail.Revisions = append(detail.Revisions, revision)
	}
	return detail, nil
}

func (s *Service) Usage(ctx context.Context) (cli.UsageResult, error) {
	token, err := s.pairedToken()
	if err != nil {
		return cli.UsageResult{}, err
	}
	response, err := s.api.GetUsageWithResponse(ctx, authorize(token))
	if err != nil {
		return cli.UsageResult{}, transportError(err)
	}
	if response.JSON200 == nil {
		return cli.UsageResult{}, responseError(response.StatusCode(), response.JSONDefault)
	}
	result := cli.UsageResult{Used: response.JSON200.Used, Limit: response.JSON200.Limit,
		ResetsAt: response.JSON200.PeriodEnd}
	if value := response.JSON200.Plan; value != nil && value.Valid() {
		result.Plan = string(*value)
	}
	if value := response.JSON200.BillingState; value != nil && value.Valid() {
		result.BillingState = string(*value)
	}
	return result, nil
}

func (s *Service) Doctor(ctx context.Context) ([]cli.DoctorCheck, error) {
	checks := []cli.DoctorCheck{{Name: "api-url", OK: true, Message: s.baseURL}}
	status, err := s.Status(ctx)
	if err != nil {
		checks = append(checks, cli.DoctorCheck{Name: "credential", OK: false, Message: err.Error()})
		return checks, nil
	}
	if !status.Paired {
		checks = append(checks, cli.DoctorCheck{Name: "credential", OK: false, Message: "not paired"})
	} else {
		checks = append(checks, cli.DoctorCheck{Name: "credential", OK: true, Message: "paired as " + status.Nickname})
	}
	return checks, nil
}

func (s *Service) pairedToken() (string, error) {
	token, err := s.credentials.Get()
	if errors.Is(err, credential.ErrNotFound) {
		return "", &cli.AuthorizationError{Err: fmt.Errorf("not authorized; run pushman login or pushman pair")}
	}
	return token, err
}

func authorize(token string) api.RequestEditorFn {
	return func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

func responseError(status int, body *api.APIError) error {
	if body == nil {
		return &cli.ServiceError{Code: "unexpected_response", Message: fmt.Sprintf("Pushman API returned HTTP %d.", status)}
	}
	errorValue := &cli.ServiceError{Code: body.Error.Code, Message: body.Error.Message}
	if body.Error.RetryAfter != nil {
		errorValue.RetryAfter = *body.Error.RetryAfter
	}
	if status == http.StatusUnauthorized {
		return &cli.AuthorizationError{Err: errorValue}
	}
	return errorValue
}

func transportError(err error) error {
	return fmt.Errorf("contact Pushman API: %w", err)
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
