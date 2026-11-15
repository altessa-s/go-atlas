// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/filter"
	"github.com/altessa-s/go-aip/pagination/pagetoken"
	"github.com/altessa-s/go-horai/storage/memory"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	filterparser "github.com/altessa-s/go-aip/filter/parser"
	schedulerv1 "github.com/altessa-s/go-atlas/proto/gen/scheduler/v1"
	grpcsched "github.com/altessa-s/go-atlas/transport/grpc/handlers/scheduler"
	horai "github.com/altessa-s/go-horai"
	sched "github.com/altessa-s/go-horai/scheduler"
	schedtask "github.com/altessa-s/go-horai/task"
)

func newScheduler(t *testing.T, opts ...sched.Option) *sched.Scheduler {
	t.Helper()
	mem, err := memory.New(memory.WithMaxHistoryPerTask(100))
	require.NoError(t, err)
	opts = append([]sched.Option{sched.WithTickInterval(time.Hour)}, opts...)
	s, err := sched.New(mem, opts...)
	require.NoError(t, err)
	require.NoError(t, s.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})
	return s
}

// pageTokens returns a codec signing with a fixed test key.
func pageTokens(t *testing.T) *pagetoken.Codec {
	t.Helper()
	codec, err := pagetoken.New(bytes.Repeat([]byte{7}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	return codec
}

type parserStub struct {
	parser grpcsched.FilterParser
	err    error
}

func (p parserStub) Parse(ctx context.Context, expr string) (filter.Node, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.parser.Parse(ctx, expr)
}

type tokenCodecStub struct {
	codec *pagetoken.Codec
}

func (c tokenCodecStub) Issue(payload []byte, b pagetoken.Bindings) (string, error) {
	return c.codec.Issue(payload, b)
}

func (c tokenCodecStub) Resolve(token string, b pagetoken.Bindings) ([]byte, error) {
	return c.codec.Resolve(token, b)
}

var (
	_ grpcsched.TokenCodec   = (*pagetoken.Codec)(nil)
	_ grpcsched.FilterParser = (*filterparser.Parser)(nil)
)

func defaultParser(t *testing.T) grpcsched.FilterParser {
	t.Helper()
	p, err := filterparser.New()
	require.NoError(t, err)
	return p
}

func startClient(t *testing.T, s *sched.Scheduler, tokens *pagetoken.Codec) schedulerv1.SchedulerServiceClient {
	var tokenPort grpcsched.TokenCodec
	if tokens != nil {
		tokenPort = tokenCodecStub{codec: tokens}
	}
	return startClientWith(t, s, tokenPort, parserStub{parser: defaultParser(t)})
}

func startClientWith(t *testing.T, s *sched.Scheduler, tokens grpcsched.TokenCodec, parser grpcsched.FilterParser) schedulerv1.SchedulerServiceClient {
	t.Helper()
	h, err := grpcsched.New(s, tokens, parser)
	require.NoError(t, err)

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	h.Register(gs, nil)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return schedulerv1.NewSchedulerServiceClient(conn)
}

func TestNew_RequiresParser(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	_, err := grpcsched.New(s, nil, nil)
	require.ErrorIs(t, err, grpcsched.ErrFilterParserRequired)

	var parser *filterparser.Parser
	_, err = grpcsched.New(s, nil, parser)
	require.ErrorIs(t, err, grpcsched.ErrFilterParserRequired)
}

func TestHandler_List_ParserError(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	register(t, s, schedtask.Config{ID: "job"})
	parserErr := filter.ErrParseFailed
	client := startClientWith(t, s, pageTokens(t), parserStub{parser: defaultParser(t), err: parserErr})
	_, err := client.List(t.Context(), &schedulerv1.TasksListRequest{Filter: "id == 'job'"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func register(t *testing.T, s *sched.Scheduler, cfg schedtask.Config) {
	t.Helper()
	if cfg.Func == nil {
		cfg.Func = func(context.Context) error { return nil }
	}
	if cfg.Schedule == "" && cfg.RunAt.IsZero() {
		cfg.Schedule = "@every 1h"
	}
	require.NoError(t, s.Register(t.Context(), cfg))
}

func TestHandler_Get(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	register(t, s, schedtask.Config{ID: "report", Description: "nightly report", Meta: map[string]string{"team": "billing"}})
	client := startClient(t, s, pageTokens(t))

	resp, err := client.Get(t.Context(), &schedulerv1.TaskGetRequest{TaskId: "report"})
	require.NoError(t, err)
	st := resp.GetTaskState()
	require.Equal(t, "report", st.GetSummary().GetId())
	require.Equal(t, "nightly report", st.GetSummary().GetDescription())
	require.Equal(t, "@every 1h", st.GetSummary().GetSchedule())
	require.Equal(t, map[string]string{"team": "billing"}, st.GetMeta())
	require.Positive(t, st.GetCreatedAt())

	_, err = client.Get(t.Context(), &schedulerv1.TaskGetRequest{TaskId: "missing"})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestHandler_List(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	register(t, s, schedtask.Config{ID: "a"})
	register(t, s, schedtask.Config{ID: "b"})
	register(t, s, schedtask.Config{ID: "c"})
	client := startClient(t, s, pageTokens(t))

	first, err := client.List(t.Context(), &schedulerv1.TasksListRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.GetTasks(), 2)
	require.NotEmpty(t, first.GetNextCursor())

	second, err := client.List(t.Context(), &schedulerv1.TasksListRequest{Limit: 2, Cursor: first.NextCursor})
	require.NoError(t, err)
	require.Len(t, second.GetTasks(), 1)
	require.Empty(t, second.GetNextCursor())

	ids := []string{first.GetTasks()[0].GetId(), first.GetTasks()[1].GetId(), second.GetTasks()[0].GetId()}
	require.ElementsMatch(t, []string{"a", "b", "c"}, ids)

	_, err = client.List(t.Context(), &schedulerv1.TasksListRequest{Cursor: new("not-a-cursor")})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_List_PageTokensNotConfigured(t *testing.T) {
	t.Parallel()

	client := startClient(t, newScheduler(t), nil)

	_, err := client.List(t.Context(), &schedulerv1.TasksListRequest{})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))

	_, err = client.ListHistory(t.Context(), &schedulerv1.TaskHistoryListRequest{TaskId: "x"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestHandler_ListHistory(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	register(t, s, schedtask.Config{ID: "job"})
	client := startClient(t, s, pageTokens(t))

	for range 2 {
		_, err := client.Trigger(t.Context(), &schedulerv1.TaskTriggerRequest{TaskId: "job"})
		// A trigger may race the previous run still holding the dispatch slot.
		if err == nil {
			testhelpers.WaitFor(t, 5*time.Second, func() bool {
				st, gerr := s.Task(t.Context(), "job")
				return gerr == nil && st != nil && st.RunStartedAt == 0 && st.LastRunAt > 0
			}, "triggered run must finish")
		}
	}

	var resp *schedulerv1.TaskHistoryListResponse
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		var err error
		resp, err = client.ListHistory(t.Context(), &schedulerv1.TaskHistoryListRequest{TaskId: "job", Limit: 10})
		return err == nil && len(resp.GetEntries()) > 0
	}, "history must record the triggered run")
	require.Equal(t, "job", resp.GetEntries()[0].GetTaskId())

	_, err := client.ListHistory(t.Context(), &schedulerv1.TaskHistoryListRequest{TaskId: "job", Cursor: new("garbage")})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetStatus(t *testing.T) {
	t.Parallel()

	s := newScheduler(t, sched.WithMaxConcurrentTasks(4))
	client := startClient(t, s, pageTokens(t))

	resp, err := client.GetStatus(t.Context(), &schedulerv1.SchedulerStatusGetRequest{})
	require.NoError(t, err)
	require.True(t, resp.GetIsRunning())
	require.Equal(t, int32(0), resp.GetRunningTasksCount())
	require.Equal(t, int32(s.Status().AvailableSlots), resp.GetAvailableSlots())
	require.Equal(t, s.Status().Leader, resp.GetIsLeader())
}

func TestHandler_Lifecycle(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	register(t, s, schedtask.Config{ID: "job"})
	register(t, s, schedtask.Config{ID: "external", Unmanaged: true})
	client := startClient(t, s, pageTokens(t))
	ctx := t.Context()

	taskStatus := func(id string) horai.TaskStatus {
		st, err := s.Task(ctx, id)
		require.NoError(t, err)
		return st.Status
	}

	_, err := client.Resume(ctx, &schedulerv1.TaskResumeRequest{TaskId: "job"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "resume of an active task")

	_, err = client.Pause(ctx, &schedulerv1.TaskPauseRequest{TaskId: "job"})
	require.NoError(t, err)
	require.Equal(t, horai.TaskStatusPaused, taskStatus("job"))

	_, err = client.Resume(ctx, &schedulerv1.TaskResumeRequest{TaskId: "job"})
	require.NoError(t, err)
	require.Equal(t, horai.TaskStatusActive, taskStatus("job"))

	_, err = client.Enable(ctx, &schedulerv1.TaskEnableRequest{TaskId: "job"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "enable of an active task")

	_, err = client.Disable(ctx, &schedulerv1.TaskDisableRequest{TaskId: "job"})
	require.NoError(t, err)
	require.Equal(t, horai.TaskStatusDisabled, taskStatus("job"))

	_, err = client.Trigger(ctx, &schedulerv1.TaskTriggerRequest{TaskId: "job"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "trigger of a disabled task")

	_, err = client.Enable(ctx, &schedulerv1.TaskEnableRequest{TaskId: "job"})
	require.NoError(t, err)
	require.Equal(t, horai.TaskStatusActive, taskStatus("job"))

	_, err = client.SkipNextRun(ctx, &schedulerv1.TaskSkipNextRunRequest{TaskId: "job"})
	require.NoError(t, err)
	st, err := s.Task(ctx, "job")
	require.NoError(t, err)
	require.True(t, st.SkipNextRun)

	_, err = client.Pause(ctx, &schedulerv1.TaskPauseRequest{TaskId: "external"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "pause of an unmanaged task")
	_, err = client.Disable(ctx, &schedulerv1.TaskDisableRequest{TaskId: "external"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "disable of an unmanaged task")

	_, err = client.Trigger(ctx, &schedulerv1.TaskTriggerRequest{TaskId: "job"})
	require.NoError(t, err)
}

func TestHandler_UnknownTask(t *testing.T) {
	t.Parallel()

	client := startClient(t, newScheduler(t), pageTokens(t))
	ctx := t.Context()

	calls := map[string]func() error{
		"pause":  func() error { _, err := client.Pause(ctx, &schedulerv1.TaskPauseRequest{TaskId: "nope"}); return err },
		"resume": func() error { _, err := client.Resume(ctx, &schedulerv1.TaskResumeRequest{TaskId: "nope"}); return err },
		"disable": func() error {
			_, err := client.Disable(ctx, &schedulerv1.TaskDisableRequest{TaskId: "nope"})
			return err
		},
		"enable": func() error { _, err := client.Enable(ctx, &schedulerv1.TaskEnableRequest{TaskId: "nope"}); return err },
		"skip": func() error {
			_, err := client.SkipNextRun(ctx, &schedulerv1.TaskSkipNextRunRequest{TaskId: "nope"})
			return err
		},
		"trigger": func() error {
			_, err := client.Trigger(ctx, &schedulerv1.TaskTriggerRequest{TaskId: "nope"})
			return err
		},
	}
	for name, call := range calls {
		require.Equal(t, codes.NotFound, status.Code(call()), name)
	}
}

func TestHandler_List_InvalidFilter(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	register(t, s, schedtask.Config{ID: "job"})
	client := startClient(t, s, pageTokens(t))

	// "secret == 1" is rejected by the task storage's field allowlist; the
	// empty in-memory history never evaluates it, so only the parse failure
	// is asserted on ListHistory.
	tests := []struct {
		name    string
		expr    string
		history bool
	}{
		{name: "malformed", expr: "status ==", history: true},
		{name: "field not allowed", expr: "secret == 1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.List(t.Context(), &schedulerv1.TasksListRequest{Filter: tc.expr})
			require.Equal(t, codes.InvalidArgument, status.Code(err), "List: %v", err)

			if tc.history {
				_, err = client.ListHistory(t.Context(), &schedulerv1.TaskHistoryListRequest{TaskId: "job", Filter: tc.expr})
				require.Equal(t, codes.InvalidArgument, status.Code(err), "ListHistory: %v", err)
			}
		})
	}
}

func TestHandler_List_Filter(t *testing.T) {
	t.Parallel()

	s := newScheduler(t)
	register(t, s, schedtask.Config{ID: "daily/report"})
	register(t, s, schedtask.Config{ID: "daily/cleanup", Unmanaged: true})
	register(t, s, schedtask.Config{ID: "hourly/sync"})
	client := startClient(t, s, pageTokens(t))

	ids := func(expr string) []string {
		t.Helper()
		resp, err := client.List(t.Context(), &schedulerv1.TasksListRequest{Filter: expr})
		require.NoError(t, err, expr)
		out := make([]string, 0, len(resp.GetTasks()))
		for _, task := range resp.GetTasks() {
			out = append(out, task.GetId())
		}
		return out
	}

	require.ElementsMatch(t, []string{"daily/report", "daily/cleanup"}, ids(`id.startsWith("daily/")`))
	require.ElementsMatch(t, []string{"daily/report"}, ids(`id.startsWith("daily/") && !unmanaged`))
	require.ElementsMatch(t, []string{"hourly/sync", "daily/cleanup"}, ids(`id in ["hourly/sync", "daily/cleanup"]`))
	require.ElementsMatch(t, []string{"daily/report", "hourly/sync"}, ids(`failures < 1 && unmanaged == false`))
}
