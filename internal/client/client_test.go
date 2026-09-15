package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pushmanhq/pushman-cli/internal/cli"
	"github.com/pushmanhq/pushman-cli/internal/credential"
)

type memoryCredentials struct{ token string }

func TestUsagePreservesCountersAndValidatesOptionalMetadata(t *testing.T) {
	for _, tt := range []struct {
		fields      string
		plan, state string
	}{
		{``, "", ""},
		{`,"plan":"pro","billingState":"synchronized"`, "pro", "synchronized"},
		{`,"plan":"free","billingState":"unavailable"`, "free", "unavailable"},
		{`,"plan":"future","billingState":"unknown"`, "", ""},
		{`,"plan":"bad\u001b[2J","billingState":"bad\nsecret"`, "", ""},
	} {
		t.Run(tt.fields, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/usage" || r.Header.Get("Authorization") != "Bearer pm_cli_test" {
					t.Error("wrong authorization")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"used":9000,"limit":200,"periodStart":"2026-09-01T00:00:00Z","periodEnd":"2026-10-01T00:00:00Z"` + tt.fields + `}`))
			}))
			defer srv.Close()
			s, err := New(srv.URL+"/v1", &memoryCredentials{token: "pm_cli_test"}, "", srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			usage, err := s.Usage(context.Background())
			if err != nil || usage.Used != 9000 || usage.Limit != 200 || usage.Plan != tt.plan || usage.BillingState != tt.state {
				t.Fatalf("usage=%+v err=%v", usage, err)
			}
		})
	}
}

func (m *memoryCredentials) Get() (string, error) {
	if m.token == "" {
		return "", credential.ErrNotFound
	}
	return m.token, nil
}
func (m *memoryCredentials) Set(value string) error { m.token = value; return nil }
func (m *memoryCredentials) Delete() error          { m.token = ""; return nil }

func TestPairStoresCredentialOnlyAfterApproval(t *testing.T) {
	expiresAt := time.Now().Add(time.Minute).UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/pairings":
			response.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(response).Encode(map[string]any{
				"id": "pair_01K00000000000000000000000", "userCode": "ABCD-EFGH",
				"verificationUri": "https://app.pushman.example/pair", "expiresAt": expiresAt,
				"interval": 5, "status": "pending", "pairingSecret": "pair-secret",
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/pairings/pair_01K00000000000000000000000":
			if request.Header.Get("pairingSecret") != "pair-secret" {
				t.Fatal("missing pairing secret header")
			}
			_ = json.NewEncoder(response).Encode(map[string]any{
				"id": "pair_01K00000000000000000000000", "userCode": "ABCD-EFGH",
				"verificationUri": "https://app.pushman.example/pair", "expiresAt": expiresAt,
				"interval": 5, "status": "approved",
				"credential": map[string]string{"token": "pm_cli_secret", "name": "Build Mac"},
			})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	credentials := new(memoryCredentials)
	service, err := New(server.URL+"/v1", credentials, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	service.wait = func(context.Context, time.Duration) error { return nil }
	var challenge cli.PairChallenge
	result, err := service.Pair(context.Background(), cli.PairRequest{
		Platform: "darwin", SuggestedName: "Build Mac",
		OnChallenge: func(value cli.PairChallenge) error { challenge = value; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if challenge.UserCode != "ABCD-EFGH" || result.Nickname != "Build Mac" || credentials.token != "pm_cli_secret" {
		t.Fatalf("challenge/result/token = %#v / %#v / %q", challenge, result, credentials.token)
	}
}

func TestLoginPollsDeviceFlowAndStoresCredential(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/device-authorizations":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("client_id") != "pushman-cli" || request.Form.Get("platform") != "darwin" || request.Form.Get("suggested_name") != "Build Mac" {
				t.Fatalf("authorization form = %v", request.Form)
			}
			_, _ = response.Write([]byte(`{"device_code":"device-secret","user_code":"ABCD-EFGH","verification_uri":"https://app.pushman.example/activate/","verification_uri_complete":"https://app.pushman.example/activate/?user_code=ABCD-EFGH","expires_in":600,"interval":5}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/oauth/token":
			requests++
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if request.Form.Get("device_code") != "device-secret" || request.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
				t.Fatalf("token form = %v", request.Form)
			}
			if requests == 1 {
				response.WriteHeader(http.StatusBadRequest)
				_, _ = response.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			_, _ = response.Write([]byte(`{"access_token":"pm_cli_web","token_type":"Bearer","scope":"push devices:read history:read usage:read","sender_name":"Build Mac"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	credentials := new(memoryCredentials)
	service, err := New(server.URL+"/v1", credentials, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	service.wait = func(context.Context, time.Duration) error { return nil }
	var challenge cli.LoginChallenge
	result, err := service.Login(context.Background(), cli.LoginRequest{
		Platform: "darwin", SuggestedName: "Build Mac",
		OnChallenge: func(value cli.LoginChallenge) error { challenge = value; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || challenge.UserCode != "ABCD-EFGH" || challenge.CompleteURL == "" ||
		result.Nickname != "Build Mac" || credentials.token != "pm_cli_web" {
		t.Fatalf("requests/challenge/result/token = %d / %#v / %#v / %q", requests, challenge, result, credentials.token)
	}
}

func TestPushPrefersAutomationToken(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		_, _ = response.Write([]byte(`{"id":"msg_01K00000000000000000000000","logicalMessageId":"lmsg_01K00000000000000000000000","acceptedAt":"2026-08-25T00:00:00Z","targetCount":1}`))
	}))
	defer server.Close()
	credentials := &memoryCredentials{token: "pm_cli_paired"}
	service, err := New(server.URL+"/v1", credentials, "pm_auto_token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Push(context.Background(), cli.PushRequest{Body: "done", Sound: "default", Format: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer pm_auto_token" || result.Status != "accepted" {
		t.Fatalf("authorization/result = %q / %#v", authorization, result)
	}
}

func TestUnauthorizedResponseMapsToAuthorizationExit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"error":{"code":"unauthorized","message":"revoked","requestId":"req_test"}}`))
	}))
	defer server.Close()
	service, err := New(server.URL+"/v1", &memoryCredentials{token: "revoked"}, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Status(context.Background())
	var authorization *cli.AuthorizationError
	if !errors.As(err, &authorization) || cli.ExitCode(err) != 4 {
		t.Fatalf("error/exit = %T %v / %d", err, err, cli.ExitCode(err))
	}
}

func TestHistoryListAndDetailUsePairedCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer pm_cli_test" {
			t.Fatal("missing paired authorization")
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/messages":
			_, _ = response.Write([]byte(`{"items":[{"id":"msg_01K00000000000000000000000","logicalMessageId":"lmsg_01K00000000000000000000000","title":"Deploy","body":"done","senderName":"Build Mac","acceptedAt":"2026-08-25T00:00:00Z","read":false,"updateCount":2,"deliveryState":"sent"}]}`))
		case "/v1/messages/msg_01K00000000000000000000000":
			_, _ = response.Write([]byte(`{"logicalMessageId":"lmsg_01K00000000000000000000000","read":false,"revisions":[{"id":"msg_01K00000000000000000000000","title":"Deploy","body":"done","senderName":"Build Mac","acceptedAt":"2026-08-25T00:00:00Z","sound":"none","format":"monospace","deliveries":[{"deviceId":"dev_01K00000000000000000000000","deviceName":"iPhone","state":"sent"}]}]}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	service, err := New(server.URL+"/v1", &memoryCredentials{token: "pm_cli_test"}, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.History(context.Background())
	if err != nil || len(items) != 1 || items[0].UpdateCount != 2 || items[0].DeliveryState != "sent" {
		t.Fatalf("history = %#v, %v", items, err)
	}
	detail, err := service.HistoryShow(context.Background(), items[0].ID)
	if err != nil || len(detail.Revisions) != 1 || detail.Revisions[0].Format != "monospace" || detail.Revisions[0].Deliveries[0].DeviceName != "iPhone" {
		t.Fatalf("detail = %#v, %v", detail, err)
	}
}

func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
	}{
		{"https://api.pushman.example/v1", true},
		{"http://127.0.0.1:8080/v1", true},
		{"http://localhost:8080/v1/", true},
		{"http://api.pushman.example/v1", false},
		{"https://api.pushman.example/v2", false},
		{"https://user@example.com/v1", false},
		{"https://api.pushman.example/v1?token=secret", false},
	}
	for _, test := range tests {
		t.Run(strings.ReplaceAll(test.value, "/", "_"), func(t *testing.T) {
			_, err := ValidateBaseURL(test.value)
			if (err == nil) != test.ok {
				t.Fatalf("ValidateBaseURL(%q) error = %v", test.value, err)
			}
		})
	}
}
