package redis

import (
	"time"

	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/go-utils/v5/json"
	"github.com/pkg/errors"
)

const (
	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusSuccess = "success"
	TaskStatusFailed  = "failed"
)

type baseTask struct {
	TaskID       string     `json:"task_id"`
	CreatedAt    time.Time  `json:"created_at"`
	Status       string     `json:"status"`
	FailedReason *string    `json:"failed_reason,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

func newBaseTask() baseTask {
	return baseTask{
		TaskID:    gutils.UUID7(),
		CreatedAt: time.Now(),
		Status:    TaskStatusPending,
	}
}

type LLMStormTask struct {
	baseTask
	Prompt           string           `json:"prompt"`
	APIKey           string           `json:"api_key"`
	ResultArticle    *string          `json:"result_article,omitempty"`
	ResultReferences *stormReferences `json:"result_references,omitempty"`
	// Runner is the name of the runner that processed the task
	Runner string `json:"runner"`
}

type stormReferences struct {
	UrlToUnifiedIndex map[string]int          `json:"url_to_unified_index"`
	UrlToInfo         map[string]stormUrlInfo `json:"url_to_info"`
}

type stormUrlInfo struct {
	Url          string           `json:"url"`
	Description  string           `json:"description"`
	Snippets     []string         `json:"snippets"`
	Title        string           `json:"title"`
	Meta         stormUrlInfoMeta `json:"meta"`
	CitationUUID int              `json:"citation_uuid"`
}

type stormUrlInfoMeta struct {
	Query string `json:"query"`
}

// ToString returns the JSON representation of a StormTask.
func (s *LLMStormTask) ToString() (string, error) {
	data, err := json.MarshalToString(s)
	if err != nil {
		return "", errors.Wrap(err, "marshal")
	}

	return data, nil
}

// NewLLMStormTaskFromString creates a StormTask instance from its JSON string representation.
func NewLLMStormTaskFromString(taskStr string) (*LLMStormTask, error) {
	var task LLMStormTask
	if err := json.Unmarshal([]byte(taskStr), &task); err != nil {
		return nil, errors.Wrapf(err, "unmarshal llm storm task %q", taskStr)
	}
	return &task, nil
}

// NewLLMStormTask creates a new StormTask instance.
func NewLLMStormTask(prompt, apikey string) *LLMStormTask {
	return &LLMStormTask{
		baseTask: newBaseTask(),
		Prompt:   prompt,
		APIKey:   apikey,
	}
}

// HTMLCrawlerTask is a task for crawling HTML pages.
type HTMLCrawlerTask struct {
	baseTask
	Url        string `json:"url"`
	ResultHTML []byte `json:"result_html,omitempty"`
}

// ToString returns the JSON representation of a HTMLCrawlerTask.
func (s *HTMLCrawlerTask) ToString() (string, error) {
	data, err := json.MarshalToString(s)
	if err != nil {
		return "", errors.Wrap(err, "marshal")
	}

	return data, nil
}

// NewHTMLCrawlerTaskFromString creates a HTMLCrawlerTask instance from its JSON string representation.
func NewHTMLCrawlerTaskFromString(taskStr string) (*HTMLCrawlerTask, error) {
	var task HTMLCrawlerTask
	if err := json.Unmarshal([]byte(taskStr), &task); err != nil {
		return nil, errors.Wrapf(err, "unmarshal html crawler task %q", taskStr)
	}

	return &task, nil
}

// NewHTMLCrawlerTask creates a new HTMLCrawlerTask instance.
func NewHTMLCrawlerTask(url string) *HTMLCrawlerTask {
	return &HTMLCrawlerTask{
		baseTask: newBaseTask(),
		Url:      url,
	}
}
