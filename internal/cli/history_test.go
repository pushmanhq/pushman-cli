package cli

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHistoryShowDisplaysNewestFirst(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 1, 3} {
		t.Run(fmt.Sprintf("%d revisions", count), func(t *testing.T) {
			revisions := make([]HistoryRevision, count)
			for i := range revisions {
				// The last two revisions have equal timestamps; retain API tie order.
				revisions[i] = HistoryRevision{ID: fmt.Sprintf("msg_%d", i+1), UpdatedAt: time.Unix(int64(min(i, 1)), 0)}
			}
			service := &historyStubService{detail: HistoryDetail{LogicalMessageID: "logical_test", Revisions: revisions}}
			before := append([]HistoryRevision{}, revisions...)
			out := new(bytes.Buffer)
			root := New(Dependencies{Out: out, Service: service})
			root.SetArgs([]string{"history", "show", "msg_test"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			previous := -1
			for number := count; number >= 1; number-- {
				heading := fmt.Sprintf("Revision %d/%d: msg_%d", number, count, number)
				position := strings.Index(out.String(), heading)
				if position <= previous {
					t.Fatalf("missing or out-of-order %q in %q", heading, out.String())
				}
				previous = position
			}
			if strings.Count(out.String(), "Revision ") != count {
				t.Fatalf("unexpected revision count in %q", out.String())
			}
			if !reflect.DeepEqual(service.detail.Revisions, before) {
				t.Fatal("presentation mutated the service's chronological revision order")
			}
			machine := mapMCPMessage(service.detail)
			for i := range revisions {
				if machine.Revisions[i].ID != revisions[i].ID {
					t.Fatal("MCP revision order changed")
				}
			}
		})
	}
}

type historyStubService struct {
	UnconfiguredService
	detail HistoryDetail
}

func (s *historyStubService) HistoryShow(context.Context, string) (HistoryDetail, error) {
	return s.detail, nil
}
