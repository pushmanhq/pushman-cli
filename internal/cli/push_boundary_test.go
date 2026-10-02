package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestPushStdinScalarBoundary(t *testing.T) {
	fullBody := strings.Repeat("\U0001F600", 4096)
	for _, test := range []struct {
		name, input, want string
		wantErr           bool
	}{
		{name: "full body", input: fullBody, want: fullBody},
		{name: "full body LF", input: fullBody + "\n", want: fullBody},
		{name: "full body CRLF", input: fullBody + "\r\n", want: fullBody},
		{name: "too many scalars", input: fullBody + "a", wantErr: true},
		{name: "too many ASCII scalars", input: strings.Repeat("a", 4097), wantErr: true},
		{name: "one extra LF remains", input: fullBody + "\n\n", wantErr: true},
		{name: "one extra CRLF remains", input: fullBody + "\r\n\r\n", wantErr: true},
		{name: "malformed UTF-8", input: "hello\xff\r\n", wantErr: true},
		{name: "preserve internal CRLF", input: "first\r\nsecond\r\n", want: "first\r\nsecond"},
		{name: "BOM remains content", input: "\ufeffhello\r\n", want: "\ufeffhello"},
		{name: "empty EOF", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := new(boundaryPushService)
			out, errOut := new(bytes.Buffer), new(bytes.Buffer)
			command := New(Dependencies{
				In: strings.NewReader(test.input), Out: out, ErrOut: errOut,
				IsTerminal: func() bool { return false }, Service: service,
			})
			command.SetArgs([]string{"push", "-", "--quiet"})
			err := command.Execute()
			if test.wantErr {
				if ExitCode(err) != 2 || service.calls != 0 {
					t.Fatalf("invalid input: exit=%d calls=%d", ExitCode(err), service.calls)
				}
				return
			}
			if err != nil || service.calls != 1 || service.body != test.want {
				t.Fatalf("valid input: error=%v calls=%d body matches=%v", err, service.calls, service.body == test.want)
			}
			if out.Len() != 0 || errOut.Len() != 0 {
				t.Fatal("quiet success produced output")
			}
		})
	}
}

func TestReadBodyBoundsOversizedStream(t *testing.T) {
	reader := &countedBodyReader{Reader: strings.NewReader(strings.Repeat("a", 1<<20))}
	_, err := readBody(reader, false, nil)
	if ExitCode(err) != 2 {
		t.Fatalf("exit=%d, want usage error", ExitCode(err))
	}
	if reader.bytes > 4096*4+3 {
		t.Fatalf("oversized stream read %d bytes", reader.bytes)
	}
}

type boundaryPushService struct {
	UnconfiguredService
	calls int
	body  string
}

func (s *boundaryPushService) Push(_ context.Context, request PushRequest) (PushResult, error) {
	s.calls++
	s.body = request.Body
	return PushResult{}, nil
}

type countedBodyReader struct {
	io.Reader
	bytes int
}

func (r *countedBodyReader) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	r.bytes += n
	return n, err
}
