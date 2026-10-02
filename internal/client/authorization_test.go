package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/pushmanhq/pushman-cli/internal/cli"
	"github.com/pushmanhq/pushman-cli/internal/credential"
)

func TestLoginFailureLeavesNoCredential(t *testing.T) {
	for _, test := range []struct {
		name, oauthError, wantCode string
		cancel, deadline           bool
	}{
		{name: "denied", oauthError: "access_denied", wantCode: "login_denied"},
		{name: "expired", oauthError: "expired_token", wantCode: "login_expired"},
		{name: "invalid grant", oauthError: "invalid_grant", wantCode: "login_failed"},
		{name: "interrupted polling", cancel: true},
		{name: "local deadline", deadline: true, wantCode: "login_expired"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := loginTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"error":%q}`, test.oauthError)
			})
			defer server.Close()
			store := new(authorizationStore)
			service, err := New(server.URL+"/v1", store, "", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			service.wait = func(context.Context, time.Duration) error {
				if test.cancel {
					return context.Canceled
				}
				return nil
			}
			if test.deadline {
				calls := 0
				now := time.Now()
				service.clock = func() time.Time {
					calls++
					if calls == 1 {
						return now
					}
					return now.Add(time.Hour)
				}
			}
			_, err = service.Login(context.Background(), cli.LoginRequest{Platform: "windows", SuggestedName: "Test sender"})
			if test.cancel {
				if !errors.Is(err, context.Canceled) || cli.ExitCode(err) != 130 {
					t.Fatalf("cancellation error/exit=%v/%d", err, cli.ExitCode(err))
				}
			} else {
				var serviceErr *cli.ServiceError
				if !errors.As(err, &serviceErr) || serviceErr.Code != test.wantCode {
					t.Fatalf("login error=%v, want %s", err, test.wantCode)
				}
			}
			if store.setCalls != 0 || store.token != "" {
				t.Fatal("failed authorization wrote a credential")
			}
		})
	}
}

func TestLoginSlowDownAndStoreFailure(t *testing.T) {
	requests := 0
	server := loginTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"slow_down"}`)
			return
		}
		fmt.Fprint(w, `{"access_token":"synthetic-test-credential","token_type":"Bearer","scope":"push","sender_name":"Test sender"}`)
	})
	defer server.Close()
	storeErr := errors.New("synthetic store unavailable")
	store := &authorizationStore{setErr: storeErr}
	service, err := New(server.URL+"/v1", store, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var intervals []time.Duration
	service.wait = func(_ context.Context, interval time.Duration) error {
		intervals = append(intervals, interval)
		return nil
	}
	_, err = service.Login(context.Background(), cli.LoginRequest{Platform: "windows", SuggestedName: "Test sender"})
	if !errors.Is(err, storeErr) || store.setCalls != 1 || store.token != "" {
		t.Fatal("credential store failure was not propagated atomically")
	}
	if !reflect.DeepEqual(intervals, []time.Duration{5 * time.Second, 10 * time.Second}) {
		t.Fatalf("poll intervals=%v", intervals)
	}
}

func TestLogoutKeepsCredentialUntilServerRevocation(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		deleteErr error
		wantCalls int
		wantErr   bool
	}{
		{name: "revoked", status: http.StatusNoContent, wantCalls: 1},
		{name: "already revoked", status: http.StatusUnauthorized, wantCalls: 1},
		{name: "server error", status: http.StatusInternalServerError, wantErr: true},
		{name: "local delete error", status: http.StatusNoContent, deleteErr: errors.New("synthetic deletion failure"), wantCalls: 1, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &authorizationStore{token: "synthetic-test-credential", deleteErr: test.deleteErr}
			revoked := make(chan struct{})
			store.beforeDelete = func() {
				select {
				case <-revoked:
				default:
					t.Error("local credential deleted before server revocation")
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete || r.URL.Path != "/v1/sender-credentials/current" {
					t.Errorf("unexpected revocation request: %s %s", r.Method, r.URL.Path)
				}
				close(revoked)
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			service, err := New(server.URL+"/v1", store, "", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			err = service.Logout(context.Background())
			if (err != nil) != test.wantErr || store.deleteCalls != test.wantCalls {
				t.Fatalf("error=%v delete calls=%d", err, store.deleteCalls)
			}
			if test.wantErr && store.token == "" {
				t.Fatal("failed logout removed its local credential")
			}
		})
	}
}

func TestLogoutTransportAndUnavailableStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	store := &authorizationStore{token: "synthetic-test-credential"}
	service, err := New(server.URL+"/v1", store, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(context.Background()); err == nil || store.deleteCalls != 0 || store.token == "" {
		t.Fatal("transport failure discarded the credential")
	}
	store.getErr = errors.New("synthetic store unavailable")
	if err := service.Logout(context.Background()); !errors.Is(err, store.getErr) || store.deleteCalls != 0 {
		t.Fatal("unavailable credential store was not reported")
	}
}

func loginTestServer(t *testing.T, exchange http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/device-authorizations":
			fmt.Fprint(w, `{"device_code":"synthetic-device-code","user_code":"TEST-CODE","verification_uri":"https://example.com/activate","verification_uri_complete":"https://example.com/activate?code=TEST-CODE","expires_in":600,"interval":5}`)
		case "/v1/oauth/token":
			exchange(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}

type authorizationStore struct {
	token                     string
	getErr, setErr, deleteErr error
	setCalls, deleteCalls     int
	beforeDelete              func()
}

func (s *authorizationStore) Get() (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}
	if s.token == "" {
		return "", credential.ErrNotFound
	}
	return s.token, nil
}

func (s *authorizationStore) Set(value string) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.token = value
	return nil
}

func (s *authorizationStore) Delete() error {
	if s.beforeDelete != nil {
		s.beforeDelete()
	}
	s.deleteCalls++
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.token = ""
	return nil
}
