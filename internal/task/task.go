package task

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Step 任务里的一个步骤
type Step struct {
	Name     string
	Optional bool // 失败不中断任务，只标记警告
	Run      func(ctx context.Context) error
}

// Event 步骤进度：开始时 Done 为假；完成时 Done 为真，可选步骤失败时 Err 同时非空；失败时只有 Err
type Event struct {
	Step int
	Done bool
	Err  error
}

// Run 顺序执行步骤并通过 report 回报进度
func Run(ctx context.Context, steps []Step, report func(Event)) error {
	for i, s := range steps {
		report(Event{Step: i})
		err := s.Run(ctx)
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && !s.Optional {
			report(Event{Step: i, Err: err})
			return err
		}
		report(Event{Step: i, Done: true, Err: err})
	}
	return nil
}

// httpGet 带重试的 GET，非 2xx 视为失败
func httpGet(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		var body []byte
		if body, err = get(ctx, client, url); err == nil {
			return body, nil
		}
	}
	return nil, err
}

func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
