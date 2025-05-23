package redis

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/redis/go-redis/v9"
)

// AddLLMStormTask adds a new LLMStormTask to the queue.
func (db *DB) AddLLMStormTask(ctx context.Context,
	prompt string,
	apiKey string,
) (taskID string, err error) {
	task := NewLLMStormTask(prompt, apiKey)
	payload, err := task.ToString()
	if err != nil {
		return "", errors.Wrap(err, "failed to serialize task using ToString")
	}

	if err = db.db.RPush(ctx, KeyTaskLLMStormPending, []any{payload}); err != nil {
		return "", errors.Wrapf(err, "failed to push task to key `%s`", KeyTaskLLMStormPending)
	}

	return task.TaskID, nil
}

// GetLLMStormTaskResult gets the result of a LLMStormTask by taskID.
func (db *DB) GetLLMStormTaskResult(ctx context.Context, taskID string) (task *LLMStormTask, err error) {
	key := KeyPrefixTaskLLMStormResult + taskID
	val, err := db.db.GetItem(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get task result by key `%s`", key)
	}

	// Check if the returned string is empty or only whitespace.
	if strings.TrimSpace(val) == "" {
		return nil, errors.Errorf("empty task result for taskID `%s` at key `%s`", taskID, key)
	}

	task, err = NewLLMStormTaskFromString(val)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse task result from string for taskID `%s`", taskID)
	}

	return task, nil
}

// AddHTMLCrawlerTask adds a new HTMLCrawlerTask to the queue.
func (db *DB) AddHTMLCrawlerTask(ctx context.Context, url string) (taskID string, err error) {
	task := NewHTMLCrawlerTask(url)
	payload, err := task.ToString()
	if err != nil {
		return "", errors.Wrap(err, "failed to serialize task using ToString")
	}

	if err = db.db.RPush(ctx, KeyTaskHTMLCrawlerPending, []any{payload}); err != nil {
		return "", errors.Wrapf(err, "failed to push task to key `%s`", KeyTaskHTMLCrawlerPending)
	}

	return task.TaskID, nil
}

// GetHTMLCrawlerTask is to get a HTMLCrawlerTask from the queue.
func (db *DB) GetHTMLCrawlerTask(ctx context.Context) (task *HTMLCrawlerTask, err error) {
	_, val, err := db.db.LPopKeysBlocking(ctx, KeyTaskHTMLCrawlerPending)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to pop task from key `%s`", KeyTaskHTMLCrawlerPending)
	}

	if val == "OK" {
		return nil, errors.Wrapf(redis.Nil, "got 'OK' for key %q", KeyTaskHTMLCrawlerPending)
	}

	task, err = NewHTMLCrawlerTaskFromString(val)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse html crawler task from string")
	}

	return task, nil
}

// GetHTMLCrawlerTaskResult gets the result of a HTMLCrawlerTask by taskID.
func (db *DB) GetHTMLCrawlerTaskResult(ctx context.Context, taskID string) (task *HTMLCrawlerTask, err error) {
	key := KeyPrefixTaskHTMLCrawlerResult + taskID
	val, err := db.db.GetItemBlocking(ctx, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get task result by key `%s`", key)
	}

	// Check if the returned string is empty or only whitespace.
	if strings.TrimSpace(val) == "" {
		return nil, errors.Errorf("empty task result for taskID `%s` at key `%s`", taskID, key)
	}

	task, err = NewHTMLCrawlerTaskFromString(val)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse task result from string for taskID `%s`", taskID)
	}

	return task, nil
}
